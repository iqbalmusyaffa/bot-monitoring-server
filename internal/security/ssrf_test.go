package security

import (
	"context"
	"net"
	"testing"

	"host-monitor/internal/monitor"
)

func TestIsPrivateOrReservedIP(t *testing.T) {
	tests := []struct {
		ipStr     string
		isBlocked bool
	}{
		// IPv4 Loopback
		{"127.0.0.1", true},
		{"127.0.1.1", true},
		{"127.255.255.255", true},

		// IPv4 Private
		{"10.0.0.1", true},
		{"10.254.1.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.1.100", true},

		// IPv4 Link-local, Broadcast, Zero
		{"169.254.169.254", true}, // AWS/Cloud metadata
		{"0.0.0.0", true},
		{"255.255.255.255", true},
		{"224.0.0.1", true},   // Multicast
		{"240.0.0.1", true},   // Reserved

		// IPv4 Public (Safe)
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"142.250.190.46", false}, // Google
		{"93.184.216.34", false},  // Example.com

		// IPv6 Loopback & Unspecified
		{"::1", true},
		{"::", true},

		// IPv6 Link-local & ULA
		{"fe80::1", true},
		{"fe80::200:5aee:feaa:20a2", true},
		{"fc00::1", true},
		{"fd12:3456:789a:1::1", true},
		{"ff02::1", true}, // Multicast

		// IPv6 Public (Safe)
		{"2606:4700:4700::1111", false}, // Cloudflare IPv6
		{"2001:4860:4860::8888", false}, // Google DNS IPv6
	}

	for _, tc := range tests {
		ip := net.ParseIP(tc.ipStr)
		if ip == nil {
			t.Fatalf("failed to parse IP: %s", tc.ipStr)
		}
		got := IsPrivateOrReservedIP(ip)
		if got != tc.isBlocked {
			t.Errorf("IsPrivateOrReservedIP(%s) = %v, want %v", tc.ipStr, got, tc.isBlocked)
		}
	}
}

func TestValidateTargetSecurity_IP(t *testing.T) {
	ctx := context.Background()

	// Blocked IPs
	blockedIPs := []struct {
		ip    string
		hType monitor.HostType
	}{
		{"127.0.0.1", monitor.HostIPv4},
		{"10.0.0.1", monitor.HostIPv4},
		{"192.168.1.1", monitor.HostIPv4},
		{"169.254.169.254", monitor.HostIPv4},
		{"::1", monitor.HostIPv6},
		{"fe80::1", monitor.HostIPv6},
		{"fc00::1", monitor.HostIPv6},
	}

	for _, b := range blockedIPs {
		err := ValidateTargetSecurity(ctx, b.ip, b.hType, nil)
		if err != ErrPrivateTarget {
			t.Errorf("ValidateTargetSecurity(%s) expected ErrPrivateTarget, got: %v", b.ip, err)
		}
	}

	// Allowed Public IPs
	allowedIPs := []struct {
		ip    string
		hType monitor.HostType
	}{
		{"8.8.8.8", monitor.HostIPv4},
		{"1.1.1.1", monitor.HostIPv4},
		{"2606:4700:4700::1111", monitor.HostIPv6},
	}

	for _, a := range allowedIPs {
		err := ValidateTargetSecurity(ctx, a.ip, a.hType, nil)
		if err != nil {
			t.Errorf("ValidateTargetSecurity(%s) expected nil, got: %v", a.ip, err)
		}
	}
}
