package notification

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"host-monitor/internal/bot"
	"host-monitor/internal/config"
	"host-monitor/internal/database"
	"host-monitor/internal/monitor"
)

// Notifier sends formatted alert notifications to all whitelisted Telegram users.
type Notifier struct {
	cfg    *config.Config
	client *bot.Client
	repo   *database.Repository
}

// NewNotifier creates a new Notifier instance.
func NewNotifier(cfg *config.Config, client *bot.Client, repo *database.Repository) *Notifier {
	return &Notifier{
		cfg:    cfg,
		client: client,
		repo:   repo,
	}
}

// FormatHostDownAlert formats the alert message when a host goes DOWN.
func FormatHostDownAlert(host string, status monitor.OverallStatus, detectedAt time.Time) string {
	wibLoc := time.FixedZone("WIB", 7*3600)
	timeStr := detectedAt.In(wibLoc).Format("15:04:05 WIB")

	var statusEmoji string
	switch status {
	case monitor.StatusServerError:
		statusEmoji = "🔴 SERVER ERROR"
	case monitor.StatusTLSError:
		statusEmoji = "🔴 TLS ERROR"
	case monitor.StatusUnreachable:
		statusEmoji = "🔴 UNREACHABLE"
	default:
		statusEmoji = "🔴 DOWN"
	}

	return fmt.Sprintf(`🚨 HOST DOWN

Host:
%s

Status:
%s

Detected:
%s`, host, statusEmoji, timeStr)
}

// FormatHostRecoveredAlert formats the alert message when a host recovers back ONLINE.
func FormatHostRecoveredAlert(host string, latencyMs int64, downtime time.Duration) string {
	downtimeStr := formatDuration(downtime)

	return fmt.Sprintf(`✅ HOST RECOVERED

Host:
%s

Status:
🟢 ONLINE

Response:
%d ms

Downtime:
%s`, host, latencyMs, downtimeStr)
}

// NotifyStatusChange dispatches the appropriate alert to all configured and registered admin user IDs.
func (n *Notifier) NotifyStatusChange(ctx context.Context, host string, oldStatus, newStatus monitor.OverallStatus, latencyMs int64, downSince *time.Time) {
	if n.client == nil {
		return
	}

	var message string
	now := time.Now()

	// Check transition: ONLINE/UNKNOWN -> DOWN
	if (oldStatus == monitor.StatusOnline || oldStatus == "") && isDownStatus(newStatus) {
		message = FormatHostDownAlert(host, newStatus, now)
	} else if isDownStatus(oldStatus) && newStatus == monitor.StatusOnline {
		// Transition: DOWN -> ONLINE
		var downtime time.Duration
		if downSince != nil && !downSince.IsZero() {
			downtime = now.Sub(*downSince)
		}
		message = FormatHostRecoveredAlert(host, latencyMs, downtime)
	} else {
		return
	}

	slog.Info("Sending status change notification", "host", host, "from", oldStatus, "to", newStatus)

	// Collect unique recipient user IDs from both .env and SQLite DB
	recipients := make(map[int64]bool)
	if n.cfg != nil {
		for id := range n.cfg.AllowedUserIDs {
			recipients[id] = true
		}
	}
	if n.repo != nil {
		dbIDs, err := n.repo.ListAdminUserIDs(ctx)
		if err == nil {
			for _, id := range dbIDs {
				recipients[id] = true
			}
		}
	}

	for userID := range recipients {
		if err := n.client.SendMessage(ctx, userID, message); err != nil {
			slog.Error("Failed to dispatch alert to user", "user_id", userID, "error", err)
		}
	}
}

func isDownStatus(st monitor.OverallStatus) bool {
	return st == monitor.StatusDown ||
		st == monitor.StatusServerError ||
		st == monitor.StatusUnreachable ||
		st == monitor.StatusTLSError
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "1m"
	}
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
