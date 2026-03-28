package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/runtime"
	"github.com/wwwcont/ryazanvpn/internal/agent/shell"
	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/infra/logging"
	"github.com/wwwcont/ryazanvpn/internal/transport/httpnode"
)

func main() {
	cfg, err := app.LoadConfig("node-agent")
	if err != nil {
		slog.Error("load config", slog.Any("error", err))
		os.Exit(1)
	}
	if cfg.AgentHMACSecret == "" {
		slog.Error("AGENT_HMAC_SECRET is required for node-agent")
		os.Exit(1)
	}

	logger := logging.NewJSONLogger(cfg.LogLevel)
	logger.Info("starting service", slog.String("config", cfg.String()))

	vpnRuntime, err := buildRuntime(cfg, logger)
	if err != nil {
		logger.Error("runtime init failed", slog.Any("error", err))
		os.Exit(1)
	}

	router := httpnode.NewRouter(httpnode.Options{
		Logger:           logger,
		ReadinessTimeout: cfg.ReadinessTimeout,
		Runtime:          vpnRuntime,
		HMACSecret:       cfg.AgentHMACSecret,
		HMACMaxSkew:      cfg.AgentHMACMaxSkew,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", cfg.HTTPAddr))
		errCh <- srv.ListenAndServe()
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-sigCtx.Done():
		logger.Info("shutdown signal received")
	case err = <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server exited", slog.Any("error", err))
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.Any("error", err))
		if closeErr := srv.Close(); closeErr != nil {
			logger.Error("force close failed", slog.Any("error", closeErr))
		}
	}

	logger.Info("service stopped")
}

func buildRuntime(cfg app.Config, logger *slog.Logger) (runtime.VPNRuntime, error) {
	adapter := strings.ToLower(strings.TrimSpace(cfg.RuntimeAdapter))
	if adapter == "shell" {
		logger.Info("runtime adapter selected", slog.String("adapter", "shell"))
		rt := runtime.NewShellRuntime(logger, runtime.ShellRuntimeConfig{
			WorkDir:         cfg.RuntimeWorkDir,
			AWGBinaryPath:   cfg.AWGBinaryPath,
			WGBinaryPath:    cfg.WGBinaryPath,
			IPBinaryPath:    cfg.IPBinaryPath,
			StatsBinaryPath: cfg.RuntimeStatsBinaryPath,
			StatsArgs:       cfg.RuntimeStatsArgs,
			CommandTimeout:  cfg.RuntimeExecTimeout,
		}, shell.NewOSExecutor(logger))

		healthCtx, cancel := context.WithTimeout(context.Background(), cfg.RuntimeExecTimeout)
		defer cancel()
		if err := rt.Health(healthCtx); err != nil {
			return nil, err
		}
		return rt, nil
	}
	if adapter == "mock" || adapter == "" {
		logger.Info("runtime adapter selected", slog.String("adapter", "mock"))
		return runtime.NewMockRuntime(logger), nil
	}

	return nil, errors.New("unsupported runtime adapter: " + adapter)
}
