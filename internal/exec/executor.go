package exec

import (
	"fmt"
	"os/exec"
	"strings"
)

// Executor abstracts command execution so setup logic is testable without
// real tools or root access.
type Executor interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

// --- RealExecutor ---

type RealExecutor struct{}

func NewRealExecutor() *RealExecutor { return &RealExecutor{} }

func (e *RealExecutor) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("running %q: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (e *RealExecutor) Output(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("running %q: %w", name, err)
	}
	return out, nil
}

// --- DryRunExecutor ---

// DryRunExecutor records commands without executing them. The record func
// is called with the full command string on every Run/Output call.
type DryRunExecutor struct {
	record func(cmd string)
}

func NewDryRunExecutor(record func(cmd string)) *DryRunExecutor {
	return &DryRunExecutor{record: record}
}

func (e *DryRunExecutor) Run(name string, args ...string) error {
	e.record(strings.Join(append([]string{name}, args...), " "))
	return nil
}

func (e *DryRunExecutor) Output(name string, args ...string) ([]byte, error) {
	e.record(strings.Join(append([]string{name}, args...), " "))
	return nil, nil
}
