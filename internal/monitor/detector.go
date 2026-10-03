package monitor

import (
	"net"
	"regexp"
	"strings"
)

// HostType represents the categorized type of host.
type HostType string

const (
	HostDomain  HostType = "DOMAIN"
	HostIPv4    HostType = "IPV4"
	HostIPv6    HostType = "IPV6"
	HostUnknown HostType = "UNKNOWN"
)

// domainRegex validates standard domain names (RFC 1035 / RFC 1123).
// Disallows protocols, paths, query strings, and invalid characters.
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// DetectHostType inspects a target string and determines whether it is a DOMAIN, IPV4, IPV6, or UNKNOWN.
func DetectHostType(target string) HostType {
	target = strings.TrimSpace(target)
	if target == "" || len(target) > 253 {
		return HostUnknown
	}

	// 1. Check IP using net.ParseIP
	if ip := net.ParseIP(target); ip != nil {
		if ip.To4() != nil {
			return HostIPv4
		}
		return HostIPv6
	}

	// 2. Reject if input contains URI schemes, slashes, colons, or query parameters
	if strings.ContainsAny(target, "/:@?#% ") {
		return HostUnknown
	}

	// 3. Check Domain Name syntax
	if domainRegex.MatchString(target) {
		return HostDomain
	}

	return HostUnknown
}

// NormalizeHost normalizes host string (e.g. lowercases domains, trims whitespace).
func NormalizeHost(target string, hType HostType) string {
	target = strings.TrimSpace(target)
	if hType == HostDomain {
		return strings.ToLower(target)
	}
	// For IP, parse and format canonically
	if ip := net.ParseIP(target); ip != nil {
		return ip.String()
	}
	return target
}
