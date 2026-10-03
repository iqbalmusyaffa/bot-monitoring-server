package monitor

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"host-monitor/internal/config"
)

func TestPingChecker_ReachableListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	openPort, _ := strconv.Atoi(portStr)

	cfg := &config.Config{
		RequestTimeout: 2 * time.Second,
		MonitorPorts:   []int{openPort},
	}
	checker := NewPingChecker(cfg)

	res := checker.Ping(context.Background(), "127.0.0.1", HostIPv4)

	if !res.Reachable {
		t.Errorf("expected host to be Reachable, got unreachable: %s", res.Error)
	}
	if res.Latency <= 0 {
		t.Errorf("expected latency > 0, got %v", res.Latency)
	}
}

func TestPingChecker_Unreachable(t *testing.T) {
	cfg := &config.Config{
		RequestTimeout: 500 * time.Millisecond,
		MonitorPorts:   []int{80},
	}
	checker := NewPingChecker(cfg)

	// Non-routable IP in TEST-NET (198.51.100.1)
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	res := checker.Ping(ctx, "198.51.100.1", HostIPv4)
	if res.Reachable {
		t.Errorf("expected non-routable IP to be unreachable")
	}
}
