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
		{Text: "⚡ Cek Cepat"},
	})

	if role == database.RoleOwner || role == database.RoleAdmin {
		rows = append(rows, []KeyboardButton{
			{Text: "➕ Tambah Host"},
			{Text: "📊 Status Host"},
		})
	}

	if role == database.RoleOwner {
		rows = append(rows, []KeyboardButton{
			{Text: "👥 Kelola User"},
			{Text: "❓ Panduan"},
		})
	} else {
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
1. Domain (contoh: example.com, google.com)
2. Public IPv4 (contoh: 123.123.123.123)
3. Public IPv6 (contoh: 2001:db8::1)

Perintah Umum (Semua Role):
/start            Menampilkan menu utama
/check <host>     Mengecek kondisi host secara instan
/list             Menampilkan semua host yang dimonitor
/status <host>    Melihat status terakhir & riwayat di database
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
	case "⚡ Cek Cepat", "⚡ Cek Host":
		return "/check", ""
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
