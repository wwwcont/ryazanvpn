package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config stores process configuration loaded from environment variables.
type Config struct {
	ServiceName            string
	HTTPAddr               string
	ShutdownTimeout        time.Duration
	LogLevel               string
	PostgresURL            string
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	ReadinessTimeout       time.Duration
	LivenessTimeout        time.Duration
	AgentHMACSecret        string
	AgentHMACMaxSkew       time.Duration
	NodeAgentBaseURL       string
	NodeAgentSecret        string
	NodeAgentTimeout       time.Duration
	NodeAgentRetries       int
	ConfigMasterKey        string
	AdminSecret            string
	AdminSecretHeader      string
	NodeHealthPollInterval time.Duration
	NodeHealthCheckTimeout time.Duration
	RuntimeAdapter         string
	RuntimeWorkDir         string
	AWGBinaryPath          string
	WGBinaryPath           string
	IPBinaryPath           string
	RuntimeExecTimeout     time.Duration
}

// LoadConfig reads and validates service configuration from env.
func LoadConfig(serviceName string) (Config, error) {
	cfg := Config{
		ServiceName:            serviceName,
		HTTPAddr:               envOrDefault("HTTP_ADDR", ":8080"),
		ShutdownTimeout:        durationFromEnv("SHUTDOWN_TIMEOUT", 15*time.Second),
		LogLevel:               envOrDefault("LOG_LEVEL", "info"),
		PostgresURL:            os.Getenv("POSTGRES_URL"),
		RedisAddr:              envOrDefault("REDIS_ADDR", "redis:6379"),
		RedisPassword:          os.Getenv("REDIS_PASSWORD"),
		RedisDB:                intFromEnv("REDIS_DB", 0),
		ReadinessTimeout:       durationFromEnv("READINESS_TIMEOUT", 2*time.Second),
		LivenessTimeout:        durationFromEnv("LIVENESS_TIMEOUT", 2*time.Second),
		AgentHMACSecret:        os.Getenv("AGENT_HMAC_SECRET"),
		AgentHMACMaxSkew:       durationFromEnv("AGENT_HMAC_MAX_SKEW", 5*time.Minute),
		NodeAgentBaseURL:       envOrDefault("NODE_AGENT_BASE_URL", "http://node-agent:8081"),
		NodeAgentSecret:        os.Getenv("NODE_AGENT_HMAC_SECRET"),
		NodeAgentTimeout:       durationFromEnv("NODE_AGENT_TIMEOUT", 5*time.Second),
		NodeAgentRetries:       intFromEnv("NODE_AGENT_RETRIES", 2),
		ConfigMasterKey:        os.Getenv("CONFIG_MASTER_KEY"),
		AdminSecret:            os.Getenv("ADMIN_API_SECRET"),
		AdminSecretHeader:      envOrDefault("ADMIN_API_SECRET_HEADER", "X-Admin-Secret"),
		NodeHealthPollInterval: durationFromEnv("NODE_HEALTH_POLL_INTERVAL", 15*time.Second),
		NodeHealthCheckTimeout: durationFromEnv("NODE_HEALTH_CHECK_TIMEOUT", 3*time.Second),
		RuntimeAdapter:         envOrDefault("RUNTIME_ADAPTER", "mock"),
		RuntimeWorkDir:         envOrDefault("RUNTIME_WORK_DIR", "/var/lib/ryazanvpn/node-agent"),
		AWGBinaryPath:          envOrDefault("AWG_BINARY_PATH", "/usr/bin/awg"),
		WGBinaryPath:           envOrDefault("WG_BINARY_PATH", "/usr/bin/wg"),
		IPBinaryPath:           envOrDefault("IP_BINARY_PATH", "/usr/sbin/ip"),
		RuntimeExecTimeout:     durationFromEnv("RUNTIME_EXEC_TIMEOUT", 10*time.Second),
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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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

func (c Config) String() string {
	return fmt.Sprintf("service=%s http_addr=%s log_level=%s redis_addr=%s redis_db=%d",
		c.ServiceName, c.HTTPAddr, c.LogLevel, c.RedisAddr, c.RedisDB,
	)
}
