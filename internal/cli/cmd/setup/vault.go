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
	credentialsDir      = ".netweave"
	credentialsFile     = "credentials.json"
	defaultPKITTL       = "87600h" // 10 years
	defaultIntPKITTL    = "43800h" // 5 years
	defaultCertTTL      = "8760h"  // 1 year
	vaultReadyTimeout   = 120 * time.Second
)

// vaultCredentials holds Vault access credentials saved to disk.
type vaultCredentials struct {
	VaultAddr  string   `json:"vault_addr"`
	RootToken  string   `json:"root_token"`
	UnsealKeys []string `json:"unseal_keys"`
}

func newVaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "vault",
		Short: "Deploy Vault, initialize, unseal, and configure PKI",
		Long: `Deploys HashiCorp Vault into the Kubernetes cluster, generates TLS
certificates for Vault itself, initializes and unseals the Vault instance,
and sets up the PKI secrets engine with Root CA, Intermediate CA, and the
netweave-mtls role for certificate issuance.`,
		RunE: runVaultSetup,
	}
}

func runVaultSetup(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	conn, err := service.NewConnector(cmd.Global.Kubeconfig, cmd.Global.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create K8s connector: %w", err)
	}
	defer conn.Close()

	steps := cmd.Printer.NewStepProgress(8)

	// Step 1: Create namespace
	steps.Stepf("Creating namespace %q...", cmd.Global.Namespace)
	if nsErr := ensureNamespace(ctx, conn); nsErr != nil {
		return nsErr
	}
	steps.Donef("Namespace ready")

	// Step 2: Generate Vault TLS certs
	steps.Stepf("Generating Vault TLS certificates...")
	tlsCert, tlsKey, caCert, genErr := generateSelfSignedTLS(
		"vault."+cmd.Global.Namespace+".svc",
	)
	if genErr != nil {
		return fmt.Errorf("failed to generate TLS: %w", genErr)
	}
	steps.Donef("TLS certificates generated")

	// Step 3: Create K8s secrets for Vault TLS
	steps.Stepf("Creating Vault TLS secrets...")
	if secErr := createVaultTLSSecrets(ctx, conn, tlsCert, tlsKey, caCert); secErr != nil {
		return secErr
	}
	steps.Donef("TLS secrets created")

	// Step 4: Deploy Vault
	steps.Stepf("Deploying Vault...")
	if depErr := deployVault(ctx, conn); depErr != nil {
		return depErr
	}
	steps.Donef("Vault deployed")

	// Step 5: Wait for Vault pod
	steps.Stepf("Waiting for Vault pod to be ready...")
	if waitErr := conn.WaitForPod(ctx, vaultLabel, vaultReadyTimeout); waitErr != nil {
		return fmt.Errorf("vault pod not ready: %w", waitErr)
	}
	steps.Donef("Vault pod running")

	// Step 6: Port-forward and initialize
	steps.Stepf("Initializing Vault...")
	fw, fwErr := conn.PortForward(ctx, vaultLabel, vaultContainerPort)
	if fwErr != nil {
		return fmt.Errorf("failed to port-forward to Vault: %w", fwErr)
	}
	defer close(fw.StopChan)

	vaultAddr := fmt.Sprintf("http://127.0.0.1:%d", fw.LocalPort)
	creds, initErr := initAndUnsealVault(ctx, vaultAddr)
	if initErr != nil {
		return initErr
	}
	steps.Donef("Vault initialized and unsealed")

	// Step 7: Setup PKI
	steps.Stepf("Setting up PKI secrets engine...")
	if pkiErr := setupVaultPKI(ctx, vaultAddr, creds.RootToken); pkiErr != nil {
		return pkiErr
	}
	steps.Donef("PKI configured with Root CA, Intermediate CA, and roles")

	// Step 8: Save credentials
	steps.Stepf("Saving credentials...")
	creds.VaultAddr = vaultAddr
	if saveErr := saveCredentials(creds); saveErr != nil {
		return saveErr
	}
	steps.Donef("Credentials saved to ~/%s/%s", credentialsDir, credentialsFile)

	cmd.Printer.Success("Vault setup complete!")
	return nil
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
	// Generate CA key
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

	// Generate server key
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal server key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	// Create server certificate
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

	// Create service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaultServiceName,
			Namespace: conn.Namespace(),
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
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
			Ports: []corev1.ContainerPort{{
				ContainerPort: int32(vaultContainerPort),
				Protocol:      corev1.ProtocolTCP,
			}},
			Env: []corev1.EnvVar{
				{Name: "VAULT_DEV_ROOT_TOKEN_ID", Value: ""},
				{Name: "VAULT_DEV_LISTEN_ADDRESS", Value: "0.0.0.0:8200"},
				{Name: "VAULT_API_ADDR", Value: "http://0.0.0.0:8200"},
			},
			Command: []string{"vault", "server", "-dev",
				"-dev-listen-address=0.0.0.0:8200"},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path:   "/v1/sys/health",
						Port:   intstr.FromInt32(int32(vaultContainerPort)),
						Scheme: corev1.URISchemeHTTP,
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
		}},
	}
}

// initAndUnsealVault initializes Vault and returns credentials.
// For dev-mode Vault, init/unseal is automatic; we just retrieve the token.
func initAndUnsealVault(
	ctx context.Context,
	vaultAddr string,
) (*vaultCredentials, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Check if Vault is already initialized
	initStatus, err := vaultAPIGet(ctx, client, vaultAddr, "/v1/sys/init", "")
	if err != nil {
		return nil, fmt.Errorf("failed to check Vault init status: %w", err)
	}

	initialized, ok := initStatus["initialized"].(bool)
	if ok && initialized {
		// Dev mode: root token is "root" by default unless configured
		cmd.Printer.Verbosef("Vault already initialized, using dev-mode token")
		return &vaultCredentials{
			VaultAddr:  vaultAddr,
			RootToken:  "root",
			UnsealKeys: []string{},
		}, nil
	}

	// Initialize Vault with 1 key share, 1 threshold (dev-like)
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

	// Unseal
	for _, key := range unsealKeys {
		unsealReq := map[string]interface{}{"key": key}
		if _, unsealErr := vaultAPIPut(
			ctx, client, vaultAddr, "/v1/sys/unseal", "", unsealReq,
		); unsealErr != nil {
			return nil, fmt.Errorf("failed to unseal Vault: %w", unsealErr)
		}
	}

	return &vaultCredentials{
		VaultAddr:  vaultAddr,
		RootToken:  rootToken,
		UnsealKeys: unsealKeys,
	}, nil
}

func setupVaultPKI(ctx context.Context, vaultAddr, token string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	if err := setupRootCA(ctx, client, vaultAddr, token); err != nil {
		return err
	}

	if err := setupIntermediateCA(ctx, client, vaultAddr, token); err != nil {
		return err
	}

	return createPKIRoles(ctx, client, vaultAddr, token)
}

func setupRootCA(
	ctx context.Context,
	client *http.Client,
	vaultAddr, token string,
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
			"issuing_certificates":    vaultAddr + "/v1/pki/ca",
			"crl_distribution_points": vaultAddr + "/v1/pki/crl",
		},
	); err != nil {
		return fmt.Errorf("failed to configure CA URLs: %w", err)
	}

	return nil
}

func setupIntermediateCA(
	ctx context.Context,
	client *http.Client,
	vaultAddr, token string,
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
			"issuing_certificates":    vaultAddr + "/v1/pki_int/ca",
			"crl_distribution_points": vaultAddr + "/v1/pki_int/crl",
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
		// Check if already enabled
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
			"allowed_domains":    []string{"netweave.local", "netweave.svc"},
			"allow_subdomains":   true,
			"allow_any_name":     true,
			"allow_ip_sans":      true,
			"max_ttl":            defaultCertTTL,
			"key_type":           "rsa",
			"key_bits":           2048,
			"server_flag":        serverFlag,
			"client_flag":        clientFlag,
			"require_cn":         true,
			"enforce_hostnames":  false,
			"organization":       []string{"Netweave"},
			"no_store":           false,
			"generate_lease":     true,
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

