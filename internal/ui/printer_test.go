package ui_test

import (
	"bytes"
	"strings"
	"testing"

	"lantern/internal/ui"
)

// capturePrinter writes all output to a buffer via FilePrinter.
func capturePrinter(buf *bytes.Buffer) ui.Printer {
	return ui.NewFilePrinter(buf)
}

func TestFilePrinter_Info(t *testing.T) {
	var buf bytes.Buffer
	p := capturePrinter(&buf)
	p.Info("hello world")
	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("expected output to contain 'hello world', got: %q", buf.String())
	}
}

func TestFilePrinter_Warn(t *testing.T) {
	var buf bytes.Buffer
	p := capturePrinter(&buf)
	p.Warn("label", "detail")
	out := buf.String()
	if !strings.Contains(out, "label") || !strings.Contains(out, "detail") {
		t.Errorf("Warn output missing label or detail: %q", out)
	}
}

func TestFilePrinter_Success(t *testing.T) {
	var buf bytes.Buffer
	p := capturePrinter(&buf)
	p.Success("label", "detail")
	out := buf.String()
	if !strings.Contains(out, "label") || !strings.Contains(out, "detail") {
		t.Errorf("Success output missing label or detail: %q", out)
	}
}

func TestMultiPrinter_FansOut(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	p := ui.NewMultiPrinter(capturePrinter(&buf1), capturePrinter(&buf2))
	p.Info("broadcast")
	if !strings.Contains(buf1.String(), "broadcast") {
		t.Errorf("buf1 missing 'broadcast': %q", buf1.String())
	}
	if !strings.Contains(buf2.String(), "broadcast") {
		t.Errorf("buf2 missing 'broadcast': %q", buf2.String())
	}
}

func TestNullPrinter_NoOutput(t *testing.T) {
	// NullPrinter must not panic on any method call.
	p := ui.NewNullPrinter()
	p.Info("x")
	p.Warn("x", "y")
	p.Success("x", "y")
	p.Fatal("x")
}

func TestPrompt_ReturnsFalseOnNo(t *testing.T) {
	// TerminalPrinter.Prompt reads from stdin; test via a reader-injected variant.
	// Use a NullPrinter for now — Prompt always returns false on empty input.
	p := ui.NewReaderPrinter(strings.NewReader("n\n"), &bytes.Buffer{})
	if p.Prompt("continue?") {
		t.Error("expected Prompt to return false for 'n'")
	}
}

func TestPrompt_ReturnsTrueOnYes(t *testing.T) {
	p := ui.NewReaderPrinter(strings.NewReader("y\n"), &bytes.Buffer{})
	if !p.Prompt("continue?") {
		t.Error("expected Prompt to return true for 'y'")
	}
}
