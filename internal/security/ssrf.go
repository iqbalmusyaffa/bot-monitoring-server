package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"host-monitor/internal/monitor"
)

var (
	// ErrPrivateTarget is returned when a target resolves to or is a private/internal IP.
	ErrPrivateTarget = errors.New("private/internal address is not allowed")
	// ErrNoIPResolved is returned when a domain fails to resolve any IP.
	ErrNoIPResolved = errors.New("could not resolve domain to any IP address")
)

// Private/reserved IPv4 and IPv6 CIDR blocks to block.
var blockedCIDRStrings = []string{
	// IPv4 Loopback & Special
	"0.0.0.0/8",
	"127.0.0.0/8",
	"169.254.0.0/16", // Link-local
	"255.255.255.255/32",

	// IPv4 RFC 1918 Private Addresses
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",

	// IPv4 Carrier-grade NAT & Other Reserved
	"100.64.0.0/10",
	"192.0.0.0/24",
	"198.18.0.0/15",
	"224.0.0.0/4", // Multicast
	"240.0.0.0/4", // Reserved

	// IPv6 Loopback & Unspecified
	"::/128",
	"::1/128",

	// IPv6 Link-Local & Unique Local (ULA)
	"fe80::/10",
	"fc00::/7",

	// IPv6 Multicast
	"ff00::/8",
}

var blockedIPNets []*net.IPNet

func init() {
	for _, cidr := range blockedCIDRStrings {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			blockedIPNets = append(blockedIPNets, ipNet)
		}
	}
}

// IsPrivateOrReservedIP checks if a given IP address belongs to loopback, private,
// link-local, multicast, or reserved ranges.
func IsPrivateOrReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Native net.IP checks
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	// CIDR blocks check (covers 0.0.0.0/8, 100.64.0.0/10, 240.0.0.0/4, etc.)
	for _, ipNet := range blockedIPNets {
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

// ValidateTargetSecurity ensures the target is public and safe against SSRF attacks.
// For domains, it performs DNS resolution and verifies that NONE of the resolved IPs are private/reserved.
func ValidateTargetSecurity(ctx context.Context, target string, hType monitor.HostType, resolver *net.Resolver) error {
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	switch hType {
	case monitor.HostIPv4, monitor.HostIPv6:
		ip := net.ParseIP(target)
		if ip == nil {
			return fmt.Errorf("invalid IP target: %s", target)
		}
		if IsPrivateOrReservedIP(ip) {
			return ErrPrivateTarget
		}
		return nil

	case monitor.HostDomain:
		resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		ips, err := resolver.LookupIPAddr(resolveCtx, target)
		if err != nil {
			return fmt.Errorf("DNS resolution failed for %s: %w", target, err)
		}
		if len(ips) == 0 {
			return ErrNoIPResolved
		}

		// Inspect EVERY resolved IP address for SSRF protection
		for _, ipAddr := range ips {
			if IsPrivateOrReservedIP(ipAddr.IP) {
				return ErrPrivateTarget
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown target type: %s", hType)
	}
}
