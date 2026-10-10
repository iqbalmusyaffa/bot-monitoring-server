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
	whoisClient *monitor.WhoisClient
	repo        *database.Repository
}

// NewBot instantiates a Bot instance.
func NewBot(cfg *config.Config, client *Client, repo *database.Repository) *Bot {
	var limit = 10
	var window = 1 * time.Minute
	var timeout = 10 * time.Second
	if cfg != nil {
		if cfg.CommandRateLimit > 0 {
			limit = cfg.CommandRateLimit
		}
		if cfg.CommandRateWindow > 0 {
			window = cfg.CommandRateWindow
		}
		if cfg.RequestTimeout > 0 {
			timeout = cfg.RequestTimeout
		}
	}

	return &Bot{
		cfg:         cfg,
		client:      client,
		auth:        NewAuthenticator(cfg, repo),
		rateLimiter: security.NewRateLimiter(limit, window),
		checker:     monitor.NewChecker(cfg),
		whoisClient: monitor.NewWhoisClient(timeout),
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

	// Automatically register Telegram Menu commands
	if err := b.client.SetMyCommands(ctx, DefaultBotCommands()); err != nil {
		slog.Warn("Failed to register bot commands with Telegram", "error", err)
	} else {
		slog.Info("Telegram menu commands registered successfully")
	}

	// Configure [ Menu ] button pill on bottom-left of input box
	if err := b.client.SetChatMenuButton(ctx, 0, "commands"); err != nil {
		slog.Warn("Failed to configure chat menu button", "error", err)
	} else {
		slog.Info("Telegram [ Menu ] button configured successfully")
	}

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
			if update.CallbackQuery != nil {
				b.handleCallbackQuery(ctx, update.CallbackQuery)
			}
		}
	}
}

// handleCallbackQuery processes button clicks from inline keyboards.
func (b *Bot) handleCallbackQuery(ctx context.Context, cb *CallbackQuery) {
	if cb == nil {
		return
	}
	_ = b.client.AnswerCallbackQuery(ctx, cb.ID, "")
	if cb.Message != nil && cb.Data != "" {
		syntheticMsg := &Message{
			MessageID: cb.Message.MessageID,
			From:      &cb.From,
			Chat:      cb.Message.Chat,
			Text:      cb.Data,
			Date:      time.Now().Unix(),
		}
		b.handleMessage(ctx, syntheticMsg)
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
		b.reply(ctx, msg.Chat.ID, StartMessage(), role)
	case "/help":
		b.reply(ctx, msg.Chat.ID, HelpMessage(), role)
	case "/check":
		b.handleCheck(ctx, msg.Chat.ID, arg, role)
	case "/list":
		b.handleList(ctx, msg.Chat.ID, role)
	case "/status":
		b.handleStatus(ctx, msg.Chat.ID, arg, role)
	case "/uptime", "/sla":
		b.handleUptime(ctx, msg.Chat.ID, arg, role)
	case "/whois", "/domain":
		b.handleWhois(ctx, msg.Chat.ID, arg, role)

	// Admin / Owner level commands
	case "/add":
		if role == database.RoleUser {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Admin/Owner", role)
			return
		}
		b.handleAdd(ctx, msg.Chat.ID, arg, role)
	case "/remove":
		if role == database.RoleUser {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Admin/Owner", role)
			return
		}
		b.handleRemove(ctx, msg.Chat.ID, arg, role)
	case "/monitor":
		if role == database.RoleUser {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Admin/Owner", role)
			return
		}
		b.handleSetMonitoring(ctx, msg.Chat.ID, arg, true, role)
	case "/unmonitor":
		if role == database.RoleUser {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Admin/Owner", role)
			return
		}
		b.handleSetMonitoring(ctx, msg.Chat.ID, arg, false, role)

	// Owner level commands
	case "/adduser":
		if role != database.RoleOwner {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Owner", role)
			return
		}
		b.handleAddUser(ctx, msg.Chat.ID, arg, role)
	case "/removeuser":
		if role != database.RoleOwner {
			b.replyForbiddenRole(ctx, msg.Chat.ID, "Owner", role)
			return
		}
		b.handleRemoveUser(ctx, msg.Chat.ID, arg, role)
	case "/listusers":
		b.handleListUsers(ctx, msg.Chat.ID, role)

	default:
		response := fmt.Sprintf("Perintah '%s' tidak dikenali. Ketik /help untuk melihat menu perintah.", cmd)
		b.reply(ctx, msg.Chat.ID, response, role)
	}
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string, role database.UserRole) {
	_ = b.client.SendMessageWithMarkup(ctx, chatID, text, DefaultReplyKeyboard(role))
}

func (b *Bot) replyForbiddenRole(ctx context.Context, chatID int64, requiredRole string, role database.UserRole) {
	msg := fmt.Sprintf("⚠️ Akses Ditolak.\n\nPerintah ini hanya dapat dijalankan oleh Role **%s**.\nRole Anda saat ini memiliki izin terbatas (Read-Only / User).", requiredRole)
	b.reply(ctx, chatID, msg, role)
}

func (b *Bot) handleCheck(ctx context.Context, chatID int64, rawTarget string, role database.UserRole) {
	if rawTarget == "" {
		b.reply(ctx, chatID, "ℹ️ Format penggunaan:\n/check <host>\n\nContoh:\n/check example.com\n/check 123.123.123.123\n/check 2001:db8::1", role)
		return
	}

	validated, userErrMsg, err := security.ValidateAndSanitizeInput(ctx, rawTarget)
	if err != nil {
		slog.Warn("Rejected check input", "target", rawTarget, "error", err)
		b.reply(ctx, chatID, userErrMsg, role)
		return
	}

	slog.Info("Executing /check", "host", validated.Normalized, "type", validated.Type)

	result := b.checker.CheckHost(ctx, validated.Normalized, validated.Type)
	formattedResponse := monitor.FormatCheckResult(result)

	b.reply(ctx, chatID, formattedResponse, role)
}

func (b *Bot) handleAdd(ctx context.Context, chatID int64, rawTarget string, role database.UserRole) {
	if rawTarget == "" {
		b.reply(ctx, chatID, "ℹ️ Format penggunaan:\n/add <host>\n\nContoh:\n/add example.com\n/add 123.123.123.123\n/add 2001:db8::1", role)
		return
	}

	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	count, err := b.repo.CountHosts(ctx)
	if err == nil && b.cfg != nil && count >= b.cfg.MaxHosts {
		msg := fmt.Sprintf("⚠️ Monitoring limit reached.\n\nMaximum:\n%d hosts", b.cfg.MaxHosts)
		b.reply(ctx, chatID, msg, role)
		return
	}

	validated, userErrMsg, err := security.ValidateAndSanitizeInput(ctx, rawTarget)
	if err != nil {
		slog.Warn("Rejected add input", "target", rawTarget, "error", err)
		b.reply(ctx, chatID, userErrMsg, role)
		return
	}

	host, err := b.repo.AddHost(ctx, validated.Normalized, validated.Type)
	if err != nil {
		if errors.Is(err, database.ErrHostAlreadyExists) {
			b.reply(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' sudah terdaftar di daftar monitoring.", validated.Normalized), role)
			return
		}
		slog.Error("Failed to save host to database", "host", validated.Normalized, "error", err)
		b.reply(ctx, chatID, "❌ Gagal menambahkan host ke database.", role)
		return
	}

	slog.Info("Host added to monitoring", "host", host.Host, "type", host.HostType)
	reply := fmt.Sprintf("✅ Berhasil menambahkan '%s' (%s) ke daftar monitoring otomatis.", host.Host, host.HostType)
	b.reply(ctx, chatID, reply, role)
}

func (b *Bot) handleRemove(ctx context.Context, chatID int64, rawTarget string, role database.UserRole) {
	if rawTarget == "" {
		b.reply(ctx, chatID, "ℹ️ Format penggunaan:\n/remove <host>\n\nContoh:\n/remove example.com", role)
		return
	}

	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	hType := monitor.DetectHostType(rawTarget)
	normalized := monitor.NormalizeHost(rawTarget, hType)

	err := b.repo.RemoveHost(ctx, normalized)
	if err != nil {
		if errors.Is(err, database.ErrHostNotFound) {
			b.reply(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' tidak ditemukan di daftar monitoring.", normalized), role)
			return
		}
		slog.Error("Failed to remove host", "host", normalized, "error", err)
		b.reply(ctx, chatID, "❌ Gagal menghapus host.", role)
		return
	}

	slog.Info("Host removed from monitoring", "host", normalized)
	b.reply(ctx, chatID, fmt.Sprintf("🗑️ Host '%s' berhasil dihapus dari monitoring.", normalized), role)
}

func (b *Bot) handleList(ctx context.Context, chatID int64, role database.UserRole) {
	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	hosts, err := b.repo.ListHosts(ctx)
	if err != nil {
		slog.Error("Failed to fetch host list", "error", err)
		b.reply(ctx, chatID, "❌ Gagal mengambil daftar host.", role)
		return
	}

	maxHosts := 50
	if b.cfg != nil && b.cfg.MaxHosts > 0 {
		maxHosts = b.cfg.MaxHosts
	}

	formatted := FormatHostList(hosts, maxHosts)
	b.reply(ctx, chatID, formatted, role)
}

func (b *Bot) handleStatus(ctx context.Context, chatID int64, rawTarget string, role database.UserRole) {
	if rawTarget == "" {
		b.reply(ctx, chatID, "ℹ️ Format penggunaan:\n/status <host>\n\nContoh:\n/status example.com", role)
		return
	}

	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	hType := monitor.DetectHostType(rawTarget)
	normalized := monitor.NormalizeHost(rawTarget, hType)

	host, err := b.repo.GetHost(ctx, normalized)
	if err != nil {
		if errors.Is(err, database.ErrHostNotFound) {
			b.reply(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' tidak ditemukan di daftar monitoring.\n\nGunakan /add <host> untuk menambahkan.", normalized), role)
			return
		}
		slog.Error("Failed to get host status", "host", normalized, "error", err)
		b.reply(ctx, chatID, "❌ Gagal mengambil status host.", role)
		return
	}

	b.reply(ctx, chatID, FormatHostDetailStatus(host), role)
}

func (b *Bot) handleUptime(ctx context.Context, chatID int64, arg string, role database.UserRole) {
	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	targetHost, dur, durStr := ParseUptimeArgs(arg)
	since := time.Now().Add(-dur)

	if targetHost == "" {
		// Global summary for all active hosts
		stats, err := b.repo.GetAllHostsUptime(ctx, since)
		if err != nil {
			slog.Error("Failed to fetch all hosts uptime", "error", err)
			b.reply(ctx, chatID, "❌ Gagal mengambil ringkasan uptime.", role)
			return
		}
		b.reply(ctx, chatID, FormatUptimeSummary(stats, durStr), role)
		return
	}

	// Specific host detail
	hType := monitor.DetectHostType(targetHost)
	normalized := monitor.NormalizeHost(targetHost, hType)

	host, err := b.repo.GetHost(ctx, normalized)
	if err != nil {
		if errors.Is(err, database.ErrHostNotFound) {
			b.reply(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' tidak ditemukan di daftar monitoring.\n\nGunakan /add <host> untuk mendaftarkan.", normalized), role)
			return
		}
		slog.Error("Failed to get host for uptime", "host", normalized, "error", err)
		b.reply(ctx, chatID, "❌ Gagal mengambil data host.", role)
		return
	}

	stats, err := b.repo.GetHostUptime(ctx, host.ID, since)
	if err != nil {
		slog.Error("Failed to calculate host uptime", "host", normalized, "error", err)
		b.reply(ctx, chatID, "❌ Gagal menghitung uptime host.", role)
		return
	}

	checkInterval := 1 * time.Minute
	if b.cfg != nil && b.cfg.CheckInterval > 0 {
		checkInterval = b.cfg.CheckInterval
	}

	b.reply(ctx, chatID, FormatHostUptimeDetail(host, stats, durStr, checkInterval), role)
}

func (b *Bot) handleWhois(ctx context.Context, chatID int64, rawTarget string, role database.UserRole) {
	if rawTarget == "" {
		b.reply(ctx, chatID, "ℹ️ Format penggunaan:\n/whois <domain>\n\nContoh:\n/whois kompas.id\n/whois google.com\n/whois bca.co.id", role)
		return
	}

	cleaned, err := monitor.CleanDomain(rawTarget)
	if err != nil {
		b.reply(ctx, chatID, "⚠️ Format domain tidak valid. Pastikan memasukkan nama domain (contoh: example.id atau google.com).", role)
		return
	}

	_ = b.client.SendMessage(ctx, chatID, fmt.Sprintf("🔍 Memeriksa data WHOIS untuk '%s'...", cleaned))

	rec, err := b.whoisClient.Lookup(ctx, cleaned)
	if err != nil {
		slog.Error("Failed to lookup whois", "domain", cleaned, "error", err)
		b.reply(ctx, chatID, fmt.Sprintf("❌ Gagal memeriksa WHOIS untuk '%s':\n%v", cleaned, err), role)
		return
	}

	b.reply(ctx, chatID, FormatWhoisReport(rec), role)
}

func (b *Bot) handleSetMonitoring(ctx context.Context, chatID int64, rawTarget string, enabled bool, role database.UserRole) {
	cmdName := "/monitor"
	if !enabled {
		cmdName = "/unmonitor"
	}

	if rawTarget == "" {
		b.reply(ctx, chatID, fmt.Sprintf("ℹ️ Format penggunaan:\n%s <host>\n\nContoh:\n%s example.com", cmdName, cmdName), role)
		return
	}

	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	hType := monitor.DetectHostType(rawTarget)
	normalized := monitor.NormalizeHost(rawTarget, hType)

	err := b.repo.SetHostEnabled(ctx, normalized, enabled)
	if err != nil {
		if errors.Is(err, database.ErrHostNotFound) {
			b.reply(ctx, chatID, fmt.Sprintf("⚠️ Host '%s' tidak ditemukan di database.", normalized), role)
			return
		}
		slog.Error("Failed to update host monitoring state", "host", normalized, "error", err)
		b.reply(ctx, chatID, "❌ Gagal mengubah status monitoring.", role)
		return
	}

	if enabled {
		b.reply(ctx, chatID, fmt.Sprintf("🟢 Monitoring otomatis untuk '%s' telah diaktifkan kembali.", normalized), role)
	} else {
		b.reply(ctx, chatID, fmt.Sprintf("⏸️ Monitoring otomatis untuk '%s' dijeda.", normalized), role)
	}
}

func (b *Bot) handleAddUser(ctx context.Context, chatID int64, rawArgs string, role database.UserRole) {
	parts := strings.Fields(rawArgs)
	if len(parts) == 0 {
		b.reply(ctx, chatID, "ℹ️ Format penggunaan:\n/adduser <telegram_user_id> [admin/user]\n\nContoh:\n/adduser 987654321 admin\n/adduser 112233445 user", role)
		return
	}

	newID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || newID <= 0 {
		b.reply(ctx, chatID, "❌ User ID harus berupa angka positif.", role)
		return
	}

	targetRole := database.RoleUser
	if len(parts) > 1 && strings.EqualFold(parts[1], "admin") {
		targetRole = database.RoleAdmin
	}

	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	err = b.repo.AddAdminUser(ctx, newID, "User", "", false, targetRole)
	if err != nil {
		slog.Error("Failed to add user", "user_id", newID, "role", targetRole, "error", err)
		b.reply(ctx, chatID, "❌ Gagal mendaftarkan user.", role)
		return
	}

	roleEmoji := "👤 USER (Viewer / Read-Only)"
	if targetRole == database.RoleAdmin {
		roleEmoji = "🛡️ ADMIN (Full Access)"
	}

	b.reply(ctx, chatID, fmt.Sprintf("✅ User ID %d berhasil didaftarkan dengan role %s.", newID, roleEmoji), role)
}

func (b *Bot) handleRemoveUser(ctx context.Context, chatID int64, rawUserID string, role database.UserRole) {
	if rawUserID == "" {
		b.reply(ctx, chatID, "ℹ️ Format penggunaan:\n/removeuser <telegram_user_id>\n\nContoh:\n/removeuser 987654321", role)
		return
	}

	targetID, err := strconv.ParseInt(rawUserID, 10, 64)
	if err != nil || targetID <= 0 {
		b.reply(ctx, chatID, "❌ User ID harus berupa angka positif.", role)
		return
	}

	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	err = b.repo.RemoveAdminUser(ctx, targetID)
	if err != nil {
		if errors.Is(err, database.ErrUserNotFound) {
			b.reply(ctx, chatID, "⚠️ User tidak ditemukan atau merupakan Owner (Owner tidak dapat dihapus).", role)
			return
		}
		slog.Error("Failed to remove admin user", "user_id", targetID, "error", err)
		b.reply(ctx, chatID, "❌ Gagal mencabut akses user.", role)
		return
	}

	b.reply(ctx, chatID, fmt.Sprintf("🗑️ Akses untuk User ID %d berhasil dicabut.", targetID), role)
}

func (b *Bot) handleListUsers(ctx context.Context, chatID int64, role database.UserRole) {
	if b.repo == nil {
		b.reply(ctx, chatID, "❌ Database belum siap.", role)
		return
	}

	users, err := b.repo.ListAdminUsers(ctx)
	if err != nil {
		slog.Error("Failed to list admin users", "error", err)
		b.reply(ctx, chatID, "❌ Gagal memuat daftar admin.", role)
		return
	}

	var envIDs map[int64]bool
	if b.cfg != nil {
		envIDs = b.cfg.AllowedUserIDs
	}

	formatted := FormatUserList(users, envIDs)
	b.reply(ctx, chatID, formatted, role)
}
