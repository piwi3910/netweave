package setup

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/action"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/service"
)

func newTeardownCmd() *cobra.Command {
	var force bool

	teardownCmd := &cobra.Command{
		Use:   "teardown",
		Short: "Uninstall everything and clean up the namespace",
		Long: `Removes all Netweave components from the cluster:
  1. Uninstall Helm release
  2. Delete Vault deployment and resources
  3. Delete PersistentVolumeClaims
  4. Delete the namespace

This is a destructive operation. Use with caution.`,
		RunE: func(c *cobra.Command, _ []string) error {
			if !force {
				cmd.Printer.Warnf(
					"This will delete all Netweave resources in namespace %q.",
					cmd.Global.Namespace,
				)
				cmd.Printer.Warnf("Use --force to confirm.")
				return fmt.Errorf("teardown requires --force flag")
			}
			return runTeardown(c.Context())
		},
	}

	teardownCmd.Flags().BoolVar(
		&force, "force", false,
		"Confirm the destructive teardown operation",
	)

	return teardownCmd
}

func runTeardown(ctx context.Context) error {
	namespace := cmd.Global.Namespace
	steps := cmd.Printer.NewStepProgress(4)

	// Step 1: Uninstall Helm release.
	steps.Stepf("Uninstalling Helm release %q...", helmReleaseName)

	if err := helmUninstall(namespace); err != nil {
		cmd.Printer.Warnf("Helm uninstall: %v (continuing...)", err)
	} else {
		steps.Donef("Helm release uninstalled")
	}

	// Step 2: Delete Vault resources.
	steps.Stepf("Deleting Vault resources...")

	if err := deleteVaultResources(ctx, namespace); err != nil {
		cmd.Printer.Warnf("Vault cleanup: %v (continuing...)", err)
	} else {
		steps.Donef("Vault resources deleted")
	}

	// Step 3: Delete PVCs.
	steps.Stepf("Deleting PersistentVolumeClaims...")

	if err := deletePVCs(ctx, namespace); err != nil {
		cmd.Printer.Warnf("PVC cleanup: %v (continuing...)", err)
	} else {
		steps.Donef("PVCs deleted")
	}

	// Step 4: Delete namespace.
	steps.Stepf("Deleting namespace %q...", namespace)

	if err := deleteNamespace(ctx, namespace); err != nil {
		cmd.Printer.Warnf("Namespace cleanup: %v (continuing...)", err)
	} else {
		steps.Donef("Namespace %q deleted", namespace)
	}

	cmd.Printer.Success("Teardown complete")
	return nil
}

func helmUninstall(namespace string) error {
	cfg, err := newHelmConfig(namespace)
	if err != nil {
		return fmt.Errorf("failed to initialize helm config: %w", err)
	}

	uninstall := action.NewUninstall(cfg)
	_, err = uninstall.Run(helmReleaseName)
	if err != nil {
		return fmt.Errorf("failed to uninstall release: %w", err)
	}

	return nil
}

func deleteVaultResources(ctx context.Context, namespace string) error {
	conn, err := service.NewConnector(cmd.Global.Kubeconfig, namespace)
	if err != nil {
		return fmt.Errorf("failed to create k8s connector: %w", err)
	}
	defer conn.Close()

	clientset := conn.Clientset()

	// Delete Vault Deployment.
	err = clientset.AppsV1().Deployments(namespace).Delete(
		ctx, vaultDeploymentName, metav1.DeleteOptions{},
	)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete vault deployment: %w", err)
	}

	// Delete Vault Service.
	err = clientset.CoreV1().Services(namespace).Delete(
		ctx, vaultServiceName, metav1.DeleteOptions{},
	)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete vault service: %w", err)
	}

	// Delete Vault secrets.
	secrets := []string{"vault-tls", serverSecretName, caConfigMapName}
	for _, name := range secrets {
		err = clientset.CoreV1().Secrets(namespace).Delete(
			ctx, name, metav1.DeleteOptions{},
		)
		if err != nil && !apierrors.IsNotFound(err) {
			cmd.Printer.Verbosef("Failed to delete secret %q: %v", name, err)
		}
	}

	// Delete ConfigMaps.
	configMaps := []string{caConfigMapName, vaultConfigMapName}
	for _, name := range configMaps {
		err = clientset.CoreV1().ConfigMaps(namespace).Delete(
			ctx, name, metav1.DeleteOptions{},
		)
		if err != nil && !apierrors.IsNotFound(err) {
			cmd.Printer.Verbosef("Failed to delete configmap %q: %v", name, err)
		}
	}

	return nil
}

func deletePVCs(ctx context.Context, namespace string) error {
	conn, err := service.NewConnector(cmd.Global.Kubeconfig, namespace)
	if err != nil {
		return fmt.Errorf("failed to create k8s connector: %w", err)
	}
	defer conn.Close()

	pvcList, err := conn.Clientset().CoreV1().PersistentVolumeClaims(namespace).List(
		ctx, metav1.ListOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to list PVCs: %w", err)
	}

	for _, pvc := range pvcList.Items {
		if deleteErr := conn.Clientset().CoreV1().PersistentVolumeClaims(namespace).Delete(
			ctx, pvc.Name, metav1.DeleteOptions{},
		); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
			cmd.Printer.Verbosef("Failed to delete PVC %q: %v", pvc.Name, deleteErr)
		} else {
			cmd.Printer.Verbosef("Deleted PVC %q", pvc.Name)
		}
	}

	return nil
}

func deleteNamespace(ctx context.Context, namespace string) error {
	conn, err := service.NewConnector(cmd.Global.Kubeconfig, namespace)
	if err != nil {
		return fmt.Errorf("failed to create k8s connector: %w", err)
	}
	defer conn.Close()

	err = conn.Clientset().CoreV1().Namespaces().Delete(
		ctx, namespace, metav1.DeleteOptions{},
	)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	return nil
}
