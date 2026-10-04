package monitor

import (
	"net"
	"regexp"
	"strconv"
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

// SplitHostPort separates base host and custom port if present.
// Supports "example.com:8080", "1.2.3.4:3306", "[2001:db8::1]:8443", or plain "example.com".
func SplitHostPort(target string) (baseHost string, port int, hasPort bool, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", 0, false, nil
	}

	// Reject if target contains URI schemes or query parameters
	if strings.Contains(target, "/") || strings.Contains(target, "?") || strings.Contains(target, "#") {
		return "", 0, false, net.InvalidAddrError("target contains invalid URL path or parameters")
	}

	// If contains colon, check if it's host:port or bare IPv6
	if strings.Contains(target, ":") {
		h, pStr, splitErr := net.SplitHostPort(target)
		if splitErr == nil {
			var p int
			for _, ch := range pStr {
				if ch < '0' || ch > '9' {
					return "", 0, false, net.InvalidAddrError("port must be numeric")
				}
				p = p*10 + int(ch-'0')
				if p > 65535 {
					return "", 0, false, net.InvalidAddrError("port exceeds 65535")
				}
			}
			if p < 1 || p > 65535 {
				return "", 0, false, net.InvalidAddrError("port must be between 1 and 65535")
			}
			return h, p, true, nil
		}

		// Bare IPv6 check (e.g. 2001:db8::1)
		if ip := net.ParseIP(target); ip != nil {
			return target, 0, false, nil
		}
		return "", 0, false, splitErr
	}

	return target, 0, false, nil
}

// DetectHostType inspects a target string and determines whether it is a DOMAIN, IPV4, IPV6, or UNKNOWN.
// Supports both standard targets and custom ports (e.g. example.com:8080, 1.2.3.4:3306).
func DetectHostType(target string) HostType {
	baseHost, _, _, err := SplitHostPort(target)
	if err != nil || baseHost == "" || len(baseHost) > 253 {
		return HostUnknown
	}

	// 1. Check IP using net.ParseIP
	if ip := net.ParseIP(baseHost); ip != nil {
		if ip.To4() != nil {
			return HostIPv4
		}
		return HostIPv6
	}

	// 2. Reject if base host contains invalid characters
	if strings.ContainsAny(baseHost, "/:@?#% ") {
		return HostUnknown
	}

	// 3. Check Domain Name syntax
	if domainRegex.MatchString(baseHost) {
		return HostDomain
	}

	return HostUnknown
}

// NormalizeHost normalizes host string (e.g. lowercases domains, canonicalizes IPs).
func NormalizeHost(target string, hType HostType) string {
	baseHost, port, hasPort, err := SplitHostPort(target)
	if err != nil {
		return target
	}

	var normHost string
	if hType == HostDomain {
		normHost = strings.ToLower(baseHost)
	} else if ip := net.ParseIP(baseHost); ip != nil {
		normHost = ip.String()
	} else {
		normHost = baseHost
	}

	if hasPort {
		return net.JoinHostPort(normHost, strconv.Itoa(port))
	}
	return normHost
}

