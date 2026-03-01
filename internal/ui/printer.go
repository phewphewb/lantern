package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Printer is the output abstraction used by all internal packages.
// Internal packages never call fmt.Println directly.
type Printer interface {
	Info(msg string)
	Success(label, detail string)
	Warn(label, detail string)
	Fatal(msg string)
	Prompt(question string) bool
}

// --- TerminalPrinter ---

type TerminalPrinter struct {
	out io.Writer
	in  io.Reader
}

func NewTerminalPrinter() *TerminalPrinter {
	return &TerminalPrinter{out: os.Stdout, in: os.Stdin}
}

func (p *TerminalPrinter) Info(msg string)              { fmt.Fprintln(p.out, msg) }
func (p *TerminalPrinter) Success(label, detail string) { fmt.Fprintf(p.out, "  ✓ %-12s %s\n", label, detail) }
func (p *TerminalPrinter) Warn(label, detail string)    { fmt.Fprintf(p.out, "  ! %-12s %s\n", label, detail) }
func (p *TerminalPrinter) Fatal(msg string)             { fmt.Fprintln(p.out, "Error:", msg) }

func (p *TerminalPrinter) Prompt(question string) bool {
	fmt.Fprintf(p.out, "%s [y/N] ", question)
	return readYes(p.in)
}

// --- FilePrinter ---

// FilePrinter writes timestamped lines to any io.Writer (file, buffer, etc.).
type FilePrinter struct {
	w io.Writer
}

func NewFilePrinter(w io.Writer) *FilePrinter { return &FilePrinter{w: w} }

func (p *FilePrinter) ts() string { return time.Now().Format("2006-01-02 15:04:05") }

func (p *FilePrinter) Info(msg string)              { fmt.Fprintf(p.w, "%s  %s\n", p.ts(), msg) }
func (p *FilePrinter) Success(label, detail string) { fmt.Fprintf(p.w, "%s  ok  %-12s %s\n", p.ts(), label, detail) }
func (p *FilePrinter) Warn(label, detail string)    { fmt.Fprintf(p.w, "%s  WARN %-12s %s\n", p.ts(), label, detail) }
func (p *FilePrinter) Fatal(msg string)             { fmt.Fprintf(p.w, "%s  ERROR %s\n", p.ts(), msg) }
func (p *FilePrinter) Prompt(_ string) bool         { return false } // non-interactive

// --- ReaderPrinter (for testing Prompt) ---

// NewReaderPrinter returns a TerminalPrinter wired to the provided reader and
// writer — useful in tests to inject controlled stdin.
func NewReaderPrinter(in io.Reader, out io.Writer) *TerminalPrinter {
	return &TerminalPrinter{out: out, in: in}
}

// --- MultiPrinter ---

type MultiPrinter struct {
	printers []Printer
}

func NewMultiPrinter(ps ...Printer) *MultiPrinter { return &MultiPrinter{printers: ps} }

func (m *MultiPrinter) Info(msg string) {
	for _, p := range m.printers {
		p.Info(msg)
	}
}
func (m *MultiPrinter) Success(label, detail string) {
	for _, p := range m.printers {
		p.Success(label, detail)
	}
}
func (m *MultiPrinter) Warn(label, detail string) {
	for _, p := range m.printers {
		p.Warn(label, detail)
	}
}
func (m *MultiPrinter) Fatal(msg string) {
	for _, p := range m.printers {
		p.Fatal(msg)
	}
}
func (m *MultiPrinter) Prompt(question string) bool {
	// Delegate to first printer that can prompt; others are file-based.
	for _, p := range m.printers {
		if tp, ok := p.(*TerminalPrinter); ok {
			return tp.Prompt(question)
		}
	}
	return false
}

// --- NullPrinter ---

type NullPrinter struct{}

func NewNullPrinter() *NullPrinter              { return &NullPrinter{} }
func (p *NullPrinter) Info(_ string)            {}
func (p *NullPrinter) Success(_, _ string)      {}
func (p *NullPrinter) Warn(_, _ string)         {}
func (p *NullPrinter) Fatal(_ string)           {}
func (p *NullPrinter) Prompt(_ string) bool     { return false }

// --- helpers ---

func readYes(r io.Reader) bool {
	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}
