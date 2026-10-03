package config

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration values for the application.
type Config struct {
	TelegramBotToken    string
	AllowedUserIDs      map[int64]bool
	CheckInterval       time.Duration
	RequestTimeout      time.Duration
	MaxHosts            int
	MaxConcurrentChecks int
	MaxResponseBody     int64
	MaxRedirects        int
	MonitorPorts        []int
	CommandRateLimit    int
	CommandRateWindow   time.Duration
	DBPath              string
	RetentionDays       int
	LogLevel            slog.Level
}

// Load reads configuration from .env file (if present) and environment variables,
// applying safe defaults for missing optional values.
func Load(envFilePath ...string) (*Config, error) {
	// Try loading .env file if path specified or default .env exists
	targetEnvPath := ".env"
	if len(envFilePath) > 0 && envFilePath[0] != "" {
		targetEnvPath = envFilePath[0]
	}

	if err := loadDotEnv(targetEnvPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Log or return error if .env exists but cannot be read
		return nil, fmt.Errorf("failed to read .env file: %w", err)
	}

	cfg := &Config{
		TelegramBotToken:    getEnv("TELEGRAM_BOT_TOKEN", ""),
		AllowedUserIDs:      parseAllowedUserIDs(getEnv("ALLOWED_TELEGRAM_USER_IDS", "")),
		CheckInterval:       getEnvDuration("CHECK_INTERVAL", 1*time.Minute),
		RequestTimeout:      getEnvDuration("REQUEST_TIMEOUT", 10*time.Second),
		MaxHosts:            getEnvInt("MAX_HOSTS", 50),
		MaxConcurrentChecks: getEnvInt("MAX_CONCURRENT_CHECKS", 5),
		MaxResponseBody:     getEnvInt64("MAX_RESPONSE_BODY", 1048576), // 1MB
		MaxRedirects:        getEnvInt("MAX_REDIRECTS", 5),
		MonitorPorts:        parsePorts(getEnv("MONITOR_PORTS", "22,80,443")),
		CommandRateLimit:    getEnvInt("COMMAND_RATE_LIMIT", 10),
		CommandRateWindow:   getEnvDuration("COMMAND_RATE_WINDOW", 1*time.Minute),
		DBPath:              getEnv("DB_PATH", "data/monitor.db"),
		RetentionDays:       getEnvInt("RETENTION_DAYS", 7),
		LogLevel:            parseLogLevel(getEnv("LOG_LEVEL", "INFO")),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate ensures critical configuration invariants are met.
func (c *Config) Validate() error {
	if c.MaxHosts <= 0 {
		return errors.New("MAX_HOSTS must be greater than 0")
	}
	if c.MaxConcurrentChecks <= 0 {
		return errors.New("MAX_CONCURRENT_CHECKS must be greater than 0")
	}
	if c.RequestTimeout <= 0 {
		return errors.New("REQUEST_TIMEOUT must be greater than 0")
	}
	if c.CheckInterval <= 0 {
		return errors.New("CHECK_INTERVAL must be greater than 0")
	}
	if c.MaxResponseBody <= 0 {
		return errors.New("MAX_RESPONSE_BODY must be greater than 0")
	}
	if c.MaxRedirects < 0 {
		return errors.New("MAX_REDIRECTS cannot be negative")
	}
	if c.CommandRateLimit <= 0 {
		return errors.New("COMMAND_RATE_LIMIT must be greater than 0")
	}
	if c.RetentionDays <= 0 {
		return errors.New("RETENTION_DAYS must be greater than 0")
	}
	if len(c.MonitorPorts) == 0 {
		return errors.New("MONITOR_PORTS cannot be empty")
	}
	for _, port := range c.MonitorPorts {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid port in MONITOR_PORTS: %d (must be 1-65535)", port)
		}
	}
	return nil
}

// IsUserAllowed checks if a Telegram User ID is whitelisted.
func (c *Config) IsUserAllowed(userID int64) bool {
	return c.AllowedUserIDs[userID]
}

// loadDotEnv parses a standard key=value .env file without external dependencies.
func loadDotEnv(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip surrounding quotes if present
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}

		// Set environment variable only if not already set in OS environment
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val, exists := os.LookupEnv(key); exists {
		if dur, err := time.ParseDuration(strings.TrimSpace(val)); err == nil {
			return dur
		}
	}
	return defaultVal
}

func parseAllowedUserIDs(raw string) map[int64]bool {
	allowed := make(map[int64]bool)
	if strings.TrimSpace(raw) == "" {
		return allowed
	}

	parts := strings.Split(raw, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if id, err := strconv.ParseInt(p, 10, 64); err == nil && id > 0 {
			allowed[id] = true
		}
	}
	return allowed
}

func parsePorts(raw string) []int {
	var ports []int
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if port, err := strconv.Atoi(p); err == nil && port >= 1 && port <= 65535 {
			ports = append(ports, port)
		}
	}
	if len(ports) == 0 {
		return []int{22, 80, 443}
	}
	return ports
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// EnsureDataDir makes sure the parent directory for the database exists.
func (c *Config) EnsureDataDir() error {
	dir := filepath.Dir(c.DBPath)
	if dir != "" && dir != "." {
		return os.MkdirAll(dir, 0700)
	}
	return nil
}
