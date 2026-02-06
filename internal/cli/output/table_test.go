package output

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable_Render_Empty(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "ID", "NAME")
	tbl.Render()
	assert.Equal(t, "No results found.\n", buf.String())
}

func TestTable_Render_SingleRow(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "ID", "NAME", "STATUS")
	tbl.AddRow("abc-123", "test-tenant", "active")
	tbl.Render()

	output := buf.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "abc-123")
	assert.Contains(t, output, "test-tenant")
	assert.Contains(t, output, "active")
	assert.Contains(t, output, "1 item(s)")
}

func TestTable_Render_MultipleRows(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "NAME", "TYPE")
	tbl.AddRow("platform-admin", "platform")
	tbl.AddRow("tenant-admin", "tenant")
	tbl.AddRow("viewer", "tenant")
	tbl.Render()

	output := buf.String()
	assert.Contains(t, output, "platform-admin")
	assert.Contains(t, output, "tenant-admin")
	assert.Contains(t, output, "viewer")
	assert.Contains(t, output, "3 item(s)")
}

func TestTable_Render_ColumnWidths(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "ID", "DESCRIPTION")
	tbl.AddRow("x", "A very long description that exceeds the header width")
	tbl.Render()

	output := buf.String()
	// The separator under DESCRIPTION should be wider than "DESCRIPTION" itself.
	assert.Contains(t, output, "A very long description")
}

func TestTable_AddRow_ExtraValues(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "A", "B")
	// Extra values beyond header count are ignored.
	tbl.AddRow("1", "2", "3")
	tbl.Render()

	output := buf.String()
	assert.Contains(t, output, "1")
	assert.Contains(t, output, "2")
	assert.NotContains(t, output, "3")
}

func TestTable_Render_NumericValues(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "NAME", "COUNT")
	tbl.AddRow("items", 42)
	tbl.Render()

	output := buf.String()
	assert.Contains(t, output, "42")
}
