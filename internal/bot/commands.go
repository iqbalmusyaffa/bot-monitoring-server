package bot

import (
	"fmt"
	"strings"
	"time"

	"host-monitor/internal/database"
	"host-monitor/internal/monitor"
)

// StartMessage returns the welcoming text for /start.
func StartMessage() string {
	return `🖥️ HOST MONITOR BOT

Monitor domain dan public IP langsung dari Telegram.

Gunakan tombol menu di bawah atau ketik perintah:`
}

// DefaultBotCommands returns list of commands for Telegram Menu button.
func DefaultBotCommands() []BotCommand {
	return []BotCommand{
		{Command: "start", Description: "Mulai & Tampilkan Menu"},
		{Command: "list", Description: "Daftar Host yang Dimonitor"},
		{Command: "check", Description: "Cek Host: /check <domain/ip>"},
		{Command: "status", Description: "Status Host: /status <host>"},
		{Command: "uptime", Description: "Uptime & SLA: /uptime [host] [durasi]"},
		{Command: "whois", Description: "Cek Domain & Expired: /whois <domain>"},
		{Command: "add", Description: "Tambah Host: /add <host>"},
		{Command: "remove", Description: "Hapus Host: /remove <host>"},
		{Command: "monitor", Description: "Aktifkan Monitor: /monitor <host>"},
		{Command: "unmonitor", Description: "Jeda Monitor: /unmonitor <host>"},
		{Command: "listusers", Description: "Daftar User & Role (Owner)"},
		{Command: "adduser", Description: "Tambah User: /adduser <id> [role]"},
		{Command: "removeuser", Description: "Hapus User: /removeuser <id>"},
		{Command: "help", Description: "Panduan & Bantuan Lengkap"},
	}
}

// DefaultReplyKeyboard creates interactive button layout at bottom of chat.
func DefaultReplyKeyboard(role database.UserRole) *ReplyKeyboardMarkup {
	var rows [][]KeyboardButton

	rows = append(rows, []KeyboardButton{
		{Text: "📋 Daftar Host"},
		{Text: "📈 Uptime & SLA"},
	})

	rows = append(rows, []KeyboardButton{
		{Text: "⚡ Cek Cepat"},
		{Text: "🌐 Cek WHOIS"},
	})

	if role == database.RoleOwner || role == database.RoleAdmin {
		rows = append(rows, []KeyboardButton{
			{Text: "➕ Tambah Host"},
			{Text: "📊 Status Host"},
		})
	} else {
		rows = append(rows, []KeyboardButton{
			{Text: "📊 Status Host"},
			{Text: "❓ Panduan"},
		})
	}

	if role == database.RoleOwner {
		rows = append(rows, []KeyboardButton{
			{Text: "👥 Kelola User"},
			{Text: "❓ Panduan"},
		})
	} else if role == database.RoleAdmin {
		rows = append(rows, []KeyboardButton{
			{Text: "❓ Panduan"},
		})
	}

	return &ReplyKeyboardMarkup{
		Keyboard:       rows,
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

// HelpMessage returns the help guide text for /help.
func HelpMessage() string {
	return `📖 PANDUAN PENGGUNAAN

Target yang didukung:
1. Domain: example.com atau example.com:8080
2. Public IPv4: 123.123.123.123 atau 123.123.123.123:3306
3. Public IPv6: 2001:db8::1 atau [2001:db8::1]:8443

Perintah Umum (Semua Role):
/start            Menampilkan menu utama
/check <host>     Mengecek kondisi host secara instan
/list             Menampilkan semua host yang dimonitor
/status <host>    Melihat status terakhir & riwayat di database
/uptime [host]    Laporan Uptime & evaluasi SLA (opsi durasi: 24h, 7d, 30d)
/whois <domain>   Mengecek masa aktif, expired date & registrar domain
/help             Menampilkan panduan ini

Perintah Admin & Owner:
/add <host>       Mendaftarkan host ke monitoring otomatis
/remove <host>    Menghapus host dari daftar monitoring
/monitor <host>   Mengaktifkan kembali monitoring host
/unmonitor <host> Menjeda monitoring otomatis untuk host

Perintah Khusus Owner (Super Admin):
/listusers                Melihat daftar user & role
/adduser <id> [admin/user] Memberikan akses ke user baru
/removeuser <id>          Mencabut akses user`
}

// FormatHostList formats the /list response.
func FormatHostList(hosts []database.Host, maxHosts int) string {
	if len(hosts) == 0 {
		return "📋 Belum ada host yang didaftarkan.\n\nGunakan /add <host> untuk menambahkan."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 DAFTAR HOST DIMONITOR (%d/%d):\n\n", len(hosts), maxHosts))

	for i, h := range hosts {
		statusEmoji := "⚪"
		switch h.LastStatus {
		case monitor.StatusOnline:
			statusEmoji = "🟢"
		case monitor.StatusWarning:
			statusEmoji = "🟡"
		case monitor.StatusServerError, monitor.StatusTLSError, monitor.StatusUnreachable, monitor.StatusDown:
			statusEmoji = "🔴"
		}

		monitorStatus := ""
		if !h.Enabled {
			monitorStatus = " [PAUSED]"
		}

		sb.WriteString(fmt.Sprintf("%d. %s %s (%s)%s\n", i+1, statusEmoji, h.Host, h.HostType, monitorStatus))
	}

	sb.WriteString("\nGunakan /status <host> untuk melihat rincian.")
	return sb.String()
}

// FormatHostDetailStatus formats the /status <host> response.
func FormatHostDetailStatus(h *database.Host) string {
	var sb strings.Builder
	sb.WriteString("🖥️ STATUS HOST TERAKHIR\n\n")
	sb.WriteString(fmt.Sprintf("Host: %s\n", h.Host))
	sb.WriteString(fmt.Sprintf("Type: %s\n\n", h.HostType))

	if h.Enabled {
		sb.WriteString("Monitoring: 🟢 AKTIF\n\n")
	} else {
		sb.WriteString("Monitoring: ⏸️ DIJEDA\n\n")
	}

	statusEmoji := "⚪ UNKNOWN"
	switch h.LastStatus {
	case monitor.StatusOnline:
		statusEmoji = "🟢 ONLINE"
	case monitor.StatusWarning:
		statusEmoji = "🟡 WARNING"
	case monitor.StatusServerError:
		statusEmoji = "🔴 SERVER ERROR"
	case monitor.StatusTLSError:
		statusEmoji = "🔴 TLS ERROR"
	case monitor.StatusUnreachable:
		statusEmoji = "🔴 UNREACHABLE"
	case monitor.StatusDown:
		statusEmoji = "🔴 DOWN"
	}

	sb.WriteString(fmt.Sprintf("Status Terakhir:\n%s\n\n", statusEmoji))

	if h.LastResponseTime > 0 {
		sb.WriteString(fmt.Sprintf("Response Time: %d ms\n", h.LastResponseTime))
	}
	if h.LastHTTPStatus > 0 {
		sb.WriteString(fmt.Sprintf("HTTP Code: %d\n", h.LastHTTPStatus))
	}

	wibLoc := time.FixedZone("WIB", 7*3600)
	if h.LastCheckedAt != nil {
		sb.WriteString(fmt.Sprintf("Terakhir Dicek: %s\n", h.LastCheckedAt.In(wibLoc).Format("02 Jan 2006 15:04:05 WIB")))
	}
	if h.LastOnlineAt != nil {
		sb.WriteString(fmt.Sprintf("Terakhir Online: %s\n", h.LastOnlineAt.In(wibLoc).Format("02 Jan 2006 15:04:05 WIB")))
	}

	return sb.String()
}

// FormatUserList formats the /listusers response.
func FormatUserList(users []database.AdminUser, envUserIDs map[int64]bool) string {
	var sb strings.Builder
	sb.WriteString("👥 DAFTAR PENGGUNA RESMI BOT:\n\n")

	if len(users) == 0 && len(envUserIDs) == 0 {
		return "👥 Belum ada admin terdaftar. Pengguna pertama yang /start akan menjadi Owner."
	}

	count := 1
	for _, u := range users {
		badge := "👤 USER (Viewer)"
		if u.IsOwner || u.Role == database.RoleOwner {
			badge = "👑 OWNER"
		} else if u.Role == database.RoleAdmin {
			badge = "🛡️ ADMIN"
		}

		name := u.FirstName
		if name == "" {
			name = "Telegram User"
		}
		sb.WriteString(fmt.Sprintf("%d. %s - %s (ID: %d)\n", count, badge, name, u.UserID))
		count++
	}

	for id := range envUserIDs {
		var found bool
		for _, u := range users {
			if u.UserID == id {
				found = true
				break
			}
		}
		if !found {
			sb.WriteString(fmt.Sprintf("%d. 🛡️ ADMIN (.env) - (ID: %d)\n", count, id))
			count++
		}
	}

	return sb.String()
}

// ParseCommand splits an incoming message text into command and arguments.
func ParseCommand(text string) (cmd string, arg string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}

	// Map interactive button clicks to corresponding commands
	switch text {
	case "📋 Daftar Host", "📋 List Host":
		return "/list", ""
	case "📈 Uptime & SLA", "📈 Uptime", "📈 SLA":
		return "/uptime", ""
	case "⚡ Cek Cepat", "⚡ Cek Host":
		return "/check", ""
	case "🌐 Cek WHOIS", "🌐 WHOIS", "🌐 Domain":
		return "/whois", ""
	case "➕ Tambah Host":
		return "/add", ""
	case "📊 Status Host":
		return "/status", ""
	case "👥 Kelola User", "👥 List Users":
		return "/listusers", ""
	case "❓ Panduan", "❓ Bantuan":
		return "/help", ""
	}

	if !strings.HasPrefix(text, "/") {
		return "", ""
	}

	parts := strings.SplitN(text, " ", 2)
	cmd = parts[0]
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	if atIndex := strings.Index(cmd, "@"); atIndex != -1 {
		cmd = cmd[:atIndex]
	}

	return strings.ToLower(cmd), arg
}

// ParseUptimeArgs parses /uptime argument into target host, duration, and human-readable duration string.
func ParseUptimeArgs(arg string) (targetHost string, dur time.Duration, durStr string) {
	arg = strings.TrimSpace(arg)
	dur = 24 * time.Hour
	durStr = "24 Jam Terakhir"

	if arg == "" {
		return "", dur, durStr
	}

	parts := strings.Fields(arg)
	if len(parts) == 1 {
		d, s, ok := parseDurationToken(parts[0])
		if ok {
			return "", d, s
		}
		return parts[0], dur, durStr
	}

	targetHost = parts[0]
	d, s, ok := parseDurationToken(parts[1])
	if ok {
		dur = d
		durStr = s
	}

	return targetHost, dur, durStr
}

func parseDurationToken(token string) (time.Duration, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(token))
	switch lower {
	case "24h", "1d", "hari", "daily":
		return 24 * time.Hour, "24 Jam Terakhir", true
	case "48h", "2d":
		return 48 * time.Hour, "48 Jam Terakhir", true
	case "7d", "1w", "week", "minggu":
		return 7 * 24 * time.Hour, "7 Hari Terakhir", true
	case "14d", "2w":
		return 14 * 24 * time.Hour, "14 Hari Terakhir", true
	case "30d", "1m", "month", "bulan":
		return 30 * 24 * time.Hour, "30 Hari Terakhir", true
	default:
		if d, err := time.ParseDuration(lower); err == nil && d > 0 {
			hours := int(d.Hours())
			if hours%24 == 0 {
				days := hours / 24
				return d, fmt.Sprintf("%d Hari Terakhir", days), true
			}
			return d, fmt.Sprintf("%d Jam Terakhir", hours), true
		}
		return 0, "", false
	}
}

// FormatSLABadge returns SLA compliance level badge based on industry standards.
func FormatSLABadge(uptimePct float64) string {
	if uptimePct >= 99.9 {
		return "✅ PASSED (Tier 99.9%)"
	} else if uptimePct >= 99.0 {
		return "⚠️ WARNING (Tier 99.0%)"
	}
	return "❌ BREACHED (< 99.0%)"
}

// FormatUptimeSummary formats the global /uptime overview for all hosts.
func FormatUptimeSummary(stats []database.HostUptimeStats, durationStr string) string {
	if len(stats) == 0 {
		return fmt.Sprintf("📊 Belum ada host yang dimonitor untuk periode %s.\n\nGunakan /add <host> untuk mendaftarkan host.", durationStr)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 RINGKASAN UPTIME & SLA (%s)\n\n", durationStr))

	for i, s := range stats {
		statusEmoji := "⚪"
		switch s.CurrentStatus {
		case monitor.StatusOnline:
			statusEmoji = "🟢"
		case monitor.StatusWarning:
			statusEmoji = "🟡"
		case monitor.StatusServerError, monitor.StatusTLSError, monitor.StatusUnreachable, monitor.StatusDown:
			statusEmoji = "🔴"
		}

		slaBadge := FormatSLABadge(s.UptimePct)
		latencyStr := "-"
		if s.AvgLatencyMs > 0 {
			latencyStr = fmt.Sprintf("%d ms", s.AvgLatencyMs)
		}

		sb.WriteString(fmt.Sprintf("%d. %s %s (%s)\n", i+1, statusEmoji, s.Host, s.HostType))
		sb.WriteString(fmt.Sprintf("   • Uptime: %.2f%% | SLA: %s\n", s.UptimePct, slaBadge))
		sb.WriteString(fmt.Sprintf("   • Checks: %d (%d UP, %d DOWN) | Avg: %s\n\n", s.TotalChecks, s.OnlineChecks, s.DownChecks, latencyStr))
	}

	sb.WriteString("💡 Ketik /uptime <host> [24h/7d/30d] untuk melihat laporan detail per host.")
	return sb.String()
}

// FormatHostUptimeDetail formats the detailed /uptime <host> report.
func FormatHostUptimeDetail(host *database.Host, stats *database.HostUptimeStats, durationStr string, checkInterval time.Duration) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📈 LAPORAN UPTIME & SLA (%s)\n\n", durationStr))
	sb.WriteString(fmt.Sprintf("Target: %s\n", host.Host))
	sb.WriteString(fmt.Sprintf("Tipe: %s\n\n", host.HostType))

	statusStr := "⚪ UNKNOWN"
	switch host.LastStatus {
	case monitor.StatusOnline:
		statusStr = "🟢 ONLINE"
	case monitor.StatusWarning:
		statusStr = "🟡 WARNING"
	case monitor.StatusServerError:
		statusStr = "🔴 SERVER ERROR"
	case monitor.StatusTLSError:
		statusStr = "🔴 TLS ERROR"
	case monitor.StatusUnreachable:
		statusStr = "🔴 UNREACHABLE"
	case monitor.StatusDown:
		statusStr = "🔴 DOWN"
	}
	sb.WriteString(fmt.Sprintf("Status Terkini: %s\n\n", statusStr))

	sb.WriteString(fmt.Sprintf("⏱️ Uptime: %.2f%%\n", stats.UptimePct))
	sb.WriteString(fmt.Sprintf("🎯 Status SLA: %s\n", FormatSLABadge(stats.UptimePct)))

	if stats.AvgLatencyMs > 0 {
		sb.WriteString(fmt.Sprintf("⚡ Rata-rata Latensi: %d ms\n", stats.AvgLatencyMs))
	}
	sb.WriteString(fmt.Sprintf("🔄 Total Pemeriksaan: %d kali\n", stats.TotalChecks))
	sb.WriteString(fmt.Sprintf("✅ Sukses (ONLINE): %d kali\n", stats.OnlineChecks))

	if stats.DownChecks > 0 {
		estimatedDowntime := time.Duration(stats.DownChecks) * checkInterval
		sb.WriteString(fmt.Sprintf("📉 Gangguan (DOWN): %d kali (~%v)\n", stats.DownChecks, estimatedDowntime))
	} else {
		sb.WriteString("📉 Gangguan (DOWN): 0 kali (100% Zero Downtime)\n")
	}

	wibLoc := time.FixedZone("WIB", 7*3600)
	if host.LastCheckedAt != nil {
		sb.WriteString(fmt.Sprintf("\nTerakhir Dicek: %s", host.LastCheckedAt.In(wibLoc).Format("02 Jan 2006 15:04:05 WIB")))
	}
	if host.LastOnlineAt != nil {
		sb.WriteString(fmt.Sprintf("\nTerakhir Online: %s", host.LastOnlineAt.In(wibLoc).Format("02 Jan 2006 15:04:05 WIB")))
	}

	return sb.String()
}

// FormatWhoisReport formats the /whois response.
func FormatWhoisReport(rec *monitor.WhoisRecord) string {
	if rec.IsAvailable {
		return fmt.Sprintf("🌐 INFORMASI DOMAIN & WHOIS\nTarget: %s\n\n🟢 STATUS: TERSEDIA (BELUM DIDAFTARKAN)\nDomain ini belum diregistrasikan dan dapat didaftarkan melalui registrar pilihan Anda.", rec.Domain)
	}

	var sb strings.Builder
	sb.WriteString("🌐 INFORMASI DOMAIN & WHOIS\n")
	sb.WriteString(fmt.Sprintf("Target: %s\n\n", rec.Domain))

	if rec.Registrar != "" {
		sb.WriteString(fmt.Sprintf("🏢 Registrar     : %s\n", rec.Registrar))
	} else {
		sb.WriteString("🏢 Registrar     : -\n")
	}

	wibLoc := time.FixedZone("WIB", 7*3600)

	if rec.CreatedDate != nil {
		sb.WriteString(fmt.Sprintf("📅 Didaftarkan   : %s\n", rec.CreatedDate.In(wibLoc).Format("02 Jan 2006")))
	}
	if rec.UpdatedDate != nil {
		sb.WriteString(fmt.Sprintf("🔄 Diperbarui    : %s\n", rec.UpdatedDate.In(wibLoc).Format("02 Jan 2006")))
	}
	if rec.ExpiryDate != nil {
		sb.WriteString(fmt.Sprintf("⏳ Kedaluwarsa   : %s\n", rec.ExpiryDate.In(wibLoc).Format("02 Jan 2006")))
	}

	if rec.ExpiryDate != nil {
		if rec.IsExpired {
			daysAgo := -rec.DaysRemaining
			sb.WriteString(fmt.Sprintf("⏱️ Sisa Waktu    : 🔴 SUDAH KEDALUWARSA (%d hari yang lalu)\n", daysAgo))
		} else {
			statusUrgency := "🟢"
			if rec.DaysRemaining <= 7 {
				statusUrgency = "🔴 PERINGATAN KRITIS:"
			} else if rec.DaysRemaining <= 30 {
				statusUrgency = "🟡 PERINGATAN:"
			}
			sb.WriteString(fmt.Sprintf("⏱️ Sisa Waktu    : %s %d hari lagi\n", statusUrgency, rec.DaysRemaining))
		}
	}

	if len(rec.DomainStatus) > 0 {
		sb.WriteString(fmt.Sprintf("🔒 Status EPP    : %s\n", strings.Join(rec.DomainStatus, ", ")))
	}

	if len(rec.NameServers) > 0 {
		sb.WriteString("\n🌐 Name Servers  :\n")
		for _, ns := range rec.NameServers {
			sb.WriteString(fmt.Sprintf("   • %s\n", ns))
		}
	}

	if rec.WhoisServer != "" {
		sb.WriteString(fmt.Sprintf("\n📡 Source Server : %s", rec.WhoisServer))
	}

	return sb.String()
}
