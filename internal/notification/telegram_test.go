package notification

import (
	"strings"
	"testing"
	"time"

	"host-monitor/internal/monitor"
)

func TestFormatHostDownAlert(t *testing.T) {
	detectedAt := time.Date(2026, 10, 3, 23, 44, 12, 0, time.UTC)
	msg := FormatHostDownAlert("example.com", monitor.StatusDown, detectedAt)

	if !strings.Contains(msg, "🚨 HOST DOWN") || !strings.Contains(msg, "example.com") || !strings.Contains(msg, "🔴 DOWN") {
		t.Errorf("unexpected down alert format: %s", msg)
	}
}

func TestFormatHostRecoveredAlert(t *testing.T) {
	msg := FormatHostRecoveredAlert("example.com", 31, 2*time.Minute+14*time.Second)

	if !strings.Contains(msg, "✅ HOST RECOVERED") ||
		!strings.Contains(msg, "example.com") ||
		!strings.Contains(msg, "🟢 ONLINE") ||
		!strings.Contains(msg, "31 ms") ||
		!strings.Contains(msg, "2m 14s") {
		t.Errorf("unexpected recovered alert format: %s", msg)
	}
}
