package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootCmd(t *testing.T) {
	root := NewRootCmd()
	assert.Equal(t, "netweave-cli", root.Use)
	assert.NotEmpty(t, root.Short)
	assert.NotEmpty(t, root.Long)
}

func TestNewRootCmd_GlobalFlags(t *testing.T) {
	root := NewRootCmd()

	ns, err := root.PersistentFlags().GetString("namespace")
	require.NoError(t, err)
	assert.Equal(t, "netweave", ns)

	jsonFlag, err := root.PersistentFlags().GetBool("json")
	require.NoError(t, err)
	assert.False(t, jsonFlag)

	verbose, err := root.PersistentFlags().GetBool("verbose")
	require.NoError(t, err)
	assert.False(t, verbose)
}

func TestNewVersionCmd(t *testing.T) {
	// Set test values
	CLIVersion = "1.2.3"
	Commit = "abc1234"
	BuildTime = "2025-01-01_00:00:00"

	root := NewRootCmd()
	root.AddCommand(NewVersionCmd())

	// Test human output
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"version"})

	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "1.2.3")
	assert.Contains(t, buf.String(), "abc1234")
}

func TestNewVersionCmd_JSON(t *testing.T) {
	CLIVersion = "1.2.3"
	Commit = "abc1234"
	BuildTime = "2025-01-01_00:00:00"

	root := NewRootCmd()
	root.AddCommand(NewVersionCmd())

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"version", "--json"})

	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"version"`)
	assert.Contains(t, buf.String(), `"1.2.3"`)
}

func TestPrinterInitialized(t *testing.T) {
	root := NewRootCmd()
	root.AddCommand(NewVersionCmd())

	root.SetArgs([]string{"version"})
	err := root.Execute()
	require.NoError(t, err)
	assert.NotNil(t, Printer)
}
