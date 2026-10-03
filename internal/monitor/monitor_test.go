package monitor

import (
	"strings"
	"testing"
	"time"
)

func TestFormatCheckResult_Domain(t *testing.T) {
	res := &CheckResult{
		Host:      "example.com",
		HostType:  HostDomain,
		CheckedAt: time.Now(),
		Status:    StatusOnline,
		DNS: DNSCheckResult{
			Resolved: true,
			IPs:      []string{"93.184.216.34"},
			Latency:  20 * time.Millisecond,
		},
		HTTP: HTTPProtocolResult{
			Attempted:  true,
			StatusCode: 200,
			StatusText: "OK",
			Latency:    120 * time.Millisecond,
		},
		HTTPS: HTTPProtocolResult{
			Attempted:  true,
			StatusCode: 200,
			StatusText: "OK",
			Latency:    135 * time.Millisecond,
		},
	}

	output := FormatCheckResult(res)

	expectedSnippets := []string{
		"Host: example.com",
		"Type: DOMAIN",
		"DNS       🟢 RESOLVED",
		"HTTP      🟢 200 OK",
		"HTTPS     🟢 200 OK",
		"HTTP      120 ms",
		"HTTPS     135 ms",
		"Status:\n🟢 ONLINE",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Errorf("FormatCheckResult output missing expected snippet: %q\nGot:\n%s", snippet, output)
		}
	}
}

func TestFormatCheckResult_IP(t *testing.T) {
	res := &CheckResult{
		Host:      "123.123.123.123",
		HostType:  HostIPv4,
		CheckedAt: time.Now(),
		Status:    StatusOnline,
		Ping: PingCheckResult{
			Reachable: true,
			Latency:   28 * time.Millisecond,
		},
		TCPPorts: []TCPPortResult{
			{Port: 22, Open: true, Latency: 25 * time.Millisecond},
			{Port: 80, Open: true, Latency: 28 * time.Millisecond},
			{Port: 443, Open: false, Latency: 30 * time.Millisecond},
		},
	}

	output := FormatCheckResult(res)

	expectedSnippets := []string{
		"Host: 123.123.123.123",
		"Type: IPV4",
		"PING\n🟢 REACHABLE\n28 ms",
		"22    🟢 OPEN",
		"80    🟢 OPEN",
		"443   🔴 CLOSED",
		"Status:\n🟢 ONLINE",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Errorf("FormatCheckResult output missing snippet: %q\nGot:\n%s", snippet, output)
		}
	}
}

func TestClassifyDomainStatus(t *testing.T) {
	// HTTPS 200 OK -> ONLINE
	st := classifyDomainStatus(
		HTTPProtocolResult{Attempted: true, StatusCode: 200},
		HTTPProtocolResult{Attempted: true, StatusCode: 200},
	)
	if st != StatusOnline {
		t.Errorf("expected StatusOnline, got %v", st)
	}

	// 500 Server Error
	st = classifyDomainStatus(
		HTTPProtocolResult{Attempted: true, StatusCode: 500},
		HTTPProtocolResult{Attempted: true, StatusCode: 500},
	)
	if st != StatusServerError {
		t.Errorf("expected StatusServerError, got %v", st)
	}

	// 404 Warning
	st = classifyDomainStatus(
		HTTPProtocolResult{Attempted: true, StatusCode: 404},
		HTTPProtocolResult{Attempted: true, StatusCode: 404},
	)
	if st != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", st)
	}

	// TLS Error
	st = classifyDomainStatus(
		HTTPProtocolResult{Attempted: true, StatusCode: 200},
		HTTPProtocolResult{Attempted: true, TLSError: true},
	)
	// Because HTTP is 200, but HTTPS had TLS Error, let's check: HTTP 200 gives online unless both fail or HTTPS error
	if st != StatusOnline {
		t.Errorf("expected StatusOnline when HTTP is working, got %v", st)
	}
}
