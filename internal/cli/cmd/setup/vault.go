package setup

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/service"
)

const (
	vaultImage          = "hashicorp/vault:1.15"
	vaultContainerPort  = 8200
	vaultLabel          = "app=vault"
	vaultServiceName    = "vault"
	vaultDeploymentName = "vault"
	vaultConfigMapName  = "vault-config"
	vaultPVCName        = "vault-data"
	vaultPVCSize        = "1Gi"
	vaultDataPath       = "/vault/data"
	vaultTLSPath        = "/vault/tls"
	vaultConfigPath     = "/vault/config"
	credentialsDir      = ".netweave"
	credentialsFile     = "credentials.json"
	defaultPKITTL       = "87600h" // 10 years
	defaultIntPKITTL    = "43800h" // 5 years
	defaultCertTTL      = "8760h"  // 1 year
	vaultReadyTimeout   = 120 * time.Second
	vaultHTTPTimeout    = 10 * time.Second
	vaultSetupSteps     = 10
)

// vaultCredentials holds Vault access credentials saved to disk.
type vaultCredentials struct {
	VaultAddr  string   `json:"vault_addr"`
	RootToken  string   `json:"root_token"`
	UnsealKeys []string `json:"unseal_keys"`
}

func newVaultCmd() *cobra.Command {
	vaultCmd := &cobra.Command{
		Use:   "vault",
		Short: "Deploy Vault, initialize, unseal, and configure PKI",
		Long: `Deploys HashiCorp Vault in production mode into the Kubernetes cluster
with TLS encryption and file-based storage. Generates TLS certificates for
Vault itself, initializes and unseals the Vault instance, and sets up the
PKI secrets engine with Root CA, Intermediate CA, and the netweave-mtls
role for certificate issuance.

Data is persisted to a PVC so it survives pod restarts. After a restart
the Vault pod will come up sealed — re-run "setup vault" to unseal it.

NOTE: This command uses a single unseal key (Shamir share=1, threshold=1)
which is suitable for local development and testing. For production
deployments, use the official Vault Helm chart with multiple unseal keys
or auto-unseal via a cloud KMS provider.`,
		RunE: runVaultSetup,
	}

	vaultCmd.AddCommand(newVaultStatusCmd())
	vaultCmd.AddCommand(newVaultCredentialsCmd())

	return vaultCmd
}

func newVaultStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Vault initialization and seal status",
		RunE:  runVaultStatus,
	}
}

func newVaultCredentialsCmd() *cobra.Command {
	var showSecrets bool

	credCmd := &cobra.Command{
		Use:   "credentials",
		Short: "Display saved Vault credentials",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runVaultCredentials(showSecrets)
		},
	}

	credCmd.Flags().BoolVar(
		&showSecrets, "show-secrets", false,
		"Show full token and unseal key values (default: masked)",
	)

	return credCmd
}

func runVaultSetup(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	conn, err := service.NewConnector(cmd.Global.Kubeconfig, cmd.Global.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create K8s connector: %w", err)
	}
	defer conn.Close()

	steps := cmd.Printer.NewStepProgress(vaultSetupSteps)

	// Step 1: Create namespace.
	steps.Stepf("Creating namespace %q...", cmd.Global.Namespace)
	if nsErr := ensureNamespace(ctx, conn); nsErr != nil {
		return nsErr
	}
	steps.Donef("Namespace ready")

	// Step 2: Generate Vault TLS certs.
	steps.Stepf("Generating Vault TLS certificates...")
	tlsCert, tlsKey, caCert, genErr := generateSelfSignedTLS(
		"vault." + cmd.Global.Namespace + ".svc",
	)
	if genErr != nil {
		return fmt.Errorf("failed to generate TLS: %w", genErr)
	}
	steps.Donef("TLS certificates generated")

	// Step 3: Create K8s secrets for Vault TLS.
	steps.Stepf("Creating Vault TLS secrets...")
	if secErr := createVaultTLSSecrets(ctx, conn, tlsCert, tlsKey, caCert); secErr != nil {
		return secErr
	}
	steps.Donef("TLS secrets created")

	// Step 4: Create Vault config ConfigMap.
	steps.Stepf("Creating Vault configuration...")
	if cmErr := createVaultConfigMap(ctx, conn); cmErr != nil {
		return cmErr
	}
	steps.Donef("Vault ConfigMap created")

	// Step 5: Create Vault PVC.
	steps.Stepf("Creating Vault persistent storage...")
	if pvcErr := createVaultPVC(ctx, conn); pvcErr != nil {
		return pvcErr
	}
	steps.Donef("Vault PVC created")

	// Step 6: Deploy Vault (PVC binds when the pod is scheduled).
	steps.Stepf("Deploying Vault...")
	if depErr := deployVault(ctx, conn); depErr != nil {
		return depErr
	}
	steps.Donef("Vault deployed")

	// Step 7: Wait for Vault pod.
	steps.Stepf("Waiting for Vault pod to be running...")
	if waitErr := conn.WaitForPod(ctx, vaultLabel, vaultReadyTimeout); waitErr != nil {
		return fmt.Errorf("vault pod not ready: %w", waitErr)
	}
	steps.Donef("Vault pod running")

	// Step 8: Port-forward and initialize/unseal.
	steps.Stepf("Initializing and unsealing Vault...")
	tlsClient, tlsClientErr := service.NewTLSClient(vaultHTTPTimeout, caCert)
	if tlsClientErr != nil {
		return fmt.Errorf("failed to create TLS client: %w", tlsClientErr)
	}

	fw, fwErr := conn.PortForward(ctx, vaultLabel, vaultContainerPort)
	if fwErr != nil {
		return fmt.Errorf("failed to port-forward to Vault: %w", fwErr)
	}
	defer close(fw.StopChan)

	vaultAddr := fmt.Sprintf("https://127.0.0.1:%d", fw.LocalPort)
	creds, initErr := initAndUnsealVault(ctx, tlsClient, vaultAddr)
	if initErr != nil {
		return initErr
	}
	steps.Donef("Vault initialized and unsealed")

	// Step 9: Setup PKI using the in-cluster address for CA URLs.
	steps.Stepf("Setting up PKI secrets engine...")
	inClusterAddr := fmt.Sprintf(
		"https://vault.%s.svc.cluster.local:8200", cmd.Global.Namespace,
	)
	if pkiErr := setupVaultPKI(ctx, tlsClient, vaultAddr, inClusterAddr, creds.RootToken); pkiErr != nil {
		return pkiErr
	}
	steps.Donef("PKI configured with Root CA, Intermediate CA, and roles")

	// Step 10: Save credentials (don't save ephemeral port-forward address).
	steps.Stepf("Saving credentials...")
	creds.VaultAddr = ""
	if saveErr := saveCredentials(creds); saveErr != nil {
		return saveErr
	}
	steps.Donef("Credentials saved to ~/%s/%s", credentialsDir, credentialsFile)

	cmd.Printer.Success("Vault setup complete!")
	return nil
}

// runVaultStatus connects to Vault and displays its health and seal status.
func runVaultStatus(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := service.NewConnector(cmd.Global.Kubeconfig, cmd.Global.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create K8s connector: %w", err)
	}
	defer conn.Close()

	fw, fwErr := conn.PortForward(ctx, vaultLabel, vaultContainerPort)
	if fwErr != nil {
		return fmt.Errorf("failed to port-forward to Vault: %w", fwErr)
	}
	defer close(fw.StopChan)

	vaultAddr := fmt.Sprintf("https://127.0.0.1:%d", fw.LocalPort)
	tlsClient := service.NewInsecureTLSClient(vaultHTTPTimeout)

	statusCode, health, err := vaultAPIGetRaw(
		ctx, tlsClient, vaultAddr, "/v1/sys/health", "",
	)
	if err != nil {
		return fmt.Errorf("failed to query Vault health: %w", err)
	}

	initialized := boolFromMap(health, "initialized")
	sealed := boolFromMap(health, "sealed")
	version := stringFromMap(health, "version")
	storageType := stringFromMap(health, "storage_type")

	cmd.Printer.Infof("Vault Status (HTTP %d)", statusCode)
	cmd.Printer.Infof("  Initialized:  %t", initialized)
	cmd.Printer.Infof("  Sealed:       %t", sealed)
	cmd.Printer.Infof("  Version:      %s", version)
	cmd.Printer.Infof("  Storage Type: %s", storageType)

	return nil
}

// runVaultCredentials loads and displays saved Vault credentials.
func runVaultCredentials(showSecrets bool) error {
	creds, err := loadVaultCredentials()
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w", err)
	}

	token := maskSecret(creds.RootToken, showSecrets)

	cmd.Printer.Infof("Vault Credentials (~/%s/%s)", credentialsDir, credentialsFile)
	cmd.Printer.Infof("  Root Token:  %s", token)

	if len(creds.UnsealKeys) == 0 {
		cmd.Printer.Infof("  Unseal Keys: (none)")
	}
	for i, key := range creds.UnsealKeys {
		cmd.Printer.Infof("  Unseal Key %d: %s", i+1, maskSecret(key, showSecrets))
	}

	if !showSecrets {
		cmd.Printer.Infof("")
		cmd.Printer.Infof("Use --show-secrets to reveal full values")
	}

	return nil
}

// maskSecret returns a masked version of a secret string.
func maskSecret(s string, reveal bool) string {
	if reveal || len(s) == 0 {
		return s
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

// boolFromMap safely extracts a bool from a map.
func boolFromMap(m map[string]interface{}, key string) bool {
	v, ok := m[key].(bool)
	if !ok {
		return false
	}
	return v
}

// stringFromMap safely extracts a string from a map.
func stringFromMap(m map[string]interface{}, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}

func ensureNamespace(ctx context.Context, conn *service.Connector) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: conn.Namespace()},
	}
	_, err := conn.Clientset().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	return nil
}

// generateSelfSignedTLS creates a self-signed CA and server certificate.
// Returns PEM-encoded cert, key, and CA cert.
func generateSelfSignedTLS(
	dnsName string,
) (certPEM, keyPEM, caCertPEM []byte, err error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "vault-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(
		rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal server key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames: []string{
			dnsName,
			"vault",
			"vault." + cmd.Global.Namespace,
			"vault." + cmd.Global.Namespace + ".svc",
			"vault." + cmd.Global.Namespace + ".svc.cluster.local",
			"localhost",
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	serverCertDER, err := x509.CreateCertificate(
		rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})

	return certPEM, keyPEM, caCertPEM, nil
}

func createVaultTLSSecrets(
	ctx context.Context,
	conn *service.Connector,
	cert, key, caCert []byte,
) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-tls",
			Namespace: conn.Namespace(),
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": cert,
			"tls.key": key,
			"ca.crt":  caCert,
		},
	}

	_, err := conn.Clientset().CoreV1().Secrets(conn.Namespace()).Create(
		ctx, secret, metav1.CreateOptions{},
	)
	if apierrors.IsAlreadyExists(err) {
		_, err = conn.Clientset().CoreV1().Secrets(conn.Namespace()).Update(
			ctx, secret, metav1.UpdateOptions{},
		)
	}
	if err != nil {
		return fmt.Errorf("failed to create vault TLS secret: %w", err)
	}
	return nil
}

// createVaultConfigMap creates a ConfigMap with the Vault HCL configuration
// for production mode: file storage backend and TLS listener.
func createVaultConfigMap(ctx context.Context, conn *service.Connector) error {
	vaultHCL := `disable_mlock = true
ui            = true

storage "file" {
  path = "` + vaultDataPath + `"
}

listener "tcp" {
  address       = "0.0.0.0:8200"
  tls_cert_file = "` + vaultTLSPath + `/tls.crt"
  tls_key_file  = "` + vaultTLSPath + `/tls.key"
}

api_addr     = "https://0.0.0.0:8200"
cluster_addr = "https://0.0.0.0:8201"
`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaultConfigMapName,
			Namespace: conn.Namespace(),
		},
		Data: map[string]string{
			"vault.hcl": vaultHCL,
		},
	}

	_, err := conn.Clientset().CoreV1().ConfigMaps(conn.Namespace()).Create(
		ctx, cm, metav1.CreateOptions{},
	)
	if apierrors.IsAlreadyExists(err) {
		_, err = conn.Clientset().CoreV1().ConfigMaps(conn.Namespace()).Update(
			ctx, cm, metav1.UpdateOptions{},
		)
	}
	if err != nil {
		return fmt.Errorf("failed to create vault config configmap: %w", err)
	}

	return nil
}

// createVaultPVC creates a PersistentVolumeClaim for Vault file storage.
// PVCs are immutable once created, so existing PVCs are left as-is.
func createVaultPVC(ctx context.Context, conn *service.Connector) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaultPVCName,
			Namespace: conn.Namespace(),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(vaultPVCSize),
				},
			},
		},
	}

	_, err := conn.Clientset().CoreV1().PersistentVolumeClaims(conn.Namespace()).Create(
		ctx, pvc, metav1.CreateOptions{},
	)
	if apierrors.IsAlreadyExists(err) {
		cmd.Printer.Verbosef("PVC %q already exists, skipping", vaultPVCName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to create vault PVC: %w", err)
	}

	return nil
}

func deployVault(ctx context.Context, conn *service.Connector) error {
	labels := map[string]string{"app": "vault"}
	replicas := int32(1)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaultDeploymentName,
			Namespace: conn.Namespace(),
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       buildVaultPodSpec(),
			},
		},
	}

	_, err := conn.Clientset().AppsV1().Deployments(conn.Namespace()).Create(
		ctx, deployment, metav1.CreateOptions{},
	)
	if apierrors.IsAlreadyExists(err) {
		_, err = conn.Clientset().AppsV1().Deployments(conn.Namespace()).Update(
			ctx, deployment, metav1.UpdateOptions{},
		)
	}
	if err != nil {
		return fmt.Errorf("failed to deploy Vault: %w", err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaultServiceName,
			Namespace: conn.Namespace(),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "https",
				Port:       int32(vaultContainerPort),
				TargetPort: intstr.FromInt32(int32(vaultContainerPort)),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	_, err = conn.Clientset().CoreV1().Services(conn.Namespace()).Create(
		ctx, svc, metav1.CreateOptions{},
	)
	if apierrors.IsAlreadyExists(err) {
		_, err = conn.Clientset().CoreV1().Services(conn.Namespace()).Update(
			ctx, svc, metav1.UpdateOptions{},
		)
	}
	if err != nil {
		return fmt.Errorf("failed to create Vault service: %w", err)
	}

	return nil
}

func buildVaultPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "vault",
			Image: vaultImage,
			Command: []string{
				"vault", "server",
				"-config=" + vaultConfigPath + "/vault.hcl",
			},
			Ports: []corev1.ContainerPort{{
				ContainerPort: int32(vaultContainerPort),
				Protocol:      corev1.ProtocolTCP,
			}},
			Env: []corev1.EnvVar{
				{Name: "VAULT_ADDR", Value: "https://127.0.0.1:8200"},
				{Name: "VAULT_SKIP_VERIFY", Value: "true"},
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path:   "/v1/sys/health?standbyok=true&uninitcode=200&sealedcode=200",
						Port:   intstr.FromInt32(int32(vaultContainerPort)),
						Scheme: corev1.URISchemeHTTPS,
					},
				},
				InitialDelaySeconds: 5,
				PeriodSeconds:       5,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "vault-data",
					MountPath: vaultDataPath,
				},
				{
					Name:      "vault-tls",
					MountPath: vaultTLSPath,
					ReadOnly:  true,
				},
				{
					Name:      "vault-config",
					MountPath: vaultConfigPath,
					ReadOnly:  true,
				},
			},
		}},
		Volumes: []corev1.Volume{
			{
				Name: "vault-data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: vaultPVCName,
					},
				},
			},
			{
				Name: "vault-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "vault-tls",
					},
				},
			},
			{
				Name: "vault-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: vaultConfigMapName,
						},
					},
				},
			},
		},
	}
}

// initAndUnsealVault initializes Vault if needed and unseals it.
// When already initialized, it loads credentials from disk and unseals.
func initAndUnsealVault(
	ctx context.Context,
	client *http.Client,
	vaultAddr string,
) (*vaultCredentials, error) {
	initStatus, err := vaultAPIGet(ctx, client, vaultAddr, "/v1/sys/init", "")
	if err != nil {
		return nil, fmt.Errorf("failed to check Vault init status: %w", err)
	}

	initialized := boolFromMap(initStatus, "initialized")
	if initialized {
		return handleAlreadyInitialized(ctx, client, vaultAddr)
	}

	return initializeAndUnseal(ctx, client, vaultAddr)
}

// handleAlreadyInitialized loads saved credentials and unseals Vault if sealed.
func handleAlreadyInitialized(
	ctx context.Context,
	client *http.Client,
	vaultAddr string,
) (*vaultCredentials, error) {
	cmd.Printer.Verbosef("Vault already initialized, loading credentials...")

	creds, err := loadVaultCredentials()
	if err != nil {
		return nil, fmt.Errorf(
			"vault is initialized but credentials not found at ~/%s/%s: %w\n"+
				"  Recovery options:\n"+
				"  1. If you have the unseal keys, create the credentials file manually\n"+
				"  2. Run 'netweave-cli setup vault credentials' on the machine that initialized Vault\n"+
				"  3. As a last resort: 'kubectl delete pvc %s -n %s' and re-run setup (DATA LOSS)",
			credentialsDir, credentialsFile, err, vaultPVCName, cmd.Global.Namespace,
		)
	}

	sealStatus, sealErr := vaultAPIGet(ctx, client, vaultAddr, "/v1/sys/seal-status", "")
	if sealErr != nil {
		return nil, fmt.Errorf("failed to check seal status: %w", sealErr)
	}

	if !boolFromMap(sealStatus, "sealed") {
		cmd.Printer.Verbosef("Vault is already unsealed")
		creds.VaultAddr = vaultAddr
		return creds, nil
	}

	cmd.Printer.Verbosef("Vault is sealed, unsealing...")
	if unsealErr := unsealVault(ctx, client, vaultAddr, creds.UnsealKeys); unsealErr != nil {
		return nil, unsealErr
	}

	creds.VaultAddr = vaultAddr
	return creds, nil
}

// initializeAndUnseal initializes Vault with 1 key share and unseals it.
func initializeAndUnseal(
	ctx context.Context,
	client *http.Client,
	vaultAddr string,
) (*vaultCredentials, error) {
	initReq := map[string]interface{}{
		"secret_shares":    1,
		"secret_threshold": 1,
	}
	initResp, err := vaultAPIPut(ctx, client, vaultAddr, "/v1/sys/init", "", initReq)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Vault: %w", err)
	}

	keys, keysOk := initResp["keys"].([]interface{})
	if !keysOk {
		return nil, fmt.Errorf("vault init response missing keys")
	}
	rootToken, tokenOk := initResp["root_token"].(string)
	if !tokenOk {
		return nil, fmt.Errorf("vault init response missing root_token")
	}

	unsealKeys := make([]string, 0, len(keys))
	for _, k := range keys {
		if s, isStr := k.(string); isStr {
			unsealKeys = append(unsealKeys, s)
		}
	}

	if unsealErr := unsealVault(ctx, client, vaultAddr, unsealKeys); unsealErr != nil {
		return nil, unsealErr
	}

	return &vaultCredentials{
		VaultAddr:  vaultAddr,
		RootToken:  rootToken,
		UnsealKeys: unsealKeys,
	}, nil
}

// unsealVault applies each unseal key to the Vault unseal endpoint.
func unsealVault(
	ctx context.Context,
	client *http.Client,
	vaultAddr string,
	keys []string,
) error {
	for _, key := range keys {
		unsealReq := map[string]interface{}{"key": key}
		if _, err := vaultAPIPut(
			ctx, client, vaultAddr, "/v1/sys/unseal", "", unsealReq,
		); err != nil {
			return fmt.Errorf("failed to unseal Vault: %w", err)
		}
	}
	return nil
}

func setupVaultPKI(
	ctx context.Context,
	client *http.Client,
	vaultAddr, inClusterAddr, token string,
) error {
	if err := setupRootCA(ctx, client, vaultAddr, inClusterAddr, token); err != nil {
		return err
	}

	if err := setupIntermediateCA(ctx, client, vaultAddr, inClusterAddr, token); err != nil {
		return err
	}

	return createPKIRoles(ctx, client, vaultAddr, token)
}

func setupRootCA(
	ctx context.Context,
	client *http.Client,
	vaultAddr, inClusterAddr, token string,
) error {
	if err := enableSecretEngine(
		ctx, client, vaultAddr, token, "pki", "pki",
	); err != nil {
		return err
	}

	if _, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/sys/mounts/pki/tune", token,
		map[string]interface{}{"max_lease_ttl": defaultPKITTL},
	); err != nil {
		return fmt.Errorf("failed to tune pki mount: %w", err)
	}

	if _, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/pki/root/generate/internal", token,
		map[string]interface{}{
			"common_name":  "Netweave Root CA",
			"issuer_name":  "netweave-root",
			"ttl":          defaultPKITTL,
			"key_type":     "rsa",
			"key_bits":     4096,
			"organization": "Netweave",
		},
	); err != nil {
		return fmt.Errorf("failed to generate root CA: %w", err)
	}

	if _, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/pki/config/urls", token,
		map[string]interface{}{
			"issuing_certificates":    inClusterAddr + "/v1/pki/ca",
			"crl_distribution_points": inClusterAddr + "/v1/pki/crl",
		},
	); err != nil {
		return fmt.Errorf("failed to configure CA URLs: %w", err)
	}

	return nil
}

func setupIntermediateCA(
	ctx context.Context,
	client *http.Client,
	vaultAddr, inClusterAddr, token string,
) error {
	if err := enableSecretEngine(
		ctx, client, vaultAddr, token, "pki_int", "pki",
	); err != nil {
		return err
	}

	if _, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/sys/mounts/pki_int/tune", token,
		map[string]interface{}{"max_lease_ttl": defaultIntPKITTL},
	); err != nil {
		return fmt.Errorf("failed to tune pki_int mount: %w", err)
	}

	signedCert, err := generateAndSignIntermediate(ctx, client, vaultAddr, token)
	if err != nil {
		return err
	}

	if _, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/pki_int/intermediate/set-signed", token,
		map[string]interface{}{"certificate": signedCert},
	); err != nil {
		return fmt.Errorf("failed to set signed intermediate cert: %w", err)
	}

	if _, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/pki_int/config/urls", token,
		map[string]interface{}{
			"issuing_certificates":    inClusterAddr + "/v1/pki_int/ca",
			"crl_distribution_points": inClusterAddr + "/v1/pki_int/crl",
		},
	); err != nil {
		return fmt.Errorf("failed to configure intermediate CA URLs: %w", err)
	}

	return nil
}

func generateAndSignIntermediate(
	ctx context.Context,
	client *http.Client,
	vaultAddr, token string,
) (string, error) {
	csrResp, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/pki_int/intermediate/generate/internal", token,
		map[string]interface{}{
			"common_name":  "Netweave Intermediate CA",
			"issuer_name":  "netweave-intermediate",
			"key_type":     "rsa",
			"key_bits":     4096,
			"organization": "Netweave",
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate intermediate CSR: %w", err)
	}

	csrData, csrDataOk := csrResp["data"].(map[string]interface{})
	if !csrDataOk {
		return "", fmt.Errorf("intermediate CSR response missing data")
	}
	csr, csrOk := csrData["csr"].(string)
	if !csrOk || csr == "" {
		return "", fmt.Errorf("empty CSR from intermediate CA generation")
	}

	signResp, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/pki/root/sign-intermediate", token,
		map[string]interface{}{
			"csr":          csr,
			"format":       "pem_bundle",
			"ttl":          defaultIntPKITTL,
			"organization": "Netweave",
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to sign intermediate CA: %w", err)
	}

	signData, signDataOk := signResp["data"].(map[string]interface{})
	if !signDataOk {
		return "", fmt.Errorf("sign intermediate response missing data")
	}
	signedCert, signedOk := signData["certificate"].(string)
	if !signedOk || signedCert == "" {
		return "", fmt.Errorf("sign intermediate response missing certificate")
	}

	return signedCert, nil
}

func createPKIRoles(
	ctx context.Context,
	client *http.Client,
	vaultAddr, token string,
) error {
	if err := createPKIRole(ctx, client, vaultAddr, token,
		"netweave-client", false, true,
	); err != nil {
		return err
	}

	return createPKIRole(ctx, client, vaultAddr, token,
		"netweave-server", true, false,
	)
}

func enableSecretEngine(
	ctx context.Context,
	client *http.Client,
	vaultAddr, token, path, engineType string,
) error {
	_, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/sys/mounts/"+path, token,
		map[string]interface{}{"type": engineType},
	)
	if err != nil {
		if strings.Contains(err.Error(), "path is already in use") {
			return nil
		}
		return fmt.Errorf("failed to enable %s engine at %s: %w", engineType, path, err)
	}
	return nil
}

func createPKIRole(
	ctx context.Context,
	client *http.Client,
	vaultAddr, token, roleName string,
	serverFlag, clientFlag bool,
) error {
	if _, err := vaultAPIPut(ctx, client, vaultAddr,
		"/v1/pki_int/roles/"+roleName, token,
		map[string]interface{}{
			"allowed_domains":   []string{"netweave.local", "netweave.svc"},
			"allow_subdomains":  true,
			"allow_any_name":    true,
			"allow_ip_sans":     true,
			"max_ttl":           defaultCertTTL,
			"key_type":          "rsa",
			"key_bits":          2048,
			"server_flag":       serverFlag,
			"client_flag":       clientFlag,
			"require_cn":        true,
			"enforce_hostnames": false,
			"organization":      []string{"Netweave"},
			"no_store":          false,
			"generate_lease":    true,
		},
	); err != nil {
		return fmt.Errorf("failed to create PKI role %s: %w", roleName, err)
	}
	return nil
}

func saveCredentials(creds *vaultCredentials) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	dir := filepath.Join(home, credentialsDir)
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return fmt.Errorf("failed to create credentials directory: %w", mkErr)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	path := filepath.Join(dir, credentialsFile)
	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("failed to write credentials: %w", writeErr)
	}

	return nil
}
