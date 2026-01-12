package compliance_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBadgeGenerator_GenerateBadge(t *testing.T) {
	generator := NewBadgeGenerator()

	tests := []struct {
		name            string
		result          Result
		expectedColor   BadgeColor
		expectedContain []string
	}{
		{
			name: "full compliance",
			result: Result{
				SpecName:        "O2-IMS",
				SpecVersion:     "v3.0.0",
				Level:           ComplianceFull,
				ComplianceScore: 100.0,
			},
			expectedColor: BadgeColorGreen,
			expectedContain: []string{
				"O--RAN__O2--IMS", // URL-encoded label: "O-RAN O2-IMS" → hyphens doubled, space to underscore, underscore doubled
				"v3.0.0",
				string(BadgeColorGreen),
			},
		},
		{
			name: "partial compliance",
			result: Result{
				SpecName:        "O2-DMS",
				SpecVersion:     "v3.0.0",
				Level:           CompliancePartial,
				ComplianceScore: 85.0,
			},
			expectedColor: BadgeColorYellow,
			expectedContain: []string{
				"O--RAN__O2--DMS",
				"v3.0.0",
				"85",
				string(BadgeColorYellow),
			},
		},
		{
			name: "no compliance",
			result: Result{
				SpecName:        "O2-SMO",
				SpecVersion:     "v3.0.0",
				Level:           ComplianceNone,
				ComplianceScore: 50.0,
			},
			expectedColor: BadgeColorRed,
			expectedContain: []string{
				"O--RAN__O2--SMO",
				string(BadgeColorRed),
				"not",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			badgeURL := generator.GenerateBadge(&tt.result)

			// Verify it's a valid shields.io URL
			assert.Contains(t, badgeURL, "https://img.shields.io/badge/")

			// Verify expected content
			for _, expected := range tt.expectedContain {
				assert.Contains(t, badgeURL, expected)
			}

			// Verify color
			assert.Contains(t, badgeURL, string(tt.expectedColor))
		})
	}
}

func TestBadgeGenerator_GenerateMarkdownBadge(t *testing.T) {
	generator := NewBadgeGenerator()

	result := Result{
		SpecName:        "O2-IMS",
		SpecVersion:     "v3.0.0",
		SpecURL:         "https://specifications.o-ran.org/o2ims",
		Level:           ComplianceFull,
		ComplianceScore: 100.0,
	}

	markdown := generator.GenerateMarkdownBadge(&result)

	// Verify markdown format
	assert.True(t, strings.HasPrefix(markdown, "[!["))
	assert.Contains(t, markdown, "](https://img.shields.io/badge/")
	assert.Contains(t, markdown, "](https://specifications.o-ran.org/o2ims)")
}

func TestBadgeGenerator_GenerateBadgeSection(t *testing.T) {
	generator := NewBadgeGenerator()

	results := []*Result{
		{
			SpecName:        "O2-IMS",
			SpecVersion:     "v3.0.0",
			SpecURL:         "https://specifications.o-ran.org/o2ims",
			Level:           ComplianceFull,
			ComplianceScore: 100.0,
			TotalEndpoints:  15,
			PassedEndpoints: 15,
			FailedEndpoints: 0,
			TestedAt:        time.Now(),
		},
		{
			SpecName:        "O2-DMS",
			SpecVersion:     "v3.0.0",
			SpecURL:         "https://specifications.o-ran.org/o2dms",
			Level:           ComplianceNone,
			ComplianceScore: 0.0,
			TotalEndpoints:  14,
			PassedEndpoints: 0,
			FailedEndpoints: 14,
			TestedAt:        time.Now(),
		},
	}

	section := generator.GenerateBadgeSection(results)

	// Verify section structure
	assert.Contains(t, section, "## O-RAN Specification Compliance")
	assert.Contains(t, section, "### Specification References")

	// Verify both specs are included
	assert.Contains(t, section, "O2-IMS")
	assert.Contains(t, section, "O2-DMS")

	// Verify spec links
	assert.Contains(t, section, "https://specifications.o-ran.org/o2ims")
	assert.Contains(t, section, "https://specifications.o-ran.org/o2dms")

	// Verify compliance details
	assert.Contains(t, section, "100.0% compliant (15/15 endpoints)")
	assert.Contains(t, section, "0.0% compliant (0/14 endpoints)")

	// Verify timestamp
	assert.Contains(t, section, "Compliance verified on")
}

func TestBadgeGenerator_GenerateComplianceReport(t *testing.T) {
	generator := NewBadgeGenerator()

	results := []*Result{
		{
			SpecName:        "O2-IMS",
			SpecVersion:     "v3.0.0",
			SpecURL:         "https://specifications.o-ran.org/o2ims",
			Level:           CompliancePartial,
			ComplianceScore: 93.3,
			TotalEndpoints:  15,
			PassedEndpoints: 14,
			FailedEndpoints: 1,
			MissingFeatures: []string{"POST /o2ims/v1/resourcePools"},
			TestedAt:        time.Now(),
		},
	}

	report := generator.GenerateComplianceReport(results)

	// Verify report structure
	assert.Contains(t, report, "O-RAN Specification Compliance Report")
	assert.Contains(t, report, "## O2-IMS v3.0.0")

	// Verify details
	assert.Contains(t, report, "Compliance Level: partial")
	assert.Contains(t, report, "Compliance Score: 93.3%")
	assert.Contains(t, report, "Endpoints Tested: 15")
	assert.Contains(t, report, "Endpoints Passed: 14")
	assert.Contains(t, report, "Endpoints Failed: 1")

	// Verify missing features section
	assert.Contains(t, report, "Missing Features:")
	assert.Contains(t, report, "POST /o2ims/v1/resourcePools")

	// Verify spec URL
	assert.Contains(t, report, "https://specifications.o-ran.org/o2ims")
}

func TestURLEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "spaces to underscores",
			input:    "O-RAN O2-IMS",
			expected: "O--RAN__O2--IMS",
		},
		{
			name:     "hyphens doubled",
			input:    "v3.0-beta",
			expected: "v3.0--beta",
		},
		{
			name:     "complex string",
			input:    "O-RAN O2-IMS v3.0",
			expected: "O--RAN__O2--IMS__v3.0",
		},
		{
			name:     "no special chars",
			input:    "O2IMS",
			expected: "O2IMS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := urlEncode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetColor(t *testing.T) {
	generator := NewBadgeGenerator()

	tests := []struct {
		name     string
		level    Level
		expected BadgeColor
	}{
		{
			name:     "full compliance = green",
			level:    ComplianceFull,
			expected: BadgeColorGreen,
		},
		{
			name:     "partial compliance = yellow",
			level:    CompliancePartial,
			expected: BadgeColorYellow,
		},
		{
			name:     "no compliance = red",
			level:    ComplianceNone,
			expected: BadgeColorRed,
		},
		{
			name:     "unknown = gray",
			level:    Level("unknown"),
			expected: BadgeColorGray,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color := generator.getColor(tt.level)
			assert.Equal(t, tt.expected, color)
		})
	}
}

func TestGetMessage(t *testing.T) {
	generator := NewBadgeGenerator()

	tests := []struct {
		name            string
		result          Result
		expectedContain []string
	}{
		{
			name: "full compliance message",
			result: Result{
				SpecVersion: "v3.0.0",
				Level:       ComplianceFull,
			},
			expectedContain: []string{"v3.0.0", "compliant"},
		},
		{
			name: "partial compliance message",
			result: Result{
				SpecVersion:     "v3.0.0",
				Level:           CompliancePartial,
				ComplianceScore: 85.5,
			},
			expectedContain: []string{"v3.0.0", "86"}, // Rounded
		},
		{
			name: "no compliance message",
			result: Result{
				Level: ComplianceNone,
			},
			expectedContain: []string{"not compliant"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := generator.getMessage(&tt.result)
			for _, expected := range tt.expectedContain {
				assert.Contains(t, message, expected)
			}
		})
	}
}
