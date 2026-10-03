package monitor

import (
	"context"
	"fmt"
	"net"
	"time"

	"host-monitor/internal/config"
)

// PingChecker checks general network reachability and round-trip latency without requiring root privileges.
type PingChecker struct {
	cfg     *config.Config
	timeout time.Duration
}

// NewPingChecker creates a new PingChecker.
func NewPingChecker(cfg *config.Config) *PingChecker {
	timeout := 3 * time.Second
	if cfg != nil && cfg.RequestTimeout > 0 {
		timeout = cfg.RequestTimeout
		if timeout > 3*time.Second {
			timeout = 3 * time.Second
		}
	}
	return &PingChecker{
		cfg:     cfg,
		timeout: timeout,
	}
}

// Ping probes reachability to host using pure standard library non-root TCP/UDP connectivity probes.
func (p *PingChecker) Ping(ctx context.Context, host string, hType HostType) PingCheckResult {
	// Probe ports list in order of popularity
	probePorts := []int{80, 443, 22, 53, 8080}
	if p.cfg != nil && len(p.cfg.MonitorPorts) > 0 {
		probePorts = p.cfg.MonitorPorts
	}

	network := "tcp"
	if hType == HostIPv4 {
		network = "tcp4"
	} else if hType == HostIPv6 {
		network = "tcp6"
	}

	var bestLatency time.Duration
	var bestMethod string
	var reached bool

	dialer := net.Dialer{Timeout: p.timeout}

	for _, port := range probePorts {
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

		dialCtx, cancel := context.WithTimeout(ctx, p.timeout)
		start := time.Now()
		conn, err := dialer.DialContext(dialCtx, network, addr)
		latency := time.Since(start)
		cancel()

		if err == nil {
			_ = conn.Close()
			return PingCheckResult{
				Reachable: true,
				Latency:   latency,
				Method:    fmt.Sprintf("TCP:%d", port),
			}
		}

		// On connection refused (RST packet returned), the server is still definitely reachable/online!
		if isHostReachableFromError(err) {
			if !reached || latency < bestLatency {
				reached = true
				bestLatency = latency
				bestMethod = fmt.Sprintf("TCP-RST:%d", port)
			}
		}
	}

	if reached {
		return PingCheckResult{
			Reachable: true,
			Latency:   bestLatency,
			Method:    bestMethod,
		}
	}

	return PingCheckResult{
		Reachable: false,
		Error:     "host did not respond to connectivity probes",
	}
}

// isHostReachableFromError checks if an error indicates the host answered with a TCP RST (connection refused),
// which proves network reachability and that the machine is online.
func isHostReachableFromError(err error) bool {
	if err == nil {
		return true
	}
	errStr := err.Error()
	return (netErrorContains(errStr, "refused") ||
		netErrorContains(errStr, "reset by peer")) &&
		!netErrorContains(errStr, "timeout") &&
		!netErrorContains(errStr, "no route to host") &&
		!netErrorContains(errStr, "unreachable")
}

func netErrorContains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || findSubstr(str, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
