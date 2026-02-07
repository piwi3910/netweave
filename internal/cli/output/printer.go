// Package output provides human-readable and machine-readable output formatting
// for the netweave CLI tool.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Printer handles output formatting for the CLI.
// It supports both human-readable and JSON output modes.
type Printer struct {
	out     io.Writer
	errOut  io.Writer
	jsonOut bool
	verbose bool
}

// NewPrinter creates a new Printer with the given options.
func NewPrinter(jsonOut, verbose bool) *Printer {
	return &Printer{
		out:     os.Stdout,
		errOut:  os.Stderr,
		jsonOut: jsonOut,
		verbose: verbose,
	}
}

// JSON returns a new Printer that outputs JSON. Used for testing.
func (p *Printer) JSON() *Printer {
	return &Printer{
		out:     p.out,
		errOut:  p.errOut,
		jsonOut: true,
		verbose: p.verbose,
	}
}

// IsJSON returns true if the printer is in JSON output mode.
func (p *Printer) IsJSON() bool {
	return p.jsonOut
}

// IsVerbose returns true if verbose output is enabled.
func (p *Printer) IsVerbose() bool {
	return p.verbose
}

// SetOutput sets the output writer. Used for testing.
func (p *Printer) SetOutput(w io.Writer) {
	p.out = w
}

// Output returns the current output writer.
func (p *Printer) Output() io.Writer {
	return p.out
}

// SetErrOutput sets the error output writer. Used for testing.
func (p *Printer) SetErrOutput(w io.Writer) {
	p.errOut = w
}

// PrintJSON outputs data as formatted JSON.
func (p *Printer) PrintJSON(data interface{}) error {
	encoder := json.NewEncoder(p.out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}

// PrintResult prints data as JSON if --json is set, otherwise uses the provided
// human-readable format function.
func (p *Printer) PrintResult(data interface{}, humanFn func()) error {
	if p.jsonOut {
		return p.PrintJSON(data)
	}
	humanFn()
	return nil
}

// Infof prints an informational message to stdout (suppressed in JSON mode).
func (p *Printer) Infof(format string, args ...interface{}) {
	if p.jsonOut {
		return
	}
	fmt.Fprintf(p.out, format+"\n", args...)
}

// Verbosef prints a verbose message (only when --verbose is set, suppressed in JSON mode).
func (p *Printer) Verbosef(format string, args ...interface{}) {
	if !p.verbose || p.jsonOut {
		return
	}
	fmt.Fprintf(p.out, format+"\n", args...)
}

// Errorf prints an error message to stderr.
func (p *Printer) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(p.errOut, "Error: "+format+"\n", args...)
}

// Warnf prints a warning message to stderr (suppressed in JSON mode).
func (p *Printer) Warnf(format string, args ...interface{}) {
	if p.jsonOut {
		return
	}
	fmt.Fprintf(p.errOut, "Warning: "+format+"\n", args...)
}

// StepProgress represents a multi-step operation with progress tracking.
type StepProgress struct {
	output     *Printer
	totalSteps int
	current    int
}

// NewStepProgress creates a new StepProgress tracker.
func (p *Printer) NewStepProgress(totalSteps int) *StepProgress {
	return &StepProgress{
		output:     p,
		totalSteps: totalSteps,
	}
}

// Stepf outputs a step progress message (e.g., "[1/14] Enabling PKI engine...").
func (s *StepProgress) Stepf(format string, args ...interface{}) {
	s.current++
	if s.output.jsonOut {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(s.output.out, "[%d/%d] %s\n", s.current, s.totalSteps, msg)
}

// Donef outputs a step completion message.
func (s *StepProgress) Donef(format string, args ...interface{}) {
	if s.output.jsonOut {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(s.output.out, "      %s\n", msg)
}

// Success prints a success message.
func (p *Printer) Success(msg string) {
	if p.jsonOut {
		return
	}
	fmt.Fprintf(p.out, "\n%s\n", msg)
}

// Separator prints a visual separator line.
func (p *Printer) Separator() {
	if p.jsonOut {
		return
	}
	fmt.Fprintln(p.out, strings.Repeat("-", 60))
}
