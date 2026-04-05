package shell

import (
	"context"
	"time"
)

// CommandExecutor executes a single command and returns collected output.
// It is intentionally generic so runtime adapters can be tested with fakes.
type CommandExecutor interface {
	Run(ctx context.Context, req ExecRequest) (ExecResult, error)
}

type ExecRequest struct {
	Bin     string
	Args    []string
	WorkDir string
	Timeout time.Duration
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}
