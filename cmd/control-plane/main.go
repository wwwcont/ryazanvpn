package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/ryazanvpn/internal/app"
	"github.com/example/ryazanvpn/internal/infra/cache"
	"github.com/example/ryazanvpn/internal/infra/crypto"
	"github.com/example/ryazanvpn/internal/infra/db"
	"github.com/example/ryazanvpn/internal/infra/logging"
	pgrepo "github.com/example/ryazanvpn/internal/infra/repository/postgres"
	"github.com/example/ryazanvpn/internal/transport/httpcontrol"
)

func main() {
	cfg, err := app.LoadConfig("control-plane")
	if err != nil {
		slog.Error("load config", slog.Any("error", err))
		os.Exit(1)
	}

	logger := logging.NewJSONLogger(cfg.LogLevel)
	logger.Info("starting service", slog.String("config", cfg.String()))

	ctx := context.Background()
	pg, err := db.NewPool(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("postgres init failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer pg.Close()

	redisClient := cache.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer redisClient.Close()

	nodeRepo := pgrepo.NewNodeRepository(pg)
	userRepo := pgrepo.NewUserRepository(pg)
	deviceRepo := pgrepo.NewDeviceRepository(pg)
	inviteRepo := pgrepo.NewInviteCodeRepository(pg)
	auditRepo := pgrepo.NewAuditLogRepository(pg)

	var downloadUC *app.DownloadDeviceConfigByToken
	if cfg.ConfigMasterKey != "" {
		encryptor, err := crypto.NewAESGCMServiceFromBase64(cfg.ConfigMasterKey)
		if err != nil {
			logger.Error("invalid CONFIG_MASTER_KEY for AES-GCM", slog.Any("error", err))
			os.Exit(1)
		}
		downloadUC = &app.DownloadDeviceConfigByToken{
			Tokens:    pgrepo.NewConfigDownloadTokenRepository(pg),
			Accesses:  pgrepo.NewDeviceAccessRepository(pg),
			Encryptor: encryptor,
		}
	}

	router := httpcontrol.NewRouter(httpcontrol.Options{
		Logger:            logger,
		PG:                pg,
		RedisClient:       redisClient,
		ReadinessTimeout:  cfg.ReadinessTimeout,
		DownloadUC:        downloadUC,
		AdminSecret:       cfg.AdminSecret,
		AdminSecretHeader: cfg.AdminSecretHeader,
		Nodes:             nodeRepo,
		Users:             userRepo,
		Devices:           deviceRepo,
		InviteCodes:       inviteRepo,
		AuditLogs:         auditRepo,
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

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go app.NodeHealthWorker{
		Logger:       logger,
		Repo:         nodeRepo,
		PollInterval: cfg.NodeHealthPollInterval,
		Client: &http.Client{
			Timeout: cfg.NodeHealthCheckTimeout,
		},
	}.Run(workerCtx)

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

	workerCancel()

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
