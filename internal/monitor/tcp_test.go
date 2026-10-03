package monitor

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"host-monitor/internal/config"
)

func TestTCPChecker_OpenAndClosedPort(t *testing.T) {
	// Start local TCP listener on ephemeral port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start local tcp listener: %v", err)
	}
	defer listener.Close()

	// Accept connections in background to avoid blocking
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// Extract listening port
	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}
	openPort, _ := strconv.Atoi(portStr)

	cfg := &config.Config{
		RequestTimeout: 2 * time.Second,
	}
	checker := NewTCPChecker(cfg)

	ctx := context.Background()

	// Test Open Port
	openRes := checker.CheckPort(ctx, "127.0.0.1", openPort, HostIPv4)
	if !openRes.Open {
		t.Errorf("expected port %d to be OPEN, got CLOSED (error: %s)", openPort, openRes.Error)
	}
	if openRes.Latency <= 0 {
		t.Errorf("expected latency > 0, got %v", openRes.Latency)
	}

	// Test Closed Port (e.g. port 1 which is normally closed)
	closedPort := 1
	if closedPort == openPort {
		closedPort = 2
	}
	closedRes := checker.CheckPort(ctx, "127.0.0.1", closedPort, HostIPv4)
	if closedRes.Open {
		t.Errorf("expected port %d to be CLOSED, got OPEN", closedPort)
	}
}

func TestTCPChecker_CheckPorts(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	openPort, _ := strconv.Atoi(portStr)

	checker := NewTCPChecker(&config.Config{RequestTimeout: 2 * time.Second})

	ports := []int{openPort, 9999}
	results := checker.CheckPorts(context.Background(), "127.0.0.1", ports, HostIPv4)

	if len(results) != 2 {
		t.Fatalf("expected 2 port results, got %d", len(results))
	}

	if results[0].Port != openPort || !results[0].Open {
		t.Errorf("expected first port to be %d OPEN, got %v", openPort, results[0])
	}

	if results[1].Port != 9999 || results[1].Open {
		t.Errorf("expected second port to be 9999 CLOSED, got %v", results[1])
	}
}
