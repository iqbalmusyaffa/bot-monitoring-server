package security

import (
	"context"
	"strings"
	"testing"
)

func TestValidateAndSanitizeInput(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errMsgMatch string
	}{
		{
			name:    "Valid Public Domain",
			input:   "google.com",
			wantErr: false,
		},
		{
			name:    "Valid Public IPv4",
			input:   "8.8.8.8",
			wantErr: false,
		},
		{
			name:    "Valid Public IPv6",
			input:   "2606:4700:4700::1111",
			wantErr: false,
		},
		{
			name:        "Empty input",
			input:       "",
			wantErr:     true,
			errMsgMatch: "Format target tidak valid",
		},
		{
			name:        "URL with Scheme (https://)",
			input:       "https://google.com",
			wantErr:     true,
			errMsgMatch: "Format target tidak valid",
		},
		{
			name:        "Host with Port",
			input:       "google.com:8080",
			wantErr:     true,
			errMsgMatch: "Format target tidak valid",
		},
		{
			name:        "Host with Path",
			input:       "google.com/index.html",
			wantErr:     true,
			errMsgMatch: "Format target tidak valid",
		},
		{
			name:        "Private IPv4 (127.0.0.1)",
			input:       "127.0.0.1",
			wantErr:     true,
			errMsgMatch: "Private/internal address tidak dapat dimonitor",
		},
		{
			name:        "Private IPv4 (192.168.1.1)",
			input:       "192.168.1.1",
			wantErr:     true,
			errMsgMatch: "Private/internal address tidak dapat dimonitor",
		},
		{
			name:        "Private IPv4 (10.0.0.1)",
			input:       "10.0.0.1",
			wantErr:     true,
			errMsgMatch: "Private/internal address tidak dapat dimonitor",
		},
		{
			name:        "Cloud Metadata IP (169.254.169.254)",
			input:       "169.254.169.254",
			wantErr:     true,
			errMsgMatch: "Private/internal address tidak dapat dimonitor",
		},
		{
			name:        "Private IPv6 (::1)",
			input:       "::1",
			wantErr:     true,
			errMsgMatch: "Private/internal address tidak dapat dimonitor",
		},
		{
			name:        "Private IPv6 (fe80::1)",
			input:       "fe80::1",
			wantErr:     true,
			errMsgMatch: "Private/internal address tidak dapat dimonitor",
		},
		{
			name:        "Extremely long input",
			input:       strings.Repeat("a", 300) + ".com",
			wantErr:     true,
			errMsgMatch: "Format target tidak valid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validated, userErrMsg, err := ValidateAndSanitizeInput(ctx, tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tc.input)
				}
				if !strings.Contains(userErrMsg, tc.errMsgMatch) {
					t.Errorf("expected user error message to contain %q, got %q", tc.errMsgMatch, userErrMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for input %q: %v", tc.input, err)
				}
				if validated == nil {
					t.Fatalf("expected validated host object, got nil")
				}
			}
		})
	}
}
