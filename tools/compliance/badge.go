package compliance

import (
	"fmt"
	"strings"
)

// BadgeColor represents badge color based on compliance level.
type BadgeColor string

const (
	BadgeColorGreen  BadgeColor = "brightgreen" // Full compliance
	BadgeColorYellow BadgeColor = "yellow"      // Partial compliance
	BadgeColorRed    BadgeColor = "red"         // No compliance
	BadgeColorGray   BadgeColor = "lightgray"   // Unknown/untested
)

// BadgeGenerator generates compliance badges for README.
type BadgeGenerator struct{}

// NewBadgeGenerator creates a new badge generator.
func NewBadgeGenerator() *BadgeGenerator {
	return &BadgeGenerator{}
}

// GenerateBadge generates a shields.io badge URL for a compliance result.
func (g *BadgeGenerator) GenerateBadge(result ComplianceResult) string {
	// Determine badge color based on compliance level
	color := g.getColor(result.ComplianceLevel)

	// Generate badge label and message
	label := fmt.Sprintf("O-RAN %s", result.SpecName)
	message := g.getMessage(result)

	// Generate shields.io badge URL
	// Format: https://img.shields.io/badge/{label}-{message}-{color}
	badgeURL := fmt.Sprintf("https://img.shields.io/badge/%s-%s-%s",
		urlEncode(label),
		urlEncode(message),
		string(color))

	return badgeURL
}

// GenerateMarkdownBadge generates a markdown badge with link to spec.
func (g *BadgeGenerator) GenerateMarkdownBadge(result ComplianceResult) string {
	badgeURL := g.GenerateBadge(result)

	// Create markdown link: [![label](badge-url)](spec-url)
	markdown := fmt.Sprintf("[![O-RAN %s %s Compliance](%s)](%s)",
		result.SpecName,
		result.SpecVersion,
		badgeURL,
		result.SpecURL)

	return markdown
}

// GenerateBadgeSection generates a complete badge section for README.
func (g *BadgeGenerator) GenerateBadgeSection(results []ComplianceResult) string {
	var sb strings.Builder

	sb.WriteString("## O-RAN Specification Compliance\n\n")
	sb.WriteString("This project implements the following O-RAN Alliance specifications:\n\n")

	for _, result := range results {
		// Badge with link
		badge := g.GenerateMarkdownBadge(result)
		sb.WriteString(badge)
		sb.WriteString(" ")

		// Compliance details
		sb.WriteString(fmt.Sprintf("**%s %s**: %.1f%% compliant (%d/%d endpoints)\n\n",
			result.SpecName,
			result.SpecVersion,
			result.ComplianceScore,
			result.PassedEndpoints,
			result.TotalEndpoints))
	}

	// Add specification links
	sb.WriteString("### Specification References\n\n")
	sb.WriteString("Official O-RAN Alliance specifications:\n\n")

	for _, result := range results {
		sb.WriteString(fmt.Sprintf("- [%s %s Specification](%s)\n",
			result.SpecName,
			result.SpecVersion,
			result.SpecURL))
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("*Compliance verified on %s*\n\n",
		results[0].TestedAt.Format("2006-01-02")))

	return sb.String()
}

// getColor determines badge color based on compliance level.
func (g *BadgeGenerator) getColor(level ComplianceLevel) BadgeColor {
	switch level {
	case ComplianceFull:
		return BadgeColorGreen
	case CompliancePartial:
		return BadgeColorYellow
	case ComplianceNone:
		return BadgeColorRed
	default:
		return BadgeColorGray
	}
}

// getMessage generates badge message based on compliance result.
func (g *BadgeGenerator) getMessage(result ComplianceResult) string {
	switch result.ComplianceLevel {
	case ComplianceFull:
		return fmt.Sprintf("%s compliant", result.SpecVersion)
	case CompliancePartial:
		return fmt.Sprintf("%s %.0f%%", result.SpecVersion, result.ComplianceScore)
	case ComplianceNone:
		return "not compliant"
	default:
		return "unknown"
	}
}

// urlEncode encodes a string for use in URL (shields.io badge).
func urlEncode(s string) string {
	// Replace spaces with underscores for shields.io
	s = strings.ReplaceAll(s, " ", "_")

	// Replace hyphens with double hyphens (shields.io escaping)
	s = strings.ReplaceAll(s, "-", "--")

	// Replace underscores with double underscores (shields.io escaping)
	s = strings.ReplaceAll(s, "_", "__")

	return s
}

// GenerateComplianceReport generates a detailed text report.
func (g *BadgeGenerator) GenerateComplianceReport(results []ComplianceResult) string {
	var sb strings.Builder

	sb.WriteString("O-RAN Specification Compliance Report\n")
	sb.WriteString("=====================================\n\n")

	for _, result := range results {
		sb.WriteString(fmt.Sprintf("## %s %s\n\n", result.SpecName, result.SpecVersion))
		sb.WriteString(fmt.Sprintf("Specification URL: %s\n", result.SpecURL))
		sb.WriteString(fmt.Sprintf("Compliance Level: %s\n", result.ComplianceLevel))
		sb.WriteString(fmt.Sprintf("Compliance Score: %.1f%%\n", result.ComplianceScore))
		sb.WriteString(fmt.Sprintf("Endpoints Tested: %d\n", result.TotalEndpoints))
		sb.WriteString(fmt.Sprintf("Endpoints Passed: %d\n", result.PassedEndpoints))
		sb.WriteString(fmt.Sprintf("Endpoints Failed: %d\n", result.FailedEndpoints))
		sb.WriteString(fmt.Sprintf("Tested At: %s\n", result.TestedAt.Format("2006-01-02 15:04:05 UTC")))

		if len(result.MissingFeatures) > 0 {
			sb.WriteString("\nMissing Features:\n")
			for _, feature := range result.MissingFeatures {
				sb.WriteString(fmt.Sprintf("  - %s\n", feature))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}
