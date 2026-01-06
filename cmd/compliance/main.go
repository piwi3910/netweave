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

	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/tools/compliance"
	"go.uber.org/zap"
)

var (
	baseURL       = flag.String("url", "http://localhost:8080", "Gateway base URL")
	outputFormat  = flag.String("output", "text", "Output format: text, json, badges")
	updateReadme  = flag.Bool("update-readme", false, "Update README.md with compliance badges")
	readmePath    = flag.String("readme", "README.md", "Path to README.md file")
	verbose       = flag.Bool("v", false, "Verbose output")
)

func main() {
	flag.Parse()

	// Initialize logger
	logger, err := observability.InitLogger("development")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Set log level
	if !*verbose {
		logger = logger.WithOptions(zap.IncreaseLevel(zap.InfoLevel))
	}

	// Create compliance checker
	checker := compliance.NewChecker(*baseURL, logger)

	// Run compliance checks
	ctx := context.Background()
	results, err := checker.CheckAll(ctx)
	if err != nil {
		logger.Error("compliance check failed", zap.Error(err))
		os.Exit(1)
	}

	// Generate output based on format
	switch *outputFormat {
	case "json":
		outputJSON(results)
	case "badges":
		outputBadges(results)
	case "text":
		outputText(results)
	default:
		logger.Error("invalid output format", zap.String("format", *outputFormat))
		os.Exit(1)
	}

	// Update README if requested
	if *updateReadme {
		if err := updateReadmeFile(*readmePath, results, logger); err != nil {
			logger.Error("failed to update README", zap.Error(err))
			os.Exit(1)
		}
		logger.Info("README.md updated with compliance badges", zap.String("path", *readmePath))
	}

	// Exit with error if any spec is not compliant
	for _, result := range results {
		if result.ComplianceLevel == compliance.ComplianceNone {
			os.Exit(1)
		}
	}
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
