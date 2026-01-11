package server

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeResourceTypeID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple type",
			input:    "machine",
			expected: "machine",
		},
		{
			name:     "spaces to hyphens",
			input:    "compute node",
			expected: "compute-node",
		},
		{
			name:     "special characters",
			input:    "gpu/accelerator",
			expected: "gpu-accelerator",
		},
		{
			name:     "path traversal attempt",
			input:    "../../../etc/passwd",
			expected: "------etc-passwd",
		},
		{
			name:     "mixed case to lowercase",
			input:    "GPU-Server",
			expected: "gpu-server",
		},
		{
			name:     "preserve underscores",
			input:    "physical_server",
			expected: "physical_server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeResourceTypeID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeForLogging(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string",
			input:    "normal log entry",
			expected: "normal log entry",
		},
		{
			name:     "CRLF injection attempt",
			input:    "user input\nINFO admin password: stolen",
			expected: "user inputINFO admin password: stolen",
		},
		{
			name:     "carriage return removal",
			input:    "line1\rline2",
			expected: "line1line2",
		},
		{
			name:     "tab to space",
			input:    "col1\tcol2",
			expected: "col1 col2",
		},
		{
			name:     "multiple CRLF",
			input:    "data\n\rmore\r\ndata",
			expected: "datamoredata",
		},
		{
			name:     "control characters",
			input:    "test\x00\x01\x02data",
			expected: "testdata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeForLogging(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
