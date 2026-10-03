package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"host-monitor/internal/bot"
	"host-monitor/internal/config"
	"host-monitor/internal/database"
	"host-monitor/internal/monitor"
	"host-monitor/internal/notification"
)

func main() {
	// 1. Temporary logger for boot
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 2. Load and validate configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// 3. Reconfigure logger to user-defined level
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting Host Monitoring Telegram Bot...")

	// 4. Ensure data directory exists with strict permissions
	if err := cfg.EnsureDataDir(); err != nil {
		slog.Error("Failed to create data directory", "path", cfg.DBPath, "error", err)
		os.Exit(1)
	}

	// 5. Initialize SQLite Database
	db, err := database.NewDatabase(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to initialize database", "path", cfg.DBPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()
	repo := database.NewRepository(db)

	// 6. Log sanitized configuration summary (NEVER log secrets/tokens)
	slog.Info("Configuration loaded successfully",
		"check_interval", cfg.CheckInterval,
		"request_timeout", cfg.RequestTimeout,
		"max_hosts", cfg.MaxHosts,
		"max_concurrent_checks", cfg.MaxConcurrentChecks,
		"max_response_body", fmt.Sprintf("%d bytes", cfg.MaxResponseBody),
		"max_redirects", cfg.MaxRedirects,
		"monitor_ports", cfg.MonitorPorts,
		"rate_limit", fmt.Sprintf("%d req / %v", cfg.CommandRateLimit, cfg.CommandRateWindow),
		"db_path", cfg.DBPath,
		"retention_days", cfg.RetentionDays,
		"allowed_users_count", len(cfg.AllowedUserIDs),
		"token_configured", cfg.TelegramBotToken != "",
	)

	// 7. Setup graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if cfg.TelegramBotToken == "" {
		slog.Warn("TELEGRAM_BOT_TOKEN is not set. Set it in .env or environment variable to start Telegram bot.")
		slog.Info("Bot running in idle mode (waiting for termination signal)...")
		<-ctx.Done()
	} else {
		// Initialize HTTP client for Telegram
		httpClient := &http.Client{
			Timeout: 35 * time.Second,
		}

		client, err := bot.NewClient(cfg.TelegramBotToken, httpClient)
		if err != nil {
			slog.Error("Failed to initialize Telegram client", "error", err)
			os.Exit(1)
		}

		notifier := notification.NewNotifier(cfg, client, repo)
		checker := monitor.NewChecker(cfg)
		scheduler := monitor.NewScheduler(cfg, checker, repo, notifier)
		tgBot := bot.NewBot(cfg, client, repo)

		var wg sync.WaitGroup

		// Start Telegram long polling worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := tgBot.StartLongPolling(ctx); err != nil {
				slog.Error("Telegram polling loop error", "error", err)
			}
		}()

		// Start Automatic Monitoring Scheduler worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			scheduler.Start(ctx)
		}()

		slog.Info("All services started successfully. Running monitoring loop...")

		// Wait for shutdown signal
		<-ctx.Done()

		slog.Info("Shutdown signal received. Stopping workers gracefully...")
		wg.Wait()
	}

	slog.Info("Closing database...")
	_ = db.Close()
	slog.Info("Bot stopped gracefully.")
}
