package output

import (
	"fmt"
	"io"
	"strings"
)

// Table formats data in a columnar table layout.
type Table struct {
	out     io.Writer
	headers []string
	rows    [][]string
	widths  []int
}

// NewTable creates a new Table with the given headers.
func NewTable(out io.Writer, headers ...string) *Table {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	return &Table{
		out:     out,
		headers: headers,
		widths:  widths,
	}
}

// AddRow adds a row of values to the table.
// Values are converted to strings via fmt.Sprintf.
func (t *Table) AddRow(values ...interface{}) {
	row := make([]string, len(t.headers))
	for i, v := range values {
		if i >= len(t.headers) {
			break
		}
		s := fmt.Sprintf("%v", v)
		row[i] = s
		if len(s) > t.widths[i] {
			t.widths[i] = len(s)
		}
	}
	t.rows = append(t.rows, row)
}

// Render writes the formatted table to the output writer.
func (t *Table) Render() {
	if len(t.rows) == 0 {
		fmt.Fprintln(t.out, "No results found.")
		return
	}

	// Build format string
	formats := make([]string, len(t.headers))
	for i, w := range t.widths {
		formats[i] = fmt.Sprintf("%%-%ds", w)
	}
	fmtStr := strings.Join(formats, "  ")

	// Print header
	headerArgs := make([]interface{}, len(t.headers))
	for i, h := range t.headers {
		headerArgs[i] = h
	}
	fmt.Fprintf(t.out, fmtStr+"\n", headerArgs...)

	// Print separator
	sepParts := make([]string, len(t.widths))
	for i, w := range t.widths {
		sepParts[i] = strings.Repeat("-", w)
	}
	fmt.Fprintln(t.out, strings.Join(sepParts, "  "))

	// Print rows
	for _, row := range t.rows {
		rowArgs := make([]interface{}, len(row))
		for i, v := range row {
			rowArgs[i] = v
		}
		fmt.Fprintf(t.out, fmtStr+"\n", rowArgs...)
	}

	// Print count
	fmt.Fprintf(t.out, "\n%d item(s)\n", len(t.rows))
}
