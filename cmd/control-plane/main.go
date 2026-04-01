package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	financeSvc := &app.FinanceService{PG: pg}
	ensureSingleNode(ctx, logger, pg, cfg)

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
				ActivateInviteUC: app.ActivateInviteCode{Store: pgrepo.NewInviteActivationStore(pg), Finance: financeSvc},
				GetGrantUC:       app.GetActiveAccessGrantByUser{AccessGrants: accessGrantRepo},
				CreateDeviceUC: app.CreateDeviceForUser{
					Users:         userRepo,
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
					MTU:              cfg.VPNAWGMTU,
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
					},
					SensitiveEncryptor: encryptor,
				},
				RevokeAccessUC:  app.RevokeDeviceAccess{Accesses: accessRepo, Devices: deviceRepo, Operations: opRepo, AuditLogs: auditRepo, Tokens: tokenRepo, RevokePeerExecutor: &app.ExecuteRevokePeerOperation{Operations: opRepo, Accesses: accessRepo, Nodes: nodeRepo, NodeClient: nodeAppClient}},
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
				ConfigEncryptor: encryptor,
				VPNExporter:     vpnkey.NewDefaultVPNExporter(),
				Finance:         financeSvc,
				DefaultVPNMTU:   cfg.VPNAWGMTU,
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
		AgentHMACSecret:   cfg.AgentHMACSecret,
		NodeRegisterToken: cfg.NodeRegistrationToken,
		Finance:           financeSvc,
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

	go app.PeerConsistencyWorker{
		Logger:        logger,
		Nodes:         nodeRepo,
		Accesses:      accessRepo,
		ClientFactory: nodeclient.TrafficFactory{Secret: cfg.NodeAgentSecret, Timeout: cfg.NodeAgentTimeout, MaxRetries: cfg.NodeAgentRetries},
		PollInterval:  cfg.PeerConsistencyInterval,
	}.Run(workerCtx)

	go app.DailyChargeWorker{
		PG:                 pg,
		RevokeAccess:       app.RevokeDeviceAccess{Accesses: accessRepo, Devices: deviceRepo, Operations: opRepo, AuditLogs: auditRepo, Tokens: tokenRepo, RevokePeerExecutor: &app.ExecuteRevokePeerOperation{Operations: opRepo, Accesses: accessRepo, Nodes: nodeRepo, NodeClient: nodeAppClient}},
		Interval:           cfg.DailyChargeInterval,
		DailyChargeKopecks: cfg.DailyChargeKopecks,
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

func ensureSingleNode(ctx context.Context, logger *slog.Logger, pg *pgxpool.Pool, cfg app.Config) {
	if cfg.NodeID == "" {
		return
	}
	nodeName := cfg.NodeName
	if nodeName == "" {
		nodeName = "single-node"
	}
	region := cfg.NodeRegion
	if region == "" {
		region = "single-server"
	}
	publicIP := cfg.NodePublicIP
	if publicIP == "" {
		publicIP = "127.0.0.1"
	}
	agentBaseURL := cfg.NodeAgentBaseURL
	if agentBaseURL == "" {
		agentBaseURL = "http://node-agent:8081"
	}
	capacity := cfg.NodeCapacity
	if capacity <= 0 {
		capacity = 40
	}
	meta, _ := json.Marshal(map[string]any{"protocols_supported": cfg.NodeProtocolsSupported, "bootstrapped_by": "control-plane"})
	_, err := pg.Exec(ctx, `
INSERT INTO vpn_nodes (
	id, name, region, status, user_capacity, agent_base_url, vpn_endpoint,
	vpn_endpoint_host, vpn_endpoint_port, server_public_key, vpn_subnet_cidr,
	runtime_metadata, last_seen_at, created_at, updated_at
) VALUES (
	$1, $2, $3, 'active', $4, $5, $6 || ':41475', $6, 41475,
	'', '10.8.1.0/24', $7::jsonb, NOW(), NOW(), NOW()
)
ON CONFLICT (id) DO UPDATE SET
	name=COALESCE(NULLIF(EXCLUDED.name,''), vpn_nodes.name),
	region=COALESCE(NULLIF(EXCLUDED.region,''), vpn_nodes.region),
	user_capacity=CASE WHEN EXCLUDED.user_capacity > 0 THEN EXCLUDED.user_capacity ELSE vpn_nodes.user_capacity END,
	agent_base_url=COALESCE(NULLIF(EXCLUDED.agent_base_url,''), vpn_nodes.agent_base_url),
	runtime_metadata=EXCLUDED.runtime_metadata,
	status='active',
	updated_at=NOW()`, cfg.NodeID, nodeName, region, capacity, agentBaseURL, publicIP, string(meta))
	if err != nil {
		logger.Error("ensure single node bootstrap failed", slog.Any("error", err), slog.String("node_id", cfg.NodeID))
		return
	}
	logger.Info("single node ensured", slog.String("node_id", cfg.NodeID), slog.String("node_name", nodeName), slog.String("region", region), slog.String("endpoint", fmt.Sprintf("%s:41475", publicIP)))
}
