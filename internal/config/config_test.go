package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	// Clear any relevant env vars
	envVars := []string{
		"TELEGRAM_BOT_TOKEN", "ALLOWED_TELEGRAM_USER_IDS", "CHECK_INTERVAL",
		"REQUEST_TIMEOUT", "MAX_HOSTS", "MAX_CONCURRENT_CHECKS", "MAX_RESPONSE_BODY",
		"MAX_REDIRECTS", "MONITOR_PORTS", "COMMAND_RATE_LIMIT", "COMMAND_RATE_WINDOW",
		"DB_PATH", "RETENTION_DAYS", "LOG_LEVEL",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}

	cfg, err := Load("non_existent_env_file.env")
	if err != nil {
		t.Fatalf("expected Load to succeed with defaults, got: %v", err)
	}

	if cfg.CheckInterval != 1*time.Minute {
		t.Errorf("expected CheckInterval 1m, got %v", cfg.CheckInterval)
	}
	if cfg.RequestTimeout != 10*time.Second {
		t.Errorf("expected RequestTimeout 10s, got %v", cfg.RequestTimeout)
	}
	if cfg.MaxHosts != 50 {
		t.Errorf("expected MaxHosts 50, got %d", cfg.MaxHosts)
	}
	if cfg.MaxConcurrentChecks != 5 {
		t.Errorf("expected MaxConcurrentChecks 5, got %d", cfg.MaxConcurrentChecks)
	}
	if cfg.MaxResponseBody != 1048576 {
		t.Errorf("expected MaxResponseBody 1048576, got %d", cfg.MaxResponseBody)
	}
	if cfg.MaxRedirects != 5 {
		t.Errorf("expected MaxRedirects 5, got %d", cfg.MaxRedirects)
	}
	if cfg.CommandRateLimit != 10 {
		t.Errorf("expected CommandRateLimit 10, got %d", cfg.CommandRateLimit)
	}
	if cfg.CommandRateWindow != 1*time.Minute {
		t.Errorf("expected CommandRateWindow 1m, got %v", cfg.CommandRateWindow)
	}
	if cfg.DBPath != "data/monitor.db" {
		t.Errorf("expected DBPath data/monitor.db, got %s", cfg.DBPath)
	}
	if cfg.RetentionDays != 7 {
		t.Errorf("expected RetentionDays 7, got %d", cfg.RetentionDays)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("expected LogLevel Info, got %v", cfg.LogLevel)
	}
	if len(cfg.MonitorPorts) != 3 || cfg.MonitorPorts[0] != 22 || cfg.MonitorPorts[1] != 80 || cfg.MonitorPorts[2] != 443 {
		t.Errorf("expected MonitorPorts [22 80 443], got %v", cfg.MonitorPorts)
	}
}

func TestConfigFromEnvFile(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env.test")

	content := `
TELEGRAM_BOT_TOKEN=secret_token_123
ALLOWED_TELEGRAM_USER_IDS=111111,222222
CHECK_INTERVAL=30s
REQUEST_TIMEOUT=5s
MAX_HOSTS=100
MAX_CONCURRENT_CHECKS=10
MAX_RESPONSE_BODY=524288
MAX_REDIRECTS=3
MONITOR_PORTS=80,443,8080
COMMAND_RATE_LIMIT=20
COMMAND_RATE_WINDOW=30s
DB_PATH=custom_data/custom.db
RETENTION_DAYS=14
LOG_LEVEL=DEBUG
`
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatalf("failed to load config from file: %v", err)
	}

	if cfg.TelegramBotToken != "secret_token_123" {
		t.Errorf("unexpected TelegramBotToken: %s", cfg.TelegramBotToken)
	}
	if !cfg.IsUserAllowed(111111) || !cfg.IsUserAllowed(222222) || cfg.IsUserAllowed(333333) {
		t.Errorf("unexpected IsUserAllowed behavior: %v", cfg.AllowedUserIDs)
	}
	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("unexpected CheckInterval: %v", cfg.CheckInterval)
	}
	if cfg.RequestTimeout != 5*time.Second {
		t.Errorf("unexpected RequestTimeout: %v", cfg.RequestTimeout)
	}
	if cfg.MaxHosts != 100 {
		t.Errorf("unexpected MaxHosts: %d", cfg.MaxHosts)
	}
	if cfg.MaxConcurrentChecks != 10 {
		t.Errorf("unexpected MaxConcurrentChecks: %d", cfg.MaxConcurrentChecks)
	}
	if cfg.MaxResponseBody != 524288 {
		t.Errorf("unexpected MaxResponseBody: %d", cfg.MaxResponseBody)
	}
	if cfg.MaxRedirects != 3 {
		t.Errorf("unexpected MaxRedirects: %d", cfg.MaxRedirects)
	}
	if cfg.CommandRateLimit != 20 {
		t.Errorf("unexpected CommandRateLimit: %d", cfg.CommandRateLimit)
	}
	if cfg.CommandRateWindow != 30*time.Second {
		t.Errorf("unexpected CommandRateWindow: %v", cfg.CommandRateWindow)
	}
	if cfg.DBPath != "custom_data/custom.db" {
		t.Errorf("unexpected DBPath: %s", cfg.DBPath)
	}
	if cfg.RetentionDays != 14 {
		t.Errorf("unexpected RetentionDays: %d", cfg.RetentionDays)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("unexpected LogLevel: %v", cfg.LogLevel)
	}
	if len(cfg.MonitorPorts) != 3 || cfg.MonitorPorts[0] != 80 || cfg.MonitorPorts[1] != 443 || cfg.MonitorPorts[2] != 8080 {
		t.Errorf("unexpected MonitorPorts: %v", cfg.MonitorPorts)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(c *Config)
		wantErr bool
	}{
		{
			name: "Valid Config",
			modify: func(c *Config) {
				// defaults are valid
			},
			wantErr: false,
		},
		{
			name: "Invalid MaxHosts",
			modify: func(c *Config) {
				c.MaxHosts = 0
			},
			wantErr: true,
		},
		{
			name: "Invalid MaxConcurrentChecks",
			modify: func(c *Config) {
				c.MaxConcurrentChecks = -1
			},
			wantErr: true,
		},
		{
			name: "Invalid RequestTimeout",
			modify: func(c *Config) {
				c.RequestTimeout = 0
			},
			wantErr: true,
		},
		{
			name: "Invalid MonitorPort Out of Range",
			modify: func(c *Config) {
				c.MonitorPorts = []int{80, 70000}
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				CheckInterval:       1 * time.Minute,
				RequestTimeout:      10 * time.Second,
				MaxHosts:            50,
				MaxConcurrentChecks: 5,
				MaxResponseBody:     1048576,
				MaxRedirects:        5,
				MonitorPorts:        []int{22, 80, 443},
				CommandRateLimit:    10,
				CommandRateWindow:   1 * time.Minute,
				DBPath:              "data/monitor.db",
				RetentionDays:       7,
				LogLevel:            slog.LevelInfo,
			}
			tc.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
