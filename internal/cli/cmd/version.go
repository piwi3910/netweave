package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build-time variables, set via ldflags.
var (
	CLIVersion = "dev"
	Commit     = "unknown"
	BuildTime  = "unknown"
)

// versionInfo holds structured version information for JSON output.
type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// NewVersionCmd creates the version subcommand.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			info := versionInfo{
				Version:   CLIVersion,
				Commit:    Commit,
				BuildTime: BuildTime,
				GoVersion: runtime.Version(),
				Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			}

			return Printer.PrintResult(info, func() {
				Printer.Infof("netweave-cli %s", info.Version)
				Printer.Infof("  commit:    %s", info.Commit)
				Printer.Infof("  built:     %s", info.BuildTime)
				Printer.Infof("  go:        %s", info.GoVersion)
				Printer.Infof("  platform:  %s", info.Platform)
			})
		},
	}
}
