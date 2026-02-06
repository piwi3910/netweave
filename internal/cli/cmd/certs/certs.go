// Package certs provides commands for certificate management.
package certs

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/service"
	"github.com/piwi3910/netweave/internal/vault"
)

const (
	vaultLabel       = "app=vault"
	vaultPort        = 8200
	vaultPKIRole     = "netweave-mtls"
	vaultPKIPath     = "pki_int"
	credentialsDir   = ".netweave"
	credentialsFile  = "credentials.json"
	defaultCertTTL   = "8760h"
	vaultReadyWait   = 120 * time.Second
	vaultHTTPTimeout = 30 * time.Second
)

// vaultCredentials holds Vault access credentials loaded from disk.
type vaultCredentials struct {
	VaultAddr  string   `json:"vault_addr"`
	RootToken  string   `json:"root_token"`
	UnsealKeys []string `json:"unseal_keys"`
}

// NewCertsCmd creates the parent "certs" command.
func NewCertsCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "certs",
		Short: "Manage TLS certificates",
		Long: `Certificate management commands for issuing and verifying TLS certificates
from the Vault PKI engine.`,
	}

	parent.PersistentFlags().String("vault-addr", "", "Vault address (default: auto-discover via K8s)")
	parent.PersistentFlags().String("vault-token", "", "Vault token (default: from ~/.netweave/credentials.json)")

	parent.AddCommand(newIssueCmd())
	parent.AddCommand(newVerifyCmd())

	return parent
}

func newIssueCmd() *cobra.Command {
	var (
		cn       string
		certType string
		ttl      string
		outDir   string
	)

	issueCmd := &cobra.Command{
		Use:   "issue",
		Short: "Issue a certificate from Vault PKI",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if cn == "" {
				return fmt.Errorf("--cn flag is required")
			}
			if certType != "server" && certType != "client" {
				return fmt.Errorf("--type must be 'server' or 'client'")
			}

			vaultClient, cleanup, err := connectVault(ctx, c)
			if err != nil {
				return err
			}
			defer cleanup()

			cert, err := vaultClient.IssueCertificate(ctx, vaultPKIRole, &vault.CertificateRequest{
				CommonName: cn,
				TTL:        ttl,
			})
			if err != nil {
				return fmt.Errorf("failed to issue certificate: %w", err)
			}

			if outDir != "" {
				if err := saveCertToDir(outDir, certType, cert); err != nil {
					return err
				}
				cmd.Printer.Infof("Certificate saved to %s/", outDir)
			}

			return cmd.Printer.PrintResult(cert, func() {
				cmd.Printer.Infof("Certificate issued:")
				cmd.Printer.Infof("  CN:      %s", cn)
				cmd.Printer.Infof("  Type:    %s", certType)
				cmd.Printer.Infof("  Serial:  %s", cert.SerialNumber)
				cmd.Printer.Infof("  Expires: %s", cert.Expiration)
				if outDir == "" {
					cmd.Printer.Infof("")
					cmd.Printer.Infof("Certificate PEM:")
					cmd.Printer.Infof("%s", cert.Certificate)
				}
			})
		},
	}

	issueCmd.Flags().StringVar(&cn, "cn", "", "Common name for the certificate (required)")
	issueCmd.Flags().StringVar(&certType, "type", "client", "Certificate type: server or client")
	issueCmd.Flags().StringVar(&ttl, "ttl", defaultCertTTL, "Certificate TTL (default: 1 year)")
	issueCmd.Flags().StringVar(&outDir, "out", "", "Output directory for cert/key files")
	return issueCmd
}

func newVerifyCmd() *cobra.Command {
	var (
		certFile string
		caFile   string
	)

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a certificate against the CA chain",
		RunE: func(c *cobra.Command, _ []string) error {
			if certFile == "" {
				return fmt.Errorf("--cert flag is required")
			}

			// If no CA file specified, try default.
			if caFile == "" {
				home, homeErr := os.UserHomeDir()
				if homeErr == nil {
					defaultCA := filepath.Join(home, credentialsDir, "ca.crt")
					if fileExists(defaultCA) {
						caFile = defaultCA
					}
				}
			}

			return verifyCertificate(c.Context(), certFile, caFile)
		},
	}

	verifyCmd.Flags().StringVar(&certFile, "cert", "", "Path to certificate file to verify (required)")
	verifyCmd.Flags().StringVar(&caFile, "ca", "", "Path to CA certificate file (default: ~/.netweave/ca.crt)")
	return verifyCmd
}

func connectVault(ctx context.Context, c *cobra.Command) (*vault.Client, func(), error) {
	vaultAddr := flagString(c, "vault-addr")
	vaultToken := flagString(c, "vault-token")

	// If explicit flags provided, use them directly.
	if vaultAddr != "" && vaultToken != "" {
		client, err := vault.NewClient(&vault.Config{
			Address: vaultAddr,
			Token:   vaultToken,
			PKIPath: vaultPKIPath,
			Timeout: vaultHTTPTimeout,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create vault client: %w", err)
		}
		return client, func() {}, nil
	}

	// Try loading credentials from disk.
	creds, err := loadCredentials()
	if err == nil && vaultToken == "" {
		vaultToken = creds.RootToken
	}

	// If we have a vault addr from creds and no explicit one, try it.
	if vaultAddr == "" && creds != nil {
		vaultAddr = creds.VaultAddr
	}

	// If still no addr, auto-discover via port-forward.
	if vaultAddr == "" {
		conn, connErr := service.NewConnector(cmd.Global.Kubeconfig, cmd.Global.Namespace)
		if connErr != nil {
			return nil, nil, fmt.Errorf("failed to create k8s connector: %w", connErr)
		}

		if waitErr := conn.WaitForPod(ctx, vaultLabel, vaultReadyWait); waitErr != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("vault pod not ready: %w", waitErr)
		}

		fwd, fwdErr := conn.PortForward(ctx, vaultLabel, vaultPort)
		if fwdErr != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("failed to port-forward to vault: %w", fwdErr)
		}

		vaultAddr = fmt.Sprintf("http://localhost:%d", fwd.LocalPort)

		client, clientErr := vault.NewClient(&vault.Config{
			Address: vaultAddr,
			Token:   vaultToken,
			PKIPath: vaultPKIPath,
			Timeout: vaultHTTPTimeout,
		})
		if clientErr != nil {
			close(fwd.StopChan)
			conn.Close()
			return nil, nil, fmt.Errorf("failed to create vault client: %w", clientErr)
		}

		cleanup := func() {
			close(fwd.StopChan)
			conn.Close()
		}
		return client, cleanup, nil
	}

	client, err := vault.NewClient(&vault.Config{
		Address: vaultAddr,
		Token:   vaultToken,
		PKIPath: vaultPKIPath,
		Timeout: vaultHTTPTimeout,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	return client, func() {}, nil
}

func loadCredentials() (*vaultCredentials, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	credPath := filepath.Join(home, credentialsDir, credentialsFile)

	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials: %w", err)
	}

	var creds vaultCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &creds, nil
}

func saveCertToDir(dir, certType string, cert *vault.Certificate) error {
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return fmt.Errorf("failed to create output directory: %w", mkErr)
	}

	certName := certType + ".crt"
	keyName := certType + ".key"

	if err := os.WriteFile(
		filepath.Join(dir, certName), []byte(cert.Certificate), 0o600,
	); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	if err := os.WriteFile(
		filepath.Join(dir, keyName), []byte(cert.PrivateKey), 0o600,
	); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	if len(cert.CAChain) > 0 {
		chain := strings.Join(cert.CAChain, "\n")
		if err := os.WriteFile(
			filepath.Join(dir, "ca.crt"), []byte(chain), 0o600,
		); err != nil {
			return fmt.Errorf("failed to write CA chain: %w", err)
		}
	}

	return nil
}

func verifyCertificate(_ context.Context, certFile, caFile string) error {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to decode PEM from %s", certFile)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	cmd.Printer.Infof("Certificate details:")
	cmd.Printer.Infof("  Subject:  %s", cert.Subject)
	cmd.Printer.Infof("  Issuer:   %s", cert.Issuer)
	cmd.Printer.Infof("  Serial:   %s", cert.SerialNumber)
	cmd.Printer.Infof("  NotAfter: %s", cert.NotAfter)

	if caFile == "" {
		cmd.Printer.Warnf("No CA file specified, cannot verify chain")
		return nil
	}

	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %w", err)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("failed to parse CA certificate from %s", caFile)
	}

	opts := x509.VerifyOptions{
		Roots: roots,
	}

	if _, err := cert.Verify(opts); err != nil {
		cmd.Printer.Warnf("Certificate verification FAILED: %v", err)
		return fmt.Errorf("certificate verification failed: %w", err)
	}

	cmd.Printer.Success("Certificate is valid and trusted by CA")
	return nil
}

// flagString safely reads a string flag, returning empty on error.
func flagString(c *cobra.Command, name string) string {
	val, err := c.Flags().GetString(name)
	if err != nil {
		return ""
	}
	return val
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
