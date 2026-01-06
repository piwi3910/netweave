// Command compliance runs O-RAN specification compliance validation.
//
// Usage:
//
//	compliance [flags]
//
// Flags:
//
//	-url string
//	    Gateway base URL (default "http://localhost:8080")
//	-output string
//	    Output format: text, json, badges (default "text")
//	-update-readme
//	    Update README.md with compliance badges
//
// Examples:
//
//	# Run compliance check against local gateway
//	compliance -url http://localhost:8080
//
//	# Generate JSON report
//	compliance -output json > compliance-report.json
//
//	# Generate badges for README
//	compliance -output badges
//
//	# Update README.md with compliance badges
//	compliance -update-readme
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/tools/compliance"
)

var (
	baseURL      = flag.String("url", "http://localhost:8080", "Gateway base URL")
	outputFormat = flag.String("output", "text", "Output format: text, json, badges")
	updateReadme = flag.Bool("update-readme", false, "Update README.md with compliance badges")
	readmePath   = flag.String("readme", "README.md", "Path to README.md file")
	verbose      = flag.Bool("v", false, "Verbose output")
)

func main() {
	flag.Parse()

	logger := initializeLogger()
	defer logger.Sync()

	// Create compliance checker and run checks
	checker := compliance.NewChecker(*baseURL, logger.Logger)
	ctx := context.Background()
	results, err := checker.CheckAll(ctx)
	if err != nil {
		logger.Logger.Error("compliance check failed", zap.Error(err))
		os.Exit(1)
	}

	// Generate output in requested format
	if err := generateOutput(results); err != nil {
		logger.Logger.Error("output generation failed", zap.Error(err))
		os.Exit(1)
	}

	// Update README if requested
	if *updateReadme {
		if err := updateReadmeFile(*readmePath, results, logger.Logger); err != nil {
			logger.Logger.Error("failed to update README", zap.Error(err))
			os.Exit(1)
		}
		logger.Logger.Info("README.md updated with compliance badges", zap.String("path", *readmePath))
	}

	// Exit with error if any spec is not compliant
	os.Exit(determineExitCode(results))
}

// initializeLogger initializes and configures the logger based on verbosity setting
func initializeLogger() *observability.Logger {
	obsLogger, err := observability.InitLogger("development")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Adjust log level based on verbosity
	if !*verbose {
		obsLogger.Logger = obsLogger.Logger.WithOptions(zap.IncreaseLevel(zap.InfoLevel))
	}

	return obsLogger
}

// generateOutput generates output in the requested format
func generateOutput(results []compliance.ComplianceResult) error {
	switch *outputFormat {
	case "json":
		outputJSON(results)
	case "badges":
		outputBadges(results)
	case "text":
		outputText(results)
	default:
		return fmt.Errorf("invalid output format: %s", *outputFormat)
	}
	return nil
}

// determineExitCode returns 1 if any spec is not compliant, 0 otherwise
func determineExitCode(results []compliance.ComplianceResult) int {
	for _, result := range results {
		if result.ComplianceLevel == compliance.ComplianceNone {
			return 1
		}
	}
	return 0
}

// outputJSON outputs results as JSON
func outputJSON(results []compliance.ComplianceResult) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
}

// outputBadges outputs badge markdown
func outputBadges(results []compliance.ComplianceResult) {
	generator := compliance.NewBadgeGenerator()
	badgeSection := generator.GenerateBadgeSection(results)
	fmt.Print(badgeSection)
}

// outputText outputs human-readable report
func outputText(results []compliance.ComplianceResult) {
	generator := compliance.NewBadgeGenerator()
	report := generator.GenerateComplianceReport(results)
	fmt.Print(report)
}

// updateReadmeFile updates README.md with compliance badge section
func updateReadmeFile(path string, results []compliance.ComplianceResult, logger *zap.Logger) error {
	// Read current README
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read README: %w", err)
	}

	// Generate badge section
	generator := compliance.NewBadgeGenerator()
	badgeSection := generator.GenerateBadgeSection(results)

	// Find and replace compliance section
	readme := string(content)

	// Markers for compliance section
	startMarker := "<!-- COMPLIANCE_BADGES_START -->"
	endMarker := "<!-- COMPLIANCE_BADGES_END -->"

	// Check if markers exist
	startIdx := strings.Index(readme, startMarker)
	endIdx := strings.Index(readme, endMarker)

	var newReadme string
	if startIdx != -1 && endIdx != -1 {
		// Replace existing section
		newReadme = readme[:startIdx+len(startMarker)] + "\n" + badgeSection + readme[endIdx:]
		logger.Info("replacing existing compliance section")
	} else {
		// Append new section after main header
		// Find first ## heading
		lines := strings.Split(readme, "\n")
		insertIdx := -1
		for i, line := range lines {
			if strings.HasPrefix(line, "## ") && i > 0 {
				insertIdx = i
				break
			}
		}

		if insertIdx == -1 {
			// Append at end
			newReadme = readme + "\n" + startMarker + "\n" + badgeSection + endMarker + "\n"
		} else {
			// Insert before first ## heading
			before := strings.Join(lines[:insertIdx], "\n")
			after := strings.Join(lines[insertIdx:], "\n")
			newReadme = before + "\n\n" + startMarker + "\n" + badgeSection + endMarker + "\n\n" + after
		}

		logger.Info("adding new compliance section")
	}

	// Write updated README
	if err := os.WriteFile(path, []byte(newReadme), 0644); err != nil {
		return fmt.Errorf("failed to write README: %w", err)
	}

	return nil
}
