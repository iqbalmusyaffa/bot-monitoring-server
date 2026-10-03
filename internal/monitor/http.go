package monitor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"host-monitor/internal/config"
)

// HTTPChecker executes HTTP and HTTPS diagnostic probes with security protections.
type HTTPChecker struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewHTTPChecker creates a new HTTPChecker configured with security limits.
func NewHTTPChecker(cfg *config.Config) *HTTPChecker {
	timeout := 10 * time.Second
	if cfg != nil && cfg.RequestTimeout > 0 {
		timeout = cfg.RequestTimeout
	}

	maxRedirects := 5
	if cfg != nil && cfg.MaxRedirects > 0 {
		maxRedirects = cfg.MaxRedirects
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, // Enforce strict TLS validation
		},
		DisableKeepAlives:     true, // Prevent socket exhaustion on small VPS
		ResponseHeaderTimeout: timeout,
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}

	return &HTTPChecker{
		cfg:        cfg,
		httpClient: client,
	}
}

// Check probes a specific URL (http:// or https://) with HEAD, falling back to GET on 405.
func (h *HTTPChecker) Check(ctx context.Context, targetURL string) HTTPProtocolResult {
	result := HTTPProtocolResult{
		Attempted: true,
	}

	maxBody := int64(1048576) // 1MB default
	if h.cfg != nil && h.cfg.MaxResponseBody > 0 {
		maxBody = h.cfg.MaxResponseBody
	}

	// 1. Try HEAD request first to save bandwidth
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", "HostMonitorBot/1.0 (+https://github.com/personal/host-monitor)")

	resp, err := h.httpClient.Do(req)
	latency := time.Since(start)

	// Fallback to GET if server rejects HEAD with 405 Method Not Allowed
	if err == nil && resp.StatusCode == http.StatusMethodNotAllowed {
		_ = resp.Body.Close()
		start = time.Now()
		getReq, getErr := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if getErr == nil {
			getReq.Header.Set("User-Agent", "HostMonitorBot/1.0")
			resp, err = h.httpClient.Do(getReq)
			latency = time.Since(start)
		}
	}

	if err != nil {
		result.Latency = latency
		result.Error = err.Error()
		result.TLSError = isTLSError(err)
		return result
	}
	defer resp.Body.Close()

	// Drain up to maxBody bytes safely to avoid memory exhaustion
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))

	result.StatusCode = resp.StatusCode
	result.StatusText = http.StatusText(resp.StatusCode)
	result.Latency = latency

	return result
}

// isTLSError identifies TLS handshake failures, certificate expiration, or untrusted CA errors.
func isTLSError(err error) bool {
	if err == nil {
		return false
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	var certErr *tls.CertificateVerificationError
	var unknownAuthErr x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var certInvalidErr x509.CertificateInvalidError

	if errors.As(err, &certErr) ||
		errors.As(err, &unknownAuthErr) ||
		errors.As(err, &hostErr) ||
		errors.As(err, &certInvalidErr) {
		return true
	}

	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "tls") ||
		strings.Contains(errMsg, "certificate") ||
		strings.Contains(errMsg, "handshake failure") ||
		strings.Contains(errMsg, "x509")
}
