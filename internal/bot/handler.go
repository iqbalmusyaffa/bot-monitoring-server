package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"host-monitor/internal/config"
	"host-monitor/internal/database"
	"host-monitor/internal/monitor"
	"host-monitor/internal/security"
)

// Bot manages the Telegram bot lifecycle, command routing, rate limiting, and database interactions.
type Bot struct {
	cfg         *config.Config
	client      *Client
	auth        *Authenticator
	rateLimiter *security.RateLimiter
	checker     *monitor.Checker
	repo        *database.Repository
}

// NewBot instantiates a Bot instance.
func NewBot(cfg *config.Config, client *Client, repo *database.Repository) *Bot {
	var limit = 10
	var window = 1 * time.Minute
	if cfg != nil {
		if cfg.CommandRateLimit > 0 {
			limit = cfg.CommandRateLimit
		}
		if cfg.CommandRateWindow > 0 {
			window = cfg.CommandRateWindow
		}
	}

	return &Bot{
		cfg:         cfg,
		client:      client,
		auth:        NewAuthenticator(cfg, repo),
		rateLimiter: security.NewRateLimiter(limit, window),
		checker:     monitor.NewChecker(cfg),
		repo:        repo,
	}
}

// StartLongPolling runs the polling loop until the context is canceled.
func (b *Bot) StartLongPolling(ctx context.Context) error {
	me, err := b.client.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("failed to authenticate bot: %w", err)
	}
	slog.Info("Telegram bot connected successfully", "bot_username", me.Username, "bot_id", me.ID)

	var offset int64 = 0
	pollTimeout := 25

	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping Telegram long polling loop...")
			return nil
		default:
		}

		updates, err := b.client.GetUpdates(ctx, offset, pollTimeout)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			slog.Error("Failed to fetch updates from Telegram", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
			}
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}

			if update.Message != nil {
				b.handleMessage(ctx, update.Message)
			}
		}
	}
}

// handleMessage processes an incoming message with role-based authorization and rate limiting.
func (b *Bot) handleMessage(ctx context.Context, msg *Message) {
	if msg.Text == "" {
		return
	}

	cmd, arg := ParseCommand(msg.Text)
	if cmd == "" {
		return
	}

	var senderID int64
	var senderName string
	var senderUsername string
	if msg.From != nil {
		senderID = msg.From.ID
		senderName = msg.From.FirstName
		senderUsername = msg.From.Username
	}

	// 1. Role-based Authorization check
	isAuthorized, role, isFirstClaim := b.auth.CheckAndAuthorize(ctx, senderID, senderName, senderUsername)
	if !isAuthorized {
		b.auth.HandleUnauthorized(ctx, b.client, msg.Chat.ID, senderID, senderName, cmd)
		return
	}

	// First time claim greeting
	if isFirstClaim {
		welcomeClaim := fmt.Sprintf(`🎉 SELAMAT DATANG!

Sistem secara otomatis mendeteksi dan menetapkan Anda sebagai **Owner (Super Admin)** bot ini!

User ID Anda (%d) telah disimpan ke database dan memiliki akses penuh.`, senderID)
		_ = b.client.SendMessage(ctx, msg.Chat.ID, welcomeClaim)
	}

	// 2. Per-user Rate limit check
	if !b.rateLimiter.Allow(senderID) {
		slog.Warn("Rate limit exceeded for user", "user_id", senderID, "command", cmd)
		_ = b.client.SendMessage(ctx, msg.Chat.ID, security.RateLimitExceededMessage)
		return
	}

	slog.Info("Authorized command received", "command", cmd, "user_id", senderID, "role", role, "arg", arg)

	// Command routing with role validation
	switch cmd {
	case "/start":
		_ = b.client.SendMessage(ctx, msg.Chat.ID, StartMessage())
	case "/help":
		_ = b.client.SendMessage(ctx, msg.Chat.ID, HelpMessage())
	case "/check":
		b.handleCheck(ctx, msg.Chat.ID, arg)
	case "/list":
		b.handleList(ctx, msg.Chat.ID)
	case "/status":
		b.handleStatus(ctx, msg.Chat.ID, arg)

	// Admin / Owner level commands
	case "/add":
		if role == database.RoleUser {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Admin/Owner")
			return
		}
		b.handleAdd(ctx, msg.Chat.ID, arg)
	case "/remove":
		if role == database.RoleUser {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Admin/Owner")
			return
		}
		b.handleRemove(ctx, msg.Chat.ID, arg)
	case "/monitor":
		if role == database.RoleUser {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Admin/Owner")
			return
		}
		b.handleSetMonitoring(ctx, msg.Chat.ID, arg, true)
	case "/unmonitor":
		if role == database.RoleUser {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Admin/Owner")
			return
		}
		b.handleSetMonitoring(ctx, msg.Chat.ID, arg, false)

	// Owner level commands
	case "/adduser":
		if role != database.RoleOwner {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Owner")
			return
		}
		b.handleAddUser(ctx, msg.Chat.ID, arg)
	case "/removeuser":
		if role != database.RoleOwner {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Owner")
			return
		}
		b.handleRemoveUser(ctx, msg.Chat.ID, arg)
	case "/listusers":
		b.handleListUsers(ctx, msg.Chat.ID)

	default:
		response := fmt.Sprintf("Perintah '%s' tidak dikenali. Ketik /help untuk melihat menu perintah.", cmd)
		_ = b.client.SendMessage(ctx, msg.Chat.ID, response)
	}
}

func (b *Bot) replyForbiddenRole(ctx context.Context, chatID int64, requiredRole string) {
	msg := fmt.Sprintf("⚠️ Akses Ditolak.\n\nPerintah ini hanya dapat dijalankan oleh Role **%s**.\nRole Anda saat ini memiliki izin terbatas (Read-Only / User).", requiredRole)
	_ = b.client.SendMessage(ctx, chatID, msg)
}

func (b *Bot) handleCheck(ctx context.Context, chatID int64, rawTarget string) {
	if rawTarget == "" {
		_ = b.client.SendMessage(ctx, chatID, "ℹ️ Format penggunaan:\n/check <host>\n\nContoh:\n/check example.com\n/check 123.123.123.123\n/check 2001:db8::1")
		return
	}

	validated, userErrMsg, err := security.ValidateAndSanitizeInput(ctx, rawTarget)
	if err != nil {
		slog.Warn("Rejected check input", "target", rawTarget, "error", err)
		_ = b.client.SendMessage(ctx, chatID, userErrMsg)
		return
	}

	slog.Info("Executing /check", "host", validated.Normalized, "type", validated.Type)

	result := b.checker.CheckHost(ctx, validated.Normalized, validated.Type)
	formattedResponse := monitor.FormatCheckResult(result)

	_ = b.client.SendMessage(ctx, chatID, formattedResponse)
}

func (b *Bot) handleAdd(ctx context.Context, chatID int64, rawTarget string) {
	if rawTarget == "" {
		_ = b.client.SendMessage(ctx, chatID, "ℹ️ Format penggunaan:\n/add <host>\n\nContoh:\n/add example.com\n/add 123.123.123.123\n/add 2001:db8::1")
		return
	}

	if b.repo == nil {
		_ = b.client.SendMessage(ctx, chatID, "❌ Database belum siap.")
		return
	}

	count, err := b.repo.CountHosts(ctx)
	if err == nil && b.cfg != nil && count >= b.cfg.MaxHosts {
		msg := fmt.Sprintf("⚠️ Monitoring limit reached.\n\nMaximum:\n%d hosts", b.cfg.MaxHosts)
		_ = b.client.SendMessage(ctx, chatID, msg)
		return
	}

	validated, userErrMsg, err := security.ValidateAndSanitizeInput(ctx, rawTarget)
	if err != nil {
		slog.Warn("Rejected add input", "target", rawTarget, "error", err)
		_ = b.client.SendMessage(ctx, chatID, userErrMsg)
		return
	}

	host, err := b.repo.AddHost(ctx, validated.Normalized, validated.Type)
	if err != nil {
		if errors.Is(err, database.ErrHostAlreadyExists) {
			_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' sudah terdaftar di daftar monitoring.", validated.Normalized))
			return
		}
		slog.Error("Failed to save host to database", "host", validated.Normalized, "error", err)
		_ = b.client.SendMessage(ctx, chatID, "❌ Gagal menambahkan host ke database.")
		return
	}

	slog.Info("Host added to monitoring", "host", host.Host, "type", host.HostType)
	reply := fmt.Sprintf("✅ Berhasil menambahkan '%s' (%s) ke daftar monitoring otomatis.", host.Host, host.HostType)
	_ = b.client.SendMessage(ctx, chatID, reply)
}

func (b *Bot) handleRemove(ctx context.Context, chatID int64, rawTarget string) {
	if rawTarget == "" {
		_ = b.client.SendMessage(ctx, chatID, "ℹ️ Format penggunaan:\n/remove <host>\n\nContoh:\n/remove example.com")
		return
	}

	if b.repo == nil {
		_ = b.client.SendMessage(ctx, chatID, "❌ Database belum siap.")
		return
	}

	hType := monitor.DetectHostType(rawTarget)
	normalized := monitor.NormalizeHost(rawTarget, hType)

	err := b.repo.RemoveHost(ctx, normalized)
	if err != nil {
		if errors.Is(err, database.ErrHostNotFound) {
			_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' tidak ditemukan di daftar monitoring.", normalized))
			return
		}
		slog.Error("Failed to remove host", "host", normalized, "error", err)
		_ = b.client.SendMessage(ctx, chatID, "❌ Gagal menghapus host.")
		return
	}

	slog.Info("Host removed from monitoring", "host", normalized)
	_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("🗑️ Host '%s' berhasil dihapus dari monitoring.", normalized))
}

func (b *Bot) handleList(ctx context.Context, chatID int64) {
	if b.repo == nil {
		_ = b.client.SendMessage(ctx, chatID, "❌ Database belum siap.")
		return
	}

	hosts, err := b.repo.ListHosts(ctx)
	if err != nil {
		slog.Error("Failed to fetch host list", "error", err)
		_ = b.client.SendMessage(ctx, chatID, "❌ Gagal mengambil daftar host.")
		return
	}

	maxHosts := 50
	if b.cfg != nil && b.cfg.MaxHosts > 0 {
		maxHosts = b.cfg.MaxHosts
	}

	formatted := FormatHostList(hosts, maxHosts)
	_ = b.client.SendMessage(ctx, chatID, formatted)
}

func (b *Bot) handleStatus(ctx context.Context, chatID int64, rawTarget string) {
	if rawTarget == "" {
		_ = b.client.SendMessage(ctx, chatID, "ℹ️ Format penggunaan:\n/status <host>\n\nContoh:\n/status example.com")
		return
	}

	if b.repo == nil {
		_ = b.client.SendMessage(ctx, chatID, "❌ Database belum siap.")
		return
	}

	hType := monitor.DetectHostType(rawTarget)
	normalized := monitor.NormalizeHost(rawTarget, hType)

	host, err := b.repo.GetHost(ctx, normalized)
	if err != nil {
		if errors.Is(err, database.ErrHostNotFound) {
			_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' tidak ditemukan di daftar monitoring.\n\nGunakan /add <host> untuk menambahkan.", normalized))
			return
		}
		slog.Error("Failed to get host status", "host", normalized, "error", err)
		_ = b.client.SendMessage(ctx, chatID, "❌ Gagal mengambil status host.")
		return
	}

	_ = b.client.SendMessage(ctx, chatID, FormatHostDetailStatus(host))
}

func (b *Bot) handleSetMonitoring(ctx context.Context, chatID int64, rawTarget string, enabled bool) {
	cmdName := "/monitor"
	if !enabled {
		cmdName = "/unmonitor"
	}

	if rawTarget == "" {
		_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("ℹ️ Format penggunaan:\n%s <host>\n\nContoh:\n%s example.com", cmdName, cmdName))
		return
	}

	if b.repo == nil {
		_ = b.client.SendMessage(ctx, chatID, "❌ Database belum siap.")
		return
	}

	hType := monitor.DetectHostType(rawTarget)
	normalized := monitor.NormalizeHost(rawTarget, hType)

	err := b.repo.SetHostEnabled(ctx, normalized, enabled)
	if err != nil {
		if errors.Is(err, database.ErrHostNotFound) {
			_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' tidak ditemukan di database.", normalized))
			return
		}
		slog.Error("Failed to update host monitoring state", "host", normalized, "error", err)
		_ = b.client.SendMessage(ctx, chatID, "❌ Gagal mengubah status monitoring.")
		return
	}

	if enabled {
		_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("🟢 Monitoring otomatis untuk '%s' telah diaktifkan kembali.", normalized))
	} else {
		_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("⏸️ Monitoring otomatis untuk '%s' dijeda.", normalized))
	}
}

func (b *Bot) handleAddUser(ctx context.Context, chatID int64, rawArgs string) {
	parts := strings.Fields(rawArgs)
	if len(parts) == 0 {
		_ = b.client.SendMessage(ctx, chatID, "ℹ️ Format penggunaan:\n/adduser <telegram_user_id> [admin/user]\n\nContoh:\n/adduser 987654321 admin\n/adduser 112233445 user")
		return
	}

	newID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || newID <= 0 {
		_ = b.client.SendMessage(ctx, chatID, "❌ User ID harus berupa angka positif.")
		return
	}

	role := database.RoleUser
	if len(parts) > 1 && strings.EqualFold(parts[1], "admin") {
		role = database.RoleAdmin
	}

	if b.repo == nil {
		_ = b.client.SendMessage(ctx, chatID, "❌ Database belum siap.")
		return
	}

	err = b.repo.AddAdminUser(ctx, newID, "User", "", false, role)
	if err != nil {
		slog.Error("Failed to add user", "user_id", newID, "role", role, "error", err)
		_ = b.client.SendMessage(ctx, chatID, "❌ Gagal mendaftarkan user.")
		return
	}

	roleEmoji := "👤 USER (Viewer / Read-Only)"
	if role == database.RoleAdmin {
		roleEmoji = "🛡️ ADMIN (Full Access)"
	}

	_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("✅ User ID %d berhasil didaftarkan dengan role %s.", newID, roleEmoji))
}

func (b *Bot) handleRemoveUser(ctx context.Context, chatID int64, rawUserID string) {
	if rawUserID == "" {
		_ = b.client.SendMessage(ctx, chatID, "ℹ️ Format penggunaan:\n/removeuser <telegram_user_id>\n\nContoh:\n/removeuser 987654321")
		return
	}

	targetID, err := strconv.ParseInt(rawUserID, 10, 64)
	if err != nil || targetID <= 0 {
		_ = b.client.SendMessage(ctx, chatID, "❌ User ID harus berupa angka positif.")
		return
	}

	if b.repo == nil {
		_ = b.client.SendMessage(ctx, chatID, "❌ Database belum siap.")
		return
	}

	err = b.repo.RemoveAdminUser(ctx, targetID)
	if err != nil {
		if errors.Is(err, database.ErrUserNotFound) {
			_ = b.client.SendMessage(ctx, chatID, "⚠️ User tidak ditemukan atau merupakan Owner (Owner tidak dapat dihapus).")
			return
		}
		slog.Error("Failed to remove admin user", "user_id", targetID, "error", err)
		_ = b.client.SendMessage(ctx, chatID, "❌ Gagal mencabut akses user.")
		return
	}

	_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("🗑️ Akses untuk User ID %d berhasil dicabut.", targetID))
}

func (b *Bot) handleListUsers(ctx context.Context, chatID int64) {
	if b.repo == nil {
		_ = b.client.SendMessage(ctx, chatID, "❌ Database belum siap.")
		return
	}

	users, err := b.repo.ListAdminUsers(ctx)
	if err != nil {
		slog.Error("Failed to list admin users", "error", err)
		_ = b.client.SendMessage(ctx, chatID, "❌ Gagal memuat daftar admin.")
		return
	}

	var envIDs map[int64]bool
	if b.cfg != nil {
		envIDs = b.cfg.AllowedUserIDs
	}

	formatted := FormatUserList(users, envIDs)
	_ = b.client.SendMessage(ctx, chatID, formatted)
}
