package shell

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"time"
)

type OSExecutor struct {
	Logger *slog.Logger
}

func NewOSExecutor(logger *slog.Logger) *OSExecutor {
	return &OSExecutor{Logger: logger}
}

func (e *OSExecutor) Run(ctx context.Context, req ExecRequest) (ExecResult, error) {
	start := time.Now()
	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, req.Bin, req.Args...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	dur := time.Since(start)
	res := ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: -1,
		Duration: dur,
	}

	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}

	if e.Logger != nil {
		e.Logger.Info("shell command executed",
			slog.String("bin", req.Bin),
			slog.Any("args", req.Args),
			slog.String("work_dir", req.WorkDir),
			slog.Int("exit_code", res.ExitCode),
			slog.Duration("duration", res.Duration),
			slog.Any("error", err),
		)
	}

	return res, err
}
