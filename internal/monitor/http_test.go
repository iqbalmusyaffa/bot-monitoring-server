package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"host-monitor/internal/config"
)

func TestHTTPChecker_Success200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	cfg := &config.Config{
		RequestTimeout:  2 * time.Second,
		MaxResponseBody: 1048576,
		MaxRedirects:    5,
	}
	checker := NewHTTPChecker(cfg)

	res := checker.Check(context.Background(), server.URL)
	if res.StatusCode != 200 {
		t.Errorf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.StatusText != "OK" {
		t.Errorf("expected status text 'OK', got %s", res.StatusText)
	}
	if res.TLSError {
		t.Errorf("expected no TLS error on plain HTTP")
	}
	if res.Latency <= 0 {
		t.Errorf("expected latency > 0, got %v", res.Latency)
	}
}

func TestHTTPChecker_FallbackOn405(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("GET OK"))
	}))
	defer server.Close()

	cfg := &config.Config{
		RequestTimeout: 2 * time.Second,
	}
	checker := NewHTTPChecker(cfg)

	res := checker.Check(context.Background(), server.URL)
	if res.StatusCode != 200 {
		t.Errorf("expected fallback to GET 200, got %d", res.StatusCode)
	}
}

func TestHTTPChecker_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"Not Found", http.StatusNotFound},
		{"Forbidden", http.StatusForbidden},
		{"Server Error", http.StatusInternalServerError},
		{"Bad Gateway", http.StatusBadGateway},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			checker := NewHTTPChecker(&config.Config{RequestTimeout: 2 * time.Second})
			res := checker.Check(context.Background(), server.URL)

			if res.StatusCode != tc.statusCode {
				t.Errorf("expected %d, got %d", tc.statusCode, res.StatusCode)
			}
		})
	}
}

func TestHTTPChecker_RedirectLimit(t *testing.T) {
	// Create circular redirect server
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL, http.StatusFound)
	}))
	defer server.Close()

	cfg := &config.Config{
		RequestTimeout: 2 * time.Second,
		MaxRedirects:   3,
	}
	checker := NewHTTPChecker(cfg)

	res := checker.Check(context.Background(), server.URL)
	if res.Error == "" || !strings.Contains(res.Error, "redirects") {
		t.Errorf("expected error stopping after max redirects, got: %v", res.Error)
	}
}

func TestHTTPChecker_TLSError(t *testing.T) {
	// Create self-signed TLS server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Using standard client with strict cert validation (will reject untrusted self-signed cert)
	cfg := &config.Config{
		RequestTimeout: 2 * time.Second,
	}
	checker := NewHTTPChecker(cfg)

	res := checker.Check(context.Background(), server.URL)
	if !res.TLSError {
		t.Errorf("expected TLSError to be true for untrusted self-signed certificate")
	}
}
