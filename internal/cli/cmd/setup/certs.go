package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/service"
	"github.com/piwi3910/netweave/internal/vault"
)

const (
	serverPKIRole    = "netweave-server"
	clientPKIRole    = "netweave-client"
	serverCertCN     = "netweave-gateway"
	clientCertCN     = "admin.netweave.local"
	serverSecretName = "netweave-tls-server"
	caConfigMapName  = "netweave-tls-ca"
	certTTL          = "8760h" // 1 year
	setupCertSteps   = 6
)

// ingressCert defines a TLS certificate to issue for an ingress host.
type ingressCert struct {
	hostname   string
	secretName string
}

// ingressCerts lists the ingress hosts that need Vault-issued TLS certificates.
var ingressCerts = []ingressCert{
	{hostname: "admin.netweave.local", secretName: "admin-portal-ingress-tls"},
	{hostname: "auth.netweave.local", secretName: "keycloak-ingress-tls"},
	{hostname: "api.netweave.local", secretName: "gateway-admin-ingress-tls"},
	{hostname: "tmf.netweave.local", secretName: "gateway-tmf-ingress-tls"},
	{hostname: "graphql.netweave.local", secretName: "gateway-graphql-ingress-tls"},
}

func newCertsCmd() *cobra.Command {
	var (
		serverCN string
		clientCN string
	)

	certsCmd := &cobra.Command{
		Use:   "certs",
		Short: "Issue gateway server and client certificates",
		Long: `Issues server and client TLS certificates from the Vault PKI engine
and creates the corresponding Kubernetes TLS secrets for the gateway.
Also issues individual TLS certificates for ingress hosts (admin portal, Keycloak)
so NGINX can terminate TLS with Vault-signed certificates.`,
		RunE: func(c *cobra.Command, _ []string) error {
			return runCertsSetup(c.Context(), serverCN, clientCN)
		},
	}

	certsCmd.Flags().StringVar(
		&serverCN, "server-cn", serverCertCN,
		"Common name for the server certificate",
	)
	certsCmd.Flags().StringVar(
		&clientCN, "client-cn", clientCertCN,
		"Common name for the client certificate",
	)

	return certsCmd
}

func runCertsSetup(ctx context.Context, serverCN, clientCN string) error {
	namespace := cmd.Global.Namespace
	steps := cmd.Printer.NewStepProgress(setupCertSteps)

	// Step 1: Load Vault credentials and connect.
	steps.Stepf("Connecting to Vault...")

	creds, err := loadVaultCredentials()
	if err != nil {
		return fmt.Errorf("failed to load vault credentials: %w", err)
	}

	conn, err := service.NewConnector(cmd.Global.Kubeconfig, namespace)
	if err != nil {
		return fmt.Errorf("failed to create k8s connector: %w", err)
	}
	defer conn.Close()

	fwd, err := conn.PortForward(ctx, vaultLabel, vaultContainerPort)
	if err != nil {
		return fmt.Errorf("failed to port-forward to vault: %w", err)
	}
	defer close(fwd.StopChan)

	vaultAddr := fmt.Sprintf("https://localhost:%d", fwd.LocalPort)
	vaultClient, err := vault.NewClient(&vault.Config{
		Address:    vaultAddr,
		Token:      creds.RootToken,
		PKIPath:    "pki_int",
		Timeout:    30 * time.Second,
		HTTPClient: service.NewInsecureTLSClient(30 * time.Second),
	})
	if err != nil {
		return fmt.Errorf("failed to create vault client: %w", err)
	}

	steps.Donef("Connected to Vault at %s", vaultAddr)

	// Step 2: Issue server certificate.
	steps.Stepf("Issuing server certificate (CN=%s)...", serverCN)

	serverCert, err := vaultClient.IssueCertificate(
		ctx, serverPKIRole, &vault.CertificateRequest{
			CommonName: serverCN,
			AltNames: []string{
				serverCN,
				fmt.Sprintf("%s.%s.svc", serverCN, namespace),
				fmt.Sprintf("%s.%s.svc.cluster.local", serverCN, namespace),
				"localhost",
				"api.netweave.local",
				"o2.netweave.local",
				"tmf.netweave.local",
				"graphql.netweave.local",
			},
			IPSANs: []string{"127.0.0.1"},
			TTL:    certTTL,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to issue server certificate: %w", err)
	}

	steps.Donef("Server certificate issued (serial: %s)", serverCert.SerialNumber)

	// Step 3: Create K8s TLS secret for server cert.
	steps.Stepf("Creating Kubernetes TLS secret %q...", serverSecretName)

	if err := createTLSSecret(
		ctx, conn, namespace, serverSecretName,
		serverCert.Certificate, serverCert.PrivateKey,
	); err != nil {
		return fmt.Errorf("failed to create server TLS secret: %w", err)
	}

	steps.Donef("TLS secret %q created", serverSecretName)

	// Step 4: Create CA ConfigMap.
	steps.Stepf("Creating CA ConfigMap %q...", caConfigMapName)

	caChain, err := vaultClient.GetCAChain(ctx)
	if err != nil {
		return fmt.Errorf("failed to get CA chain: %w", err)
	}

	if err := createCAConfigMap(ctx, conn, namespace, caChain); err != nil {
		return fmt.Errorf("failed to create CA configmap: %w", err)
	}

	steps.Donef("CA ConfigMap %q created", caConfigMapName)

	// Step 5: Issue ingress TLS certificates.
	steps.Stepf("Issuing ingress TLS certificates...")

	for _, ic := range ingressCerts {
		cert, issueErr := vaultClient.IssueCertificate(
			ctx, serverPKIRole, &vault.CertificateRequest{
				CommonName: ic.hostname,
				AltNames:   []string{ic.hostname},
				TTL:        certTTL,
			},
		)
		if issueErr != nil {
			return fmt.Errorf("failed to issue certificate for %s: %w", ic.hostname, issueErr)
		}

		if secretErr := createTLSSecret(
			ctx, conn, namespace, ic.secretName,
			cert.Certificate, cert.PrivateKey,
		); secretErr != nil {
			return fmt.Errorf("failed to create TLS secret %s: %w", ic.secretName, secretErr)
		}

		cmd.Printer.Verbosef("  %s -> secret %q", ic.hostname, ic.secretName)
	}

	steps.Donef("Ingress TLS secrets created (%d certificates)", len(ingressCerts))

	// Step 6: Issue client certificate and save locally.
	steps.Stepf("Issuing client certificate (CN=%s)...", clientCN)

	clientCert, err := vaultClient.IssueCertificate(
		ctx, clientPKIRole, &vault.CertificateRequest{
			CommonName: clientCN,
			TTL:        certTTL,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to issue client certificate: %w", err)
	}

	if err := saveClientCert(clientCert, caChain); err != nil {
		return fmt.Errorf("failed to save client certificate: %w", err)
	}

	steps.Donef("Client certificate saved to ~/%s/", credentialsDir)

	cmd.Printer.Success("Certificate setup complete")
	cmd.Printer.Infof("To trust the CA in your browser, import ~/%s/ca.crt", credentialsDir)
	return nil
}

func loadVaultCredentials() (*vaultCredentials, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	credPath := filepath.Join(home, credentialsDir, credentialsFile)

	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read credentials from %s: %w", credPath, err,
		)
	}

	var creds vaultCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &creds, nil
}

func createTLSSecret(
	ctx context.Context,
	conn *service.Connector,
	namespace, name, cert, key string,
) error {
	clientset := conn.Clientset()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		StringData: map[string]string{
			"tls.crt": cert,
			"tls.key": key,
		},
	}

	_, err := clientset.CoreV1().Secrets(namespace).Create(
		ctx, secret, metav1.CreateOptions{},
	)
	if apierrors.IsAlreadyExists(err) {
		_, err = clientset.CoreV1().Secrets(namespace).Update(
			ctx, secret, metav1.UpdateOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}
		cmd.Printer.Verbosef("Secret %q updated (already existed)", name)
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

func createCAConfigMap(
	ctx context.Context,
	conn *service.Connector,
	namespace, caChain string,
) error {
	clientset := conn.Clientset()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caConfigMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"ca.crt": caChain,
		},
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(
		ctx, cm, metav1.CreateOptions{},
	)
	if apierrors.IsAlreadyExists(err) {
		_, err = clientset.CoreV1().ConfigMaps(namespace).Update(
			ctx, cm, metav1.UpdateOptions{},
		)
		if err != nil {
			return fmt.Errorf("failed to update configmap: %w", err)
		}
		cmd.Printer.Verbosef("ConfigMap %q updated (already existed)", caConfigMapName)
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to create configmap: %w", err)
	}

	return nil
}

func saveClientCert(cert *vault.Certificate, caChain string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	certDir := filepath.Join(home, credentialsDir)

	files := map[string]string{
		"client.crt": cert.Certificate,
		"client.key": cert.PrivateKey,
		"ca.crt":     caChain,
	}

	for name, content := range files {
		path := filepath.Join(certDir, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}

	return nil
}
