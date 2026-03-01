package exec_test

import (
	"strings"
	"testing"

	xec "lantern/internal/exec"
)

func TestRealExecutor_Run(t *testing.T) {
	e := xec.NewRealExecutor()
	if err := e.Run("true"); err != nil {
		t.Fatalf("Run(true): %v", err)
	}
}

func TestRealExecutor_Run_Failure(t *testing.T) {
	e := xec.NewRealExecutor()
	if err := e.Run("false"); err == nil {
		t.Fatal("expected error from Run(false), got nil")
	}
}

func TestRealExecutor_Output(t *testing.T) {
	e := xec.NewRealExecutor()
	out, err := e.Output("echo", "hello")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Errorf("Output: got %q, want it to contain 'hello'", out)
	}
}

func TestDryRunExecutor_NeverRuns(t *testing.T) {
	var recorded []string
	e := xec.NewDryRunExecutor(func(cmd string) { recorded = append(recorded, cmd) })

	if err := e.Run("rm", "-rf", "/"); err != nil {
		t.Fatalf("DryRun.Run should not error: %v", err)
	}
	if len(recorded) != 1 {
		t.Fatalf("expected 1 recorded command, got %d", len(recorded))
	}
	if !strings.Contains(recorded[0], "rm") {
		t.Errorf("recorded command %q does not contain 'rm'", recorded[0])
	}
}

func TestDryRunExecutor_Output(t *testing.T) {
	e := xec.NewDryRunExecutor(func(string) {})
	out, err := e.Output("anything")
	if err != nil {
		t.Fatalf("DryRun.Output should not error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("DryRun.Output should return empty bytes, got %q", out)
	}
}
