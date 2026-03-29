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

	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/infra/cache"
	configrenderer "github.com/wwwcont/ryazanvpn/internal/infra/configrenderer"
	"github.com/wwwcont/ryazanvpn/internal/infra/crypto"
	"github.com/wwwcont/ryazanvpn/internal/infra/db"
	"github.com/wwwcont/ryazanvpn/internal/infra/logging"
	"github.com/wwwcont/ryazanvpn/internal/infra/nodeclient"
	pgrepo "github.com/wwwcont/ryazanvpn/internal/infra/repository/postgres"
	"github.com/wwwcont/ryazanvpn/internal/infra/telegram"
	"github.com/wwwcont/ryazanvpn/internal/infra/vpnkey"
	"github.com/wwwcont/ryazanvpn/internal/transport/httpcontrol"
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
	accessRepo := pgrepo.NewDeviceAccessRepository(pg)
	tokenRepo := pgrepo.NewConfigDownloadTokenRepository(pg)
	accessGrantRepo := pgrepo.NewAccessGrantRepository(pg)
	opRepo := pgrepo.NewNodeOperationRepository(pg)
	trafficRepo := pgrepo.NewTrafficRepository(pg)

	nodeClientCfg := nodeclient.Config{
		BaseURL:    cfg.NodeAgentBaseURL,
		Secret:     cfg.NodeAgentSecret,
		Timeout:    cfg.NodeAgentTimeout,
		MaxRetries: cfg.NodeAgentRetries,
	}
	nodeHTTPClient := nodeclient.New(nodeClientCfg)
	nodeAppClient := nodeclient.AppAdapter{Client: nodeHTTPClient, Config: nodeClientCfg}

	var downloadUC *app.DownloadDeviceConfigByToken
	var telegramWebhookHandler http.Handler
	if cfg.ConfigMasterKey != "" {
		encryptor, err := crypto.NewAESGCMServiceFromBase64(cfg.ConfigMasterKey)
		if err != nil {
			logger.Error("invalid CONFIG_MASTER_KEY for AES-GCM", slog.Any("error", err))
			os.Exit(1)
		}
		downloadUC = &app.DownloadDeviceConfigByToken{
			Tokens:    tokenRepo,
			Accesses:  accessRepo,
			Encryptor: encryptor,
		}

		if cfg.TelegramBotToken != "" {
			adminIDs := make(map[int64]struct{}, len(cfg.TelegramAdminIDs))
			for _, id := range cfg.TelegramAdminIDs {
				adminIDs[id] = struct{}{}
			}

			tgSvc := &telegram.TelegramService{
				Logger:           logger,
				Bot:              &telegram.HTTPBotClient{Token: cfg.TelegramBotToken},
				States:           telegram.RedisStateStore{Redis: redisClient, TTL: cfg.TelegramStateTTL},
				RegisterUC:       app.RegisterTelegramUser{Users: userRepo},
				ActivateInviteUC: app.ActivateInviteCode{Store: pgrepo.NewInviteActivationStore(pg)},
				GetGrantUC:       app.GetActiveAccessGrantByUser{AccessGrants: accessGrantRepo},
				CreateDeviceUC: app.CreateDeviceForUser{
					Devices:       deviceRepo,
					Nodes:         nodeRepo,
					Accesses:      accessRepo,
					Operations:    opRepo,
					AuditLogs:     auditRepo,
					KeyGenerator:  telegram.X25519KeyGenerator{},
					PresharedKeys: telegram.X25519KeyGenerator{},
					IPAllocator:   telegram.RedisIPAllocator{Redis: redisClient, SubnetCIDR: cfg.VPNSubnetCIDR},
					NodeAssigner:  app.MinLoadNodeAssigner{},
					CreatePeerExecutor: &app.ExecuteCreatePeerOperation{
						Operations:         opRepo,
						Accesses:           accessRepo,
						Nodes:              nodeRepo,
						NodeClient:         nodeAppClient,
						SensitiveEncryptor: encryptor,
					},
					ConfigIssuer: &app.IssueDeviceConfig{
						Accesses:  accessRepo,
						Tokens:    tokenRepo,
						Renderer:  configrenderer.NewAmneziaWGRenderer(),
						Encryptor: encryptor,
					},
					ServerPublicKey:  cfg.VPNServerPublicKey,
					PublicEndpoint:   cfg.VPNServerPublicEndpoint,
					ClientAllowedIPs: cfg.VPNClientAllowedIPs,
					AWG: app.DefaultVPNAWGFields{
						Jc:   cfg.VPNAWGJc,
						Jmin: cfg.VPNAWGJmin,
						Jmax: cfg.VPNAWGJmax,
						S1:   cfg.VPNAWGS1,
						S2:   cfg.VPNAWGS2,
						S3:   cfg.VPNAWGS3,
						S4:   cfg.VPNAWGS4,
						H1:   cfg.VPNAWGH1,
						H2:   cfg.VPNAWGH2,
						H3:   cfg.VPNAWGH3,
						H4:   cfg.VPNAWGH4,
						I1:   cfg.VPNAWGI1,
						I2:   cfg.VPNAWGI2,
						I3:   cfg.VPNAWGI3,
						I4:   cfg.VPNAWGI4,
						I5:   cfg.VPNAWGI5,
						MTU:  cfg.VPNAWGMTU,
					},
					SensitiveEncryptor: encryptor,
				},
				RevokeAccessUC:  app.RevokeDeviceAccess{Accesses: accessRepo, Operations: opRepo, AuditLogs: auditRepo, RevokePeerExecutor: &app.ExecuteRevokePeerOperation{Operations: opRepo, Accesses: accessRepo, Nodes: nodeRepo, NodeClient: nodeAppClient}},
				Users:           userRepo,
				Devices:         deviceRepo,
				Accesses:        accessRepo,
				Tokens:          tokenRepo,
				AccessGrants:    accessGrantRepo,
				InviteCodes:     inviteRepo,
				AuditLogs:       auditRepo,
				Nodes:           nodeRepo,
				Traffic:         trafficRepo,
				DownloadBaseURL: cfg.PublicBaseURL,
				AdminIDs:        adminIDs,
				VPNSubnetCIDR:   cfg.VPNSubnetCIDR,
				ConfigEncryptor: encryptor,
				VPNExporter:     vpnkey.NewDefaultVPNExporter(),
				DefaultVPNAWG: app.DefaultVPNAWGFields{
					Jc:   cfg.VPNAWGJc,
					Jmin: cfg.VPNAWGJmin,
					Jmax: cfg.VPNAWGJmax,
					S1:   cfg.VPNAWGS1,
					S2:   cfg.VPNAWGS2,
					S3:   cfg.VPNAWGS3,
					S4:   cfg.VPNAWGS4,
					H1:   cfg.VPNAWGH1,
					H2:   cfg.VPNAWGH2,
					H3:   cfg.VPNAWGH3,
					H4:   cfg.VPNAWGH4,
					I1:   cfg.VPNAWGI1,
					I2:   cfg.VPNAWGI2,
					I3:   cfg.VPNAWGI3,
					I4:   cfg.VPNAWGI4,
					I5:   cfg.VPNAWGI5,
					MTU:  cfg.VPNAWGMTU,
				},
			}
			telegramWebhookHandler = telegram.WebhookHandler{SecretToken: cfg.TelegramWebhookSecret, Service: tgSvc}
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
		TelegramWebhook:   telegramWebhookHandler,
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

	go app.TrafficCollectorWorker{
		Logger:        logger,
		Nodes:         nodeRepo,
		Accesses:      accessRepo,
		Traffic:       trafficRepo,
		ClientFactory: nodeclient.TrafficFactory{Secret: cfg.NodeAgentSecret, Timeout: cfg.NodeAgentTimeout, MaxRetries: cfg.NodeAgentRetries},
		PollInterval:  1 * time.Minute,
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
