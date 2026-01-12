package server_test

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

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
