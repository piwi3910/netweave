package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/output"
	"github.com/piwi3910/netweave/internal/cli/service"
)

const (
	helmReleaseName  = "netweave"
	helmDriver       = "secret"
	helmTimeout      = 5 * time.Minute
	podReadyTimeout  = 5 * time.Minute
	defaultChartPath = "deployments/helm/netweave"
	defaultValues    = "values-local.yaml"
)

func newHelmCmd() *cobra.Command {
	var (
		chartPath  string
		valuesFile string
		upgrade    bool
	)

	helmCmd := &cobra.Command{
		Use:   "helm",
		Short: "Install or upgrade the Netweave Helm chart",
		Long: `Installs or upgrades the Netweave Helm chart using the Helm SDK.
Waits for all pods (PostgreSQL, Redis, Keycloak, Gateway, Admin Portal) to
become ready and reports their status.`,
		RunE: func(c *cobra.Command, _ []string) error {
			return runHelmSetup(
				c.Context(),
				chartPath,
				valuesFile,
				upgrade,
			)
		},
	}

	helmCmd.Flags().StringVar(
		&chartPath, "chart-path", defaultChartPath,
		"Path to the Netweave Helm chart directory",
	)
	helmCmd.Flags().StringVar(
		&valuesFile, "values-file", "",
		"Path to a values override file (default: <chart-path>/values-local.yaml)",
	)
	helmCmd.Flags().BoolVar(
		&upgrade, "upgrade", false,
		"Upgrade an existing release instead of installing",
	)

	return helmCmd
}

func runHelmSetup(
	ctx context.Context,
	chartPath, valuesFile string,
	upgradeMode bool,
) error {
	namespace := cmd.Global.Namespace
	steps := cmd.Printer.NewStepProgress(4)

	// Step 1: Load Helm chart.
	steps.Stepf("Loading Helm chart from %s...", chartPath)

	chrt, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load Helm chart from %s: %w", chartPath, err)
	}

	steps.Donef("Chart loaded: %s v%s", chrt.Metadata.Name, chrt.Metadata.Version)

	// Step 2: Load values overrides.
	steps.Stepf("Loading values overrides...")

	vals, err := loadHelmValues(chartPath, valuesFile)
	if err != nil {
		return err
	}

	// Override vault.namespace to match CLI deployment namespace.
	// The CLI deploys Vault into the same namespace as the rest of the stack,
	// but values-local.yaml may have a different default (e.g. vault-system).
	applyVaultNamespaceOverride(vals, namespace)

	steps.Donef("Values loaded")

	// Step 3: Install or upgrade release.
	if upgradeMode {
		steps.Stepf("Upgrading Helm release %q...", helmReleaseName)

		if err := helmUpgrade(ctx, namespace, chrt, vals); err != nil {
			return err
		}

		steps.Donef(
			"Release %q upgraded in namespace %q",
			helmReleaseName, namespace,
		)
	} else {
		steps.Stepf("Installing Helm release %q...", helmReleaseName)

		if err := helmInstall(ctx, namespace, chrt, vals); err != nil {
			return err
		}

		steps.Donef(
			"Release %q installed in namespace %q",
			helmReleaseName, namespace,
		)
	}

	// Step 4: Wait for pods to be ready.
	steps.Stepf("Waiting for pods to be ready...")

	if err := waitForAllPods(ctx, namespace); err != nil {
		return err
	}

	steps.Donef("All pods ready")

	cmd.Printer.Success("Helm setup complete")
	return nil
}

func loadHelmValues(
	chartPath, valuesFile string,
) (map[string]interface{}, error) {
	if valuesFile == "" {
		valuesFile = filepath.Join(chartPath, defaultValues)
	}

	if _, err := os.Stat(valuesFile); os.IsNotExist(err) {
		cmd.Printer.Warnf(
			"No values file found at %s, using chart defaults",
			valuesFile,
		)
		return map[string]interface{}{}, nil
	}

	vals, err := chartutil.ReadValuesFile(valuesFile)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read values file %s: %w", valuesFile, err,
		)
	}

	return vals, nil
}

func newHelmConfig(namespace string) (*action.Configuration, error) {
	cfgFlags := genericclioptions.NewConfigFlags(true)
	if cmd.Global.Kubeconfig != "" {
		cfgFlags.KubeConfig = &cmd.Global.Kubeconfig
	}
	cfgFlags.Namespace = &namespace

	cfg := new(action.Configuration)
	debugLog := func(format string, v ...interface{}) {
		if cmd.Global.Verbose {
			cmd.Printer.Verbosef(format, v...)
		}
	}

	if err := cfg.Init(cfgFlags, namespace, helmDriver, debugLog); err != nil {
		return nil, fmt.Errorf(
			"failed to initialize Helm configuration: %w", err,
		)
	}

	return cfg, nil
}

func helmInstall(
	ctx context.Context,
	namespace string,
	chrt *chart.Chart,
	vals map[string]interface{},
) error {
	cfg, err := newHelmConfig(namespace)
	if err != nil {
		return err
	}

	install := action.NewInstall(cfg)
	install.ReleaseName = helmReleaseName
	install.Namespace = namespace
	install.CreateNamespace = true
	install.Wait = true
	install.Timeout = helmTimeout

	_, err = install.RunWithContext(ctx, chrt, vals)
	if err != nil {
		return fmt.Errorf("failed to install Helm chart: %w", err)
	}

	return nil
}

func helmUpgrade(
	ctx context.Context,
	namespace string,
	chrt *chart.Chart,
	vals map[string]interface{},
) error {
	cfg, err := newHelmConfig(namespace)
	if err != nil {
		return err
	}

	upg := action.NewUpgrade(cfg)
	upg.Namespace = namespace
	upg.Wait = true
	upg.Timeout = helmTimeout
	upg.Atomic = true

	_, err = upg.RunWithContext(ctx, helmReleaseName, chrt, vals)
	if err != nil {
		return fmt.Errorf("failed to upgrade Helm chart: %w", err)
	}

	return nil
}

func waitForAllPods(ctx context.Context, namespace string) error {
	conn, err := service.NewConnector(cmd.Global.Kubeconfig, namespace)
	if err != nil {
		return fmt.Errorf("failed to create k8s connector: %w", err)
	}
	defer conn.Close()

	labelSelectors := []struct {
		name     string
		selector string
	}{
		{"PostgreSQL", "app.kubernetes.io/component=postgresql"},
		{"Redis", "app.kubernetes.io/component=redis"},
		{"Keycloak", "app.kubernetes.io/component=keycloak"},
		{"Gateway", "app.kubernetes.io/component=gateway"},
	}

	for _, ls := range labelSelectors {
		cmd.Printer.Verbosef("Waiting for %s pods...", ls.name)

		if waitErr := conn.WaitForPod(ctx, ls.selector, podReadyTimeout); waitErr != nil {
			return fmt.Errorf("%s pods not ready: %w", ls.name, waitErr)
		}

		cmd.Printer.Verbosef("%s pod ready", ls.name)
	}

	// Print pod status summary.
	pods, err := conn.Clientset().CoreV1().Pods(namespace).List(
		ctx, metav1.ListOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	renderPodSummary(pods.Items)

	return nil
}

func renderPodSummary(pods []corev1.Pod) {
	tbl := output.NewTable(
		cmd.Printer.Output(), "NAME", "STATUS", "READY", "AGE",
	)

	for _, pod := range pods {
		ready := countReadyContainers(&pod)
		total := len(pod.Spec.Containers)
		age := time.Since(pod.CreationTimestamp.Time).Truncate(time.Second)

		tbl.AddRow(
			pod.Name,
			string(pod.Status.Phase),
			fmt.Sprintf("%d/%d", ready, total),
			age.String(),
		)
	}

	tbl.Render()
}

func applyVaultNamespaceOverride(vals map[string]interface{}, namespace string) {
	vaultVals, ok := vals["vault"].(map[string]interface{})
	if !ok {
		vaultVals = make(map[string]interface{})
		vals["vault"] = vaultVals
	}
	vaultVals["namespace"] = namespace
}

func countReadyContainers(pod *corev1.Pod) int {
	count := 0
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			count++
		}
	}
	return count
}
