package server

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultConfigPath = "/etc/server-logger/config.yaml"

type Config struct {
	Port      int `yaml:"port"`
	Database  DatabaseConfig
	Logs      LogsConfig
	Security  SecurityConfig
	Server    ServerConfig
	Retention RetentionConfig `yaml:"retention"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type LogsConfig struct {
	RetentionDays int      `yaml:"retention_days"`
	MaxLineBytes  int      `yaml:"max_line_bytes"`
	PollInterval  int      `yaml:"poll_interval_seconds"`
	BatchSize     int      `yaml:"batch_size"`
	BufferSize    int      `yaml:"buffer_size"`
	Sources       []string `yaml:"sources"`
}

type SecurityConfig struct {
	AuthEnabled bool            `yaml:"auth_enabled"`
	Username    string          `yaml:"username"`
	Password    string          `yaml:"password"`
	HTTPS       HTTPSConfig     `yaml:"https"`
	RateLimit   RateLimitConfig `yaml:"rate_limit"`
}

type HTTPSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type RateLimitConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
	Burst             int `yaml:"burst"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
}

type RetentionConfig struct {
	CleanupIntervalMinutes int `yaml:"cleanup_interval_minutes"`
}

func LoadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	if path == "" {
		path = DefaultConfigPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "/var/lib/server-logger/logs.db"
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Logs.RetentionDays <= 0 {
		cfg.Logs.RetentionDays = 7
	}
	if cfg.Logs.MaxLineBytes <= 0 {
		cfg.Logs.MaxLineBytes = 1024 * 1024
	}
	if cfg.Logs.PollInterval <= 0 {
		cfg.Logs.PollInterval = 1
	}
	if cfg.Logs.BatchSize <= 0 {
		cfg.Logs.BatchSize = 500
	}
	if cfg.Logs.BufferSize <= 0 {
		cfg.Logs.BufferSize = 4096
	}
	if cfg.Security.RateLimit.RequestsPerMinute <= 0 {
		cfg.Security.RateLimit.RequestsPerMinute = 240
	}
	if cfg.Security.RateLimit.Burst <= 0 {
		cfg.Security.RateLimit.Burst = 60
	}
	if cfg.Retention.CleanupIntervalMinutes <= 0 {
		cfg.Retention.CleanupIntervalMinutes = 60
	}
	return cfg, nil
}

func defaultConfig() Config {
	return Config{
		Port: 8085,
		Database: DatabaseConfig{
			Path: "/var/lib/server-logger/logs.db",
		},
		Logs: LogsConfig{
			RetentionDays: 7,
			MaxLineBytes:  1024 * 1024,
			PollInterval:  1,
			BatchSize:     500,
			BufferSize:    4096,
		},
		Security: SecurityConfig{
			AuthEnabled: false,
			HTTPS: HTTPSConfig{
				Enabled: false,
			},
			RateLimit: RateLimitConfig{
				RequestsPerMinute: 240,
				Burst:             60,
			},
		},
		Server: ServerConfig{
			Host: "0.0.0.0",
		},
		Retention: RetentionConfig{
			CleanupIntervalMinutes: 60,
		},
	}
}

func EnsureRuntimePaths(cfg Config) error {
	paths := []string{
		filepath.Dir(cfg.Database.Path),
		"/var/log/server-logger",
		filepath.Dir(DefaultConfigPath),
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func CleanupInterval(cfg Config) time.Duration {
	return time.Duration(cfg.Retention.CleanupIntervalMinutes) * time.Minute
}
