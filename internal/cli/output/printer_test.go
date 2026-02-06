package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPrinter(t *testing.T) {
	tests := []struct {
		name    string
		jsonOut bool
		verbose bool
	}{
		{name: "default", jsonOut: false, verbose: false},
		{name: "json mode", jsonOut: true, verbose: false},
		{name: "verbose mode", jsonOut: false, verbose: true},
		{name: "json and verbose", jsonOut: true, verbose: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPrinter(tt.jsonOut, tt.verbose)
			assert.Equal(t, tt.jsonOut, p.IsJSON())
			assert.Equal(t, tt.verbose, p.IsVerbose())
		})
	}
}

func TestPrinter_JSON(t *testing.T) {
	p := NewPrinter(false, false)
	assert.False(t, p.IsJSON())

	jp := p.JSON()
	assert.True(t, jp.IsJSON())
}

func TestPrinter_PrintJSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(true, false)
	p.SetOutput(&buf)

	data := map[string]string{"key": "value"}
	err := p.PrintJSON(data)
	require.NoError(t, err)

	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

func TestPrinter_PrintResult_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(true, false)
	p.SetOutput(&buf)

	data := map[string]int{"count": 42}
	humanCalled := false
	err := p.PrintResult(data, func() {
		humanCalled = true
	})

	require.NoError(t, err)
	assert.False(t, humanCalled)
	assert.Contains(t, buf.String(), "42")
}

func TestPrinter_PrintResult_Human(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(false, false)
	p.SetOutput(&buf)

	humanCalled := false
	err := p.PrintResult(nil, func() {
		humanCalled = true
	})

	require.NoError(t, err)
	assert.True(t, humanCalled)
}

func TestPrinter_Infof(t *testing.T) {
	tests := []struct {
		name     string
		jsonOut  bool
		expected string
	}{
		{name: "human mode prints", jsonOut: false, expected: "hello world\n"},
		{name: "json mode suppresses", jsonOut: true, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(tt.jsonOut, false)
			p.SetOutput(&buf)

			p.Infof("hello %s", "world")
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestPrinter_Verbosef(t *testing.T) {
	tests := []struct {
		name     string
		jsonOut  bool
		verbose  bool
		expected string
	}{
		{name: "verbose on, human mode", jsonOut: false, verbose: true, expected: "detail\n"},
		{name: "verbose off, human mode", jsonOut: false, verbose: false, expected: ""},
		{name: "verbose on, json mode", jsonOut: true, verbose: true, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(tt.jsonOut, tt.verbose)
			p.SetOutput(&buf)

			p.Verbosef("detail")
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestPrinter_Errorf(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(false, false)
	p.SetErrOutput(&buf)

	p.Errorf("something %s", "failed")
	assert.Equal(t, "Error: something failed\n", buf.String())
}

func TestPrinter_Warnf(t *testing.T) {
	tests := []struct {
		name     string
		jsonOut  bool
		expected string
	}{
		{name: "human mode prints", jsonOut: false, expected: "Warning: be careful\n"},
		{name: "json mode suppresses", jsonOut: true, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(tt.jsonOut, false)
			p.SetErrOutput(&buf)

			p.Warnf("be careful")
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestStepProgress(t *testing.T) {
	tests := []struct {
		name     string
		jsonOut  bool
		expected string
	}{
		{
			name:    "human mode shows steps",
			jsonOut: false,
			expected: "[1/3] Step one\n" +
				"      Done one\n" +
				"[2/3] Step two\n",
		},
		{
			name:     "json mode suppresses steps",
			jsonOut:  true,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewPrinter(tt.jsonOut, false)
			p.SetOutput(&buf)

			sp := p.NewStepProgress(3)
			sp.Stepf("Step %s", "one")
			sp.Donef("Done %s", "one")
			sp.Stepf("Step %s", "two")

			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestPrinter_Success(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(false, false)
	p.SetOutput(&buf)

	p.Success("All done!")
	assert.Equal(t, "\nAll done!\n", buf.String())
}

func TestPrinter_Success_JSONSuppressed(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(true, false)
	p.SetOutput(&buf)

	p.Success("All done!")
	assert.Empty(t, buf.String())
}

func TestPrinter_Separator(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(false, false)
	p.SetOutput(&buf)

	p.Separator()
	assert.Len(t, buf.String(), 61) // 60 dashes + newline
}
