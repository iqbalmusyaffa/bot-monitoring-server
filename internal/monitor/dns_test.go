package monitor

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDNSChecker_ResolveSuccess(t *testing.T) {
	checker := NewDNSChecker(net.DefaultResolver, 5*time.Second)
	ctx := context.Background()

	// Resolve public test domain
	res, err := checker.Resolve(ctx, "one.one.one.one")
	if err != nil {
		t.Skipf("skipping test due to network availability in test environment: %v", err)
		return
	}

	if !res.Resolved {
		t.Errorf("expected DNS to resolve, got failed: %s", res.Error)
	}

	if len(res.IPs) == 0 {
		t.Errorf("expected at least one IP returned, got 0")
	}

	if res.Latency <= 0 {
		t.Errorf("expected latency > 0, got %v", res.Latency)
	}
}

func TestDNSChecker_ResolveNXDOMAIN(t *testing.T) {
	checker := NewDNSChecker(net.DefaultResolver, 5*time.Second)
	ctx := context.Background()

	// Resolve non-existent domain
	nonExistentDomain := "this-domain-definitely-does-not-exist-123456789.invalid"
	res, err := checker.Resolve(ctx, nonExistentDomain)

	if err == nil {
		t.Errorf("expected error for non-existent domain, got nil")
	}

	if res.Resolved {
		t.Errorf("expected Resolved to be false for non-existent domain")
	}

	if res.Error == "" {
		t.Errorf("expected descriptive error message for NXDOMAIN")
	}
}

func TestCleanDNSError(t *testing.T) {
	tests := []struct {
		err         error
		expectedSub string
	}{
		{
			err:         &net.DNSError{IsNotFound: true},
			expectedSub: "host not found (NXDOMAIN)",
		},
		{
			err:         &net.DNSError{IsTimeout: true},
			expectedSub: "timed out",
		},
		{
			err:         nil,
			expectedSub: "",
		},
	}

	for _, tc := range tests {
		got := cleanDNSError(tc.err)
		if !strings.Contains(got, tc.expectedSub) {
			t.Errorf("cleanDNSError(%v) = %q, want substring %q", tc.err, got, tc.expectedSub)
		}
	}
}
