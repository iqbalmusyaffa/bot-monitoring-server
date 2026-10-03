package monitor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// DNSChecker provides DNS resolution methods with timeouts and latency tracking.
type DNSChecker struct {
	resolver *net.Resolver
	timeout  time.Duration
}

// NewDNSChecker creates a new DNSChecker instance.
func NewDNSChecker(resolver *net.Resolver, timeout time.Duration) *DNSChecker {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &DNSChecker{
		resolver: resolver,
		timeout:  timeout,
	}
}

// Resolve performs DNS resolution for both A (IPv4) and AAAA (IPv6) records.
func (d *DNSChecker) Resolve(ctx context.Context, domain string) (*DNSCheckResult, error) {
	start := time.Now()

	resolveCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	ipAddrs, err := d.resolver.LookupIPAddr(resolveCtx, domain)
	latency := time.Since(start)

	result := &DNSCheckResult{
		Latency: latency,
	}

	if err != nil {
		result.Resolved = false
		result.Error = cleanDNSError(err)
		return result, err
	}

	if len(ipAddrs) == 0 {
		result.Resolved = false
		result.Error = "no IP addresses found for host"
		return result, errors.New(result.Error)
	}

	result.Resolved = true
	for _, addr := range ipAddrs {
		result.IPs = append(result.IPs, addr.IP.String())
	}

	return result, nil
}

// cleanDNSError simplifies internal DNS error messages for presentation and logging.
func cleanDNSError(err error) string {
	if err == nil {
		return ""
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "host not found (NXDOMAIN)"
		}
		if dnsErr.IsTimeout {
			return "DNS lookup timed out"
		}
		return fmt.Sprintf("DNS error: %s", dnsErr.Err)
	}
	return err.Error()
}
