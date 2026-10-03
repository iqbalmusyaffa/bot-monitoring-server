package monitor

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"host-monitor/internal/config"
)

// OverallStatus represents the high-level status of a host.
type OverallStatus string

const (
	StatusOnline      OverallStatus = "ONLINE"
	StatusWarning     OverallStatus = "WARNING"
	StatusServerError OverallStatus = "SERVER ERROR"
	StatusDown        OverallStatus = "DOWN"
	StatusUnreachable OverallStatus = "UNREACHABLE"
	StatusTLSError    OverallStatus = "TLS ERROR"
)

// DNSCheckResult holds DNS resolution details.
type DNSCheckResult struct {
	Resolved bool
	IPs      []string
	Latency  time.Duration
	Error    string
}

// HTTPProtocolResult holds the result of HTTP or HTTPS check.
type HTTPProtocolResult struct {
	Attempted  bool
	StatusCode int
	StatusText string
	Latency    time.Duration
	TLSError   bool
	Error      string
}

// TCPPortResult holds the connectivity result for a specific port.
type TCPPortResult struct {
	Port    int
	Open    bool
	Latency time.Duration
	Error   string
}

// PingCheckResult holds ping/TCP fallback connectivity check details.
type PingCheckResult struct {
	Reachable bool
	Latency   time.Duration
	Method    string
	Error     string
}

// CheckResult represents the aggregate diagnostics for a single host check.
type CheckResult struct {
	Host        string
	HostType    HostType
	CheckedAt   time.Time
	Status      OverallStatus
	DNS         DNSCheckResult
	HTTP        HTTPProtocolResult
	HTTPS       HTTPProtocolResult
	TCPPorts    []TCPPortResult
	Ping        PingCheckResult
	TotalTimeMs int64
}

// Checker coordinates host checks safely.
type Checker struct {
	cfg         *config.Config
	dnsChecker  *DNSChecker
	httpChecker *HTTPChecker
	tcpChecker  *TCPChecker
	pingChecker *PingChecker
}

// NewChecker creates a new Checker.
func NewChecker(cfg *config.Config) *Checker {
	var timeout time.Duration = 5 * time.Second
	if cfg != nil && cfg.RequestTimeout > 0 {
		timeout = cfg.RequestTimeout
	}

	return &Checker{
		cfg:         cfg,
		dnsChecker:  NewDNSChecker(net.DefaultResolver, timeout),
		httpChecker: NewHTTPChecker(cfg),
		tcpChecker:  NewTCPChecker(cfg),
		pingChecker: NewPingChecker(cfg),
	}
}

// CheckHost executes full diagnostic checks based on host type (DOMAIN vs IP).
func (c *Checker) CheckHost(ctx context.Context, host string, hType HostType) *CheckResult {
	start := time.Now()
	res := &CheckResult{
		Host:      host,
		HostType:  hType,
		CheckedAt: start,
	}

	timeout := 10 * time.Second
	if c.cfg != nil && c.cfg.RequestTimeout > 0 {
		timeout = c.cfg.RequestTimeout
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if hType == HostDomain {
		c.checkDomain(checkCtx, host, res)
	} else {
		c.checkIP(checkCtx, host, hType, res)
	}

	res.TotalTimeMs = time.Since(start).Milliseconds()
	return res
}

// checkDomain runs DNS, HTTP, and HTTPS checks.
func (c *Checker) checkDomain(ctx context.Context, host string, res *CheckResult) {
	// 1. DNS Check via modular DNSChecker
	dnsRes, err := c.dnsChecker.Resolve(ctx, host)
	if dnsRes != nil {
		res.DNS = *dnsRes
	}

	if err != nil || !res.DNS.Resolved {
		res.Status = StatusDown
		return
	}

	// 2. Perform HTTP Check (Port 80)
	res.HTTP = c.httpChecker.Check(ctx, "http://"+host)

	// 3. Perform HTTPS Check (Port 443)
	res.HTTPS = c.httpChecker.Check(ctx, "https://"+host)

	// 4. Classify Domain Status
	res.Status = classifyDomainStatus(res.HTTP, res.HTTPS)
}

// checkIP runs Ping/connectivity and TCP port checks.
func (c *Checker) checkIP(ctx context.Context, host string, hType HostType, res *CheckResult) {
	ports := []int{22, 80, 443}
	if c.cfg != nil && len(c.cfg.MonitorPorts) > 0 {
		ports = c.cfg.MonitorPorts
	}

	// 1. Check TCP ports via modular TCPChecker
	res.TCPPorts = c.tcpChecker.CheckPorts(ctx, host, ports, hType)

	var anyPortOpen bool
	for _, portRes := range res.TCPPorts {
		if portRes.Open {
			anyPortOpen = true
		}
	}

	// 2. Ping / Connectivity check via modular PingChecker
	res.Ping = c.pingChecker.Ping(ctx, host, hType)

	// 3. Classify IP Status (Section 10: ping reachable OR any port open = online)
	if res.Ping.Reachable || anyPortOpen {
		res.Status = StatusOnline
	} else {
		res.Status = StatusUnreachable
	}
}

// FormatCheckResult generates the formatted Telegram message according to specs.
func FormatCheckResult(res *CheckResult) string {
	var sb strings.Builder

	sb.WriteString("🖥️ HOST STATUS\n\n")
	sb.WriteString(fmt.Sprintf("Host: %s\n", res.Host))
	sb.WriteString(fmt.Sprintf("Type: %s\n\n", res.HostType))

	if res.HostType == HostDomain {
		// DNS section
		if res.DNS.Resolved {
			sb.WriteString("DNS       🟢 RESOLVED\n\n")
		} else {
			sb.WriteString("DNS       🔴 FAILED\n\n")
		}

		// HTTP & HTTPS status
		if res.HTTP.Attempted {
			sb.WriteString(fmt.Sprintf("HTTP      %s\n", formatHTTPStatusEmoji(res.HTTP)))
		}
		if res.HTTPS.Attempted {
			sb.WriteString(fmt.Sprintf("HTTPS     %s\n", formatHTTPStatusEmoji(res.HTTPS)))
		}
		sb.WriteString("\n")

		// Latency section
		if res.HTTP.Attempted && res.HTTP.Latency > 0 {
			sb.WriteString(fmt.Sprintf("HTTP      %d ms\n", res.HTTP.Latency.Milliseconds()))
		}
		if res.HTTPS.Attempted && res.HTTPS.Latency > 0 {
			sb.WriteString(fmt.Sprintf("HTTPS     %d ms\n", res.HTTPS.Latency.Milliseconds()))
		}
		sb.WriteString("\n")

	} else {
		// IP Section (Ping & TCP)
		sb.WriteString("PING\n")
		if res.Ping.Reachable {
			sb.WriteString("🟢 REACHABLE\n")
			sb.WriteString(fmt.Sprintf("%d ms\n\n", res.Ping.Latency.Milliseconds()))
		} else {
			sb.WriteString("🔴 UNREACHABLE\n\n")
		}

		sb.WriteString("TCP\n\n")
		for _, portRes := range res.TCPPorts {
			if portRes.Open {
				sb.WriteString(fmt.Sprintf("%-5d 🟢 OPEN\n", portRes.Port))
			} else {
				sb.WriteString(fmt.Sprintf("%-5d 🔴 CLOSED\n", portRes.Port))
			}
		}
		sb.WriteString("\n")
	}

	// Overall Status
	sb.WriteString("Status:\n")
	sb.WriteString(formatOverallStatus(res.Status))

	return sb.String()
}

func formatHTTPStatusEmoji(res HTTPProtocolResult) string {
	if res.TLSError {
		return "🔴 TLS ERROR"
	}
	if res.StatusCode >= 200 && res.StatusCode < 400 {
		return fmt.Sprintf("🟢 %d %s", res.StatusCode, res.StatusText)
	}
	if res.StatusCode >= 400 && res.StatusCode < 500 {
		return fmt.Sprintf("🟡 %d %s", res.StatusCode, res.StatusText)
	}
	if res.StatusCode >= 500 {
		return fmt.Sprintf("🔴 %d %s", res.StatusCode, res.StatusText)
	}
	return "🔴 DOWN"
}

func formatOverallStatus(st OverallStatus) string {
	switch st {
	case StatusOnline:
		return "🟢 ONLINE"
	case StatusWarning:
		return "🟡 WARNING"
	case StatusServerError:
		return "🔴 SERVER ERROR"
	case StatusTLSError:
		return "🔴 TLS ERROR"
	case StatusUnreachable:
		return "🔴 UNREACHABLE"
	default:
		return "🔴 DOWN"
	}
}

func classifyDomainStatus(httpRes, httpsRes HTTPProtocolResult) OverallStatus {
	if httpsRes.Attempted && httpsRes.StatusCode >= 200 && httpsRes.StatusCode < 400 {
		return StatusOnline
	}
	if httpRes.Attempted && httpRes.StatusCode >= 200 && httpRes.StatusCode < 400 {
		return StatusOnline
	}
	if httpsRes.TLSError {
		return StatusTLSError
	}
	if (httpsRes.Attempted && httpsRes.StatusCode >= 500) || (httpRes.Attempted && httpRes.StatusCode >= 500) {
		return StatusServerError
	}
	if (httpsRes.Attempted && httpsRes.StatusCode >= 400) || (httpRes.Attempted && httpRes.StatusCode >= 400) {
		return StatusWarning
	}
	return StatusDown
}
