package security

import (
	"context"
	"errors"
	"strings"

	"host-monitor/internal/monitor"
)

const (
	// MaxHostInputLength defines the max allowed characters for target input (RFC DNS limit).
	MaxHostInputLength = 253

	// RejectPrivateTargetMessage is the standard rejection text for private/internal IPs.
	RejectPrivateTargetMessage = `❌ Target tidak diperbolehkan.

Private/internal address tidak dapat dimonitor.`

	// RejectInvalidHostMessage is the standard rejection text for malformed input.
	RejectInvalidHostMessage = `❌ Format target tidak valid.

Gunakan format:
* Domain: example.com
* IPv4: 123.123.123.123
* IPv6: 2001:db8::1

(Jangan menyertakan http://, https://, path, atau port)`
)

// ValidatedHost contains the sanitized host and its detected type.
type ValidatedHost struct {
	RawHost    string
	Normalized string
	Type       monitor.HostType
}

// ValidateAndSanitizeInput validates raw user input from Telegram commands,
// rejects dangerous or malformed input, and runs SSRF checks.
func ValidateAndSanitizeInput(ctx context.Context, rawInput string) (*ValidatedHost, string, error) {
	rawInput = strings.TrimSpace(rawInput)

	if rawInput == "" || len(rawInput) > MaxHostInputLength {
		return nil, RejectInvalidHostMessage, errors.New("empty or excessively long host input")
	}

	hType := monitor.DetectHostType(rawInput)
	if hType == monitor.HostUnknown {
		return nil, RejectInvalidHostMessage, errors.New("malformed or unsupported host format")
	}

	normalized := monitor.NormalizeHost(rawInput, hType)

	// Perform SSRF validation
	if err := ValidateTargetSecurity(ctx, normalized, hType, nil); err != nil {
		if errors.Is(err, ErrPrivateTarget) {
			return nil, RejectPrivateTargetMessage, err
		}
		// If DNS resolution fails completely for domain
		return nil, "❌ Gagal me-resolve DNS target. Pastikan domain terdaftar dan aktif.", err
	}

	return &ValidatedHost{
		RawHost:    rawInput,
		Normalized: normalized,
		Type:       hType,
	}, "", nil
}
