package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config stores process configuration loaded from environment variables.
type Config struct {
	ServiceName             string
	HTTPAddr                string
	ShutdownTimeout         time.Duration
	LogLevel                string
	PostgresURL             string
	RedisAddr               string
	RedisPassword           string
	RedisDB                 int
	ReadinessTimeout        time.Duration
	LivenessTimeout         time.Duration
	AgentHMACSecret         string
	AgentHMACMaxSkew        time.Duration
	NodeAgentBaseURL        string
	NodeAgentSecret         string
	NodeAgentTimeout        time.Duration
	NodeAgentRetries        int
	ConfigMasterKey         string
	AdminSecret             string
	AdminSecretHeader       string
	NodeHealthPollInterval  time.Duration
	NodeHealthCheckTimeout  time.Duration
	PeerConsistencyInterval time.Duration
	RuntimeAdapter          string
	RuntimeWorkDir          string
	AWGBinaryPath           string
	WGBinaryPath            string
	IPBinaryPath            string
	RuntimeExecTimeout      time.Duration
	RuntimeStatsBinaryPath  string
	RuntimeStatsArgs        []string
	AmneziaContainerName    string
	AmneziaInterfaceName    string
	XrayContainerName       string
	DockerBinaryPath        string
	VPNSubnetCIDR           string
	VPNServerPublicEndpoint string
	VPNServerPublicKey      string
	VPNClientAllowedIPs     []string
	VPNAWGJc                int
	VPNAWGJmin              int
	VPNAWGJmax              int
	VPNAWGS1                int
	VPNAWGS2                int
	VPNAWGS3                int
	VPNAWGS4                int
	VPNAWGH1                string
	VPNAWGH2                string
	VPNAWGH3                string
	VPNAWGH4                string
	VPNAWGI1                string
	VPNAWGI2                string
	VPNAWGI3                string
	VPNAWGI4                string
	VPNAWGI5                string
	VPNAWGMTU               int
	TelegramBotToken        string
	TelegramWebhookSecret   string
	PublicBaseURL           string
	TelegramStateTTL        time.Duration
	TelegramAdminIDs        []int64
	NodeName                string
	ControlPlaneBaseURL     string
	NodeHeartbeatInterval   time.Duration
	NodeProtocolsSupported  []string
	NodeRegistrationToken   string
	NodeID                  string
	NodeToken               string
	NodeRegion              string
	NodePublicIP            string
	NodeCapacity            int
	DailyChargeInterval     time.Duration
	DailyChargeKopecks      int64
}

// LoadConfig reads and validates service configuration from env.
func LoadConfig(serviceName string) (Config, error) {
	cfg := Config{
		ServiceName:             serviceName,
		HTTPAddr:                serviceHTTPAddr(serviceName),
		ShutdownTimeout:         durationFromEnv("SHUTDOWN_TIMEOUT", 15*time.Second),
		LogLevel:                envOrDefault("LOG_LEVEL", "info"),
		PostgresURL:             os.Getenv("POSTGRES_URL"),
		RedisAddr:               envOrDefault("REDIS_ADDR", "redis:6379"),
		RedisPassword:           os.Getenv("REDIS_PASSWORD"),
		RedisDB:                 intFromEnv("REDIS_DB", 0),
		ReadinessTimeout:        durationFromEnv("READINESS_TIMEOUT", 2*time.Second),
		LivenessTimeout:         durationFromEnv("LIVENESS_TIMEOUT", 2*time.Second),
		AgentHMACSecret:         firstNonEmpty(os.Getenv("AGENT_HMAC_SECRET"), os.Getenv("NODE_AGENT_HMAC_SECRET")),
		AgentHMACMaxSkew:        durationFromEnv("AGENT_HMAC_MAX_SKEW", 5*time.Minute),
		NodeAgentBaseURL:        envOrDefault("NODE_AGENT_BASE_URL", "http://node-agent:8081"),
		NodeAgentSecret:         firstNonEmpty(os.Getenv("NODE_AGENT_HMAC_SECRET"), os.Getenv("AGENT_HMAC_SECRET")),
		NodeAgentTimeout:        durationFromEnv("NODE_AGENT_TIMEOUT", 5*time.Second),
		NodeAgentRetries:        intFromEnv("NODE_AGENT_RETRIES", 2),
		ConfigMasterKey:         os.Getenv("CONFIG_MASTER_KEY"),
		AdminSecret:             os.Getenv("ADMIN_API_SECRET"),
		AdminSecretHeader:       envOrDefault("ADMIN_API_SECRET_HEADER", "X-Admin-Secret"),
		NodeHealthPollInterval:  durationFromEnv("NODE_HEALTH_POLL_INTERVAL", 15*time.Second),
		NodeHealthCheckTimeout:  durationFromEnv("NODE_HEALTH_CHECK_TIMEOUT", 3*time.Second),
		PeerConsistencyInterval: durationFromEnv("PEER_CONSISTENCY_INTERVAL", 2*time.Minute),
		RuntimeAdapter:          envOrDefault("RUNTIME_ADAPTER", "mock"),
		RuntimeWorkDir:          envOrDefault("RUNTIME_WORK_DIR", "/var/lib/ryazanvpn/node-agent"),
		AWGBinaryPath:           envOrDefault("AWG_BINARY_PATH", "/usr/bin/awg"),
		WGBinaryPath:            envOrDefault("WG_BINARY_PATH", "/usr/bin/wg"),
		IPBinaryPath:            envOrDefault("IP_BINARY_PATH", "/usr/sbin/ip"),
		RuntimeExecTimeout:      durationFromEnv("RUNTIME_EXEC_TIMEOUT", 10*time.Second),
		RuntimeStatsBinaryPath:  os.Getenv("RUNTIME_STATS_BINARY_PATH"),
		RuntimeStatsArgs:        csvListFromEnv("RUNTIME_STATS_ARGS"),
		AmneziaContainerName:    envOrDefault("AMNEZIA_CONTAINER_NAME", "amnezia-awg2"),
		AmneziaInterfaceName:    envOrDefault("AMNEZIA_INTERFACE_NAME", "awg0"),
		XrayContainerName:       envOrDefault("XRAY_CONTAINER_NAME", "ryazanvpn-xray"),
		DockerBinaryPath:        envOrDefault("DOCKER_BINARY_PATH", "/usr/bin/docker"),
		VPNSubnetCIDR:           envOrDefault("VPN_SUBNET_CIDR", "10.8.1.0/24"),
		VPNServerPublicEndpoint: envOrDefault("VPN_SERVER_PUBLIC_ENDPOINT", ""),
		VPNServerPublicKey:      envOrDefault("VPN_SERVER_PUBLIC_KEY", ""),
		VPNClientAllowedIPs:     csvListFromEnvOrDefault("VPN_CLIENT_ALLOWED_IPS", []string{"0.0.0.0/0", "::/0"}),
		VPNAWGJc:                intFromEnv("VPN_AWG_JC", 4),
		VPNAWGJmin:              intFromEnv("VPN_AWG_JMIN", 10),
		VPNAWGJmax:              intFromEnv("VPN_AWG_JMAX", 50),
		VPNAWGS1:                intFromEnv("VPN_AWG_S1", 50),
		VPNAWGS2:                intFromEnv("VPN_AWG_S2", 74),
		VPNAWGS3:                intFromEnv("VPN_AWG_S3", 45),
		VPNAWGS4:                intFromEnv("VPN_AWG_S4", 16),
		VPNAWGH1:                envOrDefault("VPN_AWG_H1", "1391505721-1463481553"),
		VPNAWGH2:                envOrDefault("VPN_AWG_H2", "1725378175-1834354614"),
		VPNAWGH3:                envOrDefault("VPN_AWG_H3", "2076643873-2118219660"),
		VPNAWGH4:                envOrDefault("VPN_AWG_H4", "2141781406-2147031473"),
		VPNAWGI1:                envOrDefault("VPN_AWG_I1", "<r 2><b 0x858000010001000000000669636c6f756403636f6d0000010001c00c000100010000105a00044d583737>"),
		VPNAWGI2:                envOrDefault("VPN_AWG_I2", ""),
		VPNAWGI3:                envOrDefault("VPN_AWG_I3", ""),
		VPNAWGI4:                envOrDefault("VPN_AWG_I4", ""),
		VPNAWGI5:                envOrDefault("VPN_AWG_I5", ""),
		VPNAWGMTU:               intFromEnv("VPN_AWG_MTU", 1376),
		TelegramBotToken:        os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramWebhookSecret:   os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		PublicBaseURL:           envOrDefault("PUBLIC_BASE_URL", "http://localhost:8080"),
		TelegramStateTTL:        durationFromEnv("TELEGRAM_STATE_TTL", 24*time.Hour),
		TelegramAdminIDs:        int64ListFromEnv("TELEGRAM_ADMIN_IDS"),
		NodeName:                envOrDefault("NODE_NAME", ""),
		ControlPlaneBaseURL:     envOrDefault("CONTROL_PLANE_BASE_URL", "http://control-plane:8080"),
		NodeHeartbeatInterval:   durationFromEnv("NODE_HEARTBEAT_INTERVAL", 45*time.Second),
		NodeProtocolsSupported:  csvListFromEnvOrDefault("NODE_PROTOCOLS_SUPPORTED", []string{"wireguard", "xray"}),
		NodeRegistrationToken:   envOrDefault("NODE_REGISTRATION_TOKEN", ""),
		NodeID:                  envOrDefault("NODE_ID", ""),
		NodeToken:               envOrDefault("NODE_TOKEN", ""),
		NodeRegion:              envOrDefault("NODE_REGION", ""),
		NodePublicIP:            envOrDefault("NODE_PUBLIC_IP", ""),
		NodeCapacity:            intFromEnv("NODE_CAPACITY", 0),
		DailyChargeInterval:     durationFromEnv("DAILY_CHARGE_INTERVAL", 1*time.Hour),
		DailyChargeKopecks:      int64(intFromEnv("DAILY_CHARGE_KOPECKS", 800)),
	}

	if cfg.HTTPAddr == "" {
		return Config{}, errors.New("HTTP_ADDR must not be empty")
	}

	switch serviceName {
	case "control-plane":
		if cfg.PostgresURL == "" {
			return Config{}, errors.New("POSTGRES_URL is required")
		}
	case "node-agent":
		// node-agent routes do not depend on postgres/redis in startup path.
	default:
		if cfg.PostgresURL == "" {
			return Config{}, errors.New("POSTGRES_URL is required")
		}
	}

	return cfg, nil
}

func serviceHTTPAddr(serviceName string) string {
	var scopedKey string
	switch serviceName {
	case "control-plane":
		scopedKey = "CONTROL_PLANE_HTTP_ADDR"
	case "node-agent":
		scopedKey = "NODE_AGENT_HTTP_ADDR"
	}
	if scopedKey != "" {
		if v := strings.TrimSpace(os.Getenv(scopedKey)); v != "" {
			return v
		}
	}
	return envOrDefault("HTTP_ADDR", ":8080")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func intFromEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func int64ListFromEnv(key string) []int64 {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

func csvListFromEnv(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func csvListFromEnvOrDefault(key string, fallback []string) []string {
	values := csvListFromEnv(key)
	if len(values) == 0 {
		return append([]string(nil), fallback...)
	}
	return values
}

func (c Config) String() string {
	return fmt.Sprintf("service=%s http_addr=%s log_level=%s redis_addr=%s redis_db=%d",
		c.ServiceName, c.HTTPAddr, c.LogLevel, c.RedisAddr, c.RedisDB,
	)
}
