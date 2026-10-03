package monitor

import (
	"context"
	"fmt"
	"net"
	"time"

	"host-monitor/internal/config"
)

// TCPChecker probes specific TCP ports securely using net.Dialer.
type TCPChecker struct {
	cfg     *config.Config
	timeout time.Duration
}

// NewTCPChecker creates a new TCPChecker.
func NewTCPChecker(cfg *config.Config) *TCPChecker {
	timeout := 3 * time.Second
	if cfg != nil && cfg.RequestTimeout > 0 {
		timeout = cfg.RequestTimeout
		// Keep individual port dial timeout reasonable
		if timeout > 3*time.Second {
			timeout = 3 * time.Second
		}
	}
	return &TCPChecker{
		cfg:     cfg,
		timeout: timeout,
	}
}

// CheckPort attempts a TCP connection to host:port with strict timeouts.
func (t *TCPChecker) CheckPort(ctx context.Context, host string, port int, hType HostType) TCPPortResult {
	start := time.Now()
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	network := "tcp"
	if hType == HostIPv4 {
		network = "tcp4"
	} else if hType == HostIPv6 {
		network = "tcp6"
	}

	dialer := net.Dialer{
		Timeout: t.timeout,
	}

	dialCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	conn, err := dialer.DialContext(dialCtx, network, addr)
	latency := time.Since(start)

	if err != nil {
		return TCPPortResult{
			Port:    port,
			Open:    false,
			Latency: latency,
			Error:   err.Error(),
		}
	}
	_ = conn.Close() // Immediately close to prevent socket leak

	return TCPPortResult{
		Port:    port,
		Open:    true,
		Latency: latency,
	}
}

// CheckPorts probes a list of whitelisted ports sequentially or concurrently within limits.
func (t *TCPChecker) CheckPorts(ctx context.Context, host string, ports []int, hType HostType) []TCPPortResult {
	if len(ports) == 0 {
		if t.cfg != nil && len(t.cfg.MonitorPorts) > 0 {
			ports = t.cfg.MonitorPorts
		} else {
			ports = []int{22, 80, 443}
		}
	}

	results := make([]TCPPortResult, 0, len(ports))
	for _, port := range ports {
		res := t.CheckPort(ctx, host, port, hType)
		results = append(results, res)
	}
	return results
}
