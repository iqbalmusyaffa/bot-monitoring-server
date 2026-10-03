package monitor

import (
	"testing"
)

func TestDetectHostType(t *testing.T) {
	tests := []struct {
		input    string
		expected HostType
	}{
		// Valid Domains
		{"example.com", HostDomain},
		{"google.com", HostDomain},
		{"api.example.com", HostDomain},
		{"sub.domain.co.id", HostDomain},
		{"my-server-01.cloud.provider.net", HostDomain},
		{"UPPERCASE.COM", HostDomain},

		// Valid IPv4
		{"123.123.123.123", HostIPv4},
		{"1.1.1.1", HostIPv4},
		{"8.8.8.8", HostIPv4},
		{"192.168.1.1", HostIPv4},
		{"127.0.0.1", HostIPv4},

		// Valid IPv6
		{"2001:db8::1", HostIPv6},
		{"::1", HostIPv6},
		{"2001:0db8:85a3:0000:0000:8a2e:0370:7334", HostIPv6},
		{"fe80::1", HostIPv6},

		// Invalid Inputs / Malformed
		{"", HostUnknown},
		{"   ", HostUnknown},
		{"https://example.com", HostUnknown},
		{"http://123.123.123.123", HostUnknown},
		{"example.com/test", HostUnknown},
		{"example.com:8080", HostUnknown},
		{"example.com?query=1", HostUnknown},
		{"example..com", HostUnknown},
		{"-example.com", HostUnknown},
		{"example-.com", HostUnknown},
		{"invalid_domain.com", HostUnknown},
		{"999.999.999.999", HostUnknown},
		{"2001:xyz::1", HostUnknown},
		{"rm -rf /", HostUnknown},
		{"example.com; ls", HostUnknown},
		{"example.com && whoami", HostUnknown},
	}

	for _, tc := range tests {
		got := DetectHostType(tc.input)
		if got != tc.expected {
			t.Errorf("DetectHostType(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		input    string
		hType    HostType
		expected string
	}{
		{"Example.COM", HostDomain, "example.com"},
		{"API.Example.COM", HostDomain, "api.example.com"},
		{"1.1.1.1", HostIPv4, "1.1.1.1"},
		{"2001:0db8:0000:0000:0000:0000:0000:0001", HostIPv6, "2001:db8::1"},
	}

	for _, tc := range tests {
		got := NormalizeHost(tc.input, tc.hType)
		if got != tc.expected {
			t.Errorf("NormalizeHost(%q, %v) = %q, want %q", tc.input, tc.hType, got, tc.expected)
		}
	}
}
