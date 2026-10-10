package monitor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

var (
	// ErrInvalidDomain is returned when input is not a domain.
	ErrInvalidDomain = errors.New("target harus berupa nama domain (contoh: example.id atau google.com)")
	// ErrWhoisServerNotFound is returned when no whois server could be determined.
	ErrWhoisServerNotFound = errors.New("server WHOIS untuk domain ini tidak ditemukan")
)

// WhoisRecord holds parsed WHOIS and domain registration data.
type WhoisRecord struct {
	Domain        string
	Registrar     string
	CreatedDate   *time.Time
	ExpiryDate    *time.Time
	UpdatedDate   *time.Time
	DomainStatus  []string
	NameServers   []string
	DaysRemaining int
	IsExpired     bool
	IsAvailable   bool
	WhoisServer   string
}

// WhoisClient queries and parses WHOIS data directly over TCP port 43.
type WhoisClient struct {
	timeout time.Duration
	dialer  func(ctx context.Context, network, address string) (net.Conn, error)
}

// NewWhoisClient initializes a new WhoisClient.
func NewWhoisClient(timeout time.Duration) *WhoisClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	d := &net.Dialer{Timeout: timeout}
	return &WhoisClient{
		timeout: timeout,
		dialer:  d.DialContext,
	}
}

// SetDialer allows mocking network connections in tests.
func (c *WhoisClient) SetDialer(dialer func(ctx context.Context, network, address string) (net.Conn, error)) {
	c.dialer = dialer
}

var knownWhoisServers = map[string]string{
	"id":   "whois.id",
	"com":  "whois.verisign-grs.com",
	"net":  "whois.verisign-grs.com",
	"org":  "whois.publicinterestregistry.org",
	"io":   "whois.nic.io",
	"co":   "whois.nic.co",
	"me":   "whois.nic.me",
	"info": "whois.afilias.net",
	"biz":  "whois.biz",
	"xyz":  "whois.nic.xyz",
	"dev":  "whois.nic.google",
	"app":  "whois.nic.google",
	"ai":   "whois.nic.ai",
	"sg":   "whois.sgnic.sg",
	"my":   "whois.mynic.my",
	"th":   "whois.thnic.co.th",
	"in":   "whois.registry.in",
	"us":   "whois.nic.us",
	"uk":   "whois.nic.uk",
	"ca":   "whois.cira.ca",
	"de":   "whois.denic.de",
	"jp":   "whois.jprs.jp",
	"nl":   "whois.domain-registry.nl",
	"eu":   "whois.eu",
}

// CleanDomain extracts pure domain name without protocol, path, or port.
func CleanDomain(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")

	if idx := strings.Index(raw, "/"); idx != -1 {
		raw = raw[:idx]
	}
	if idx := strings.Index(raw, "?"); idx != -1 {
		raw = raw[:idx]
	}
	if idx := strings.Index(raw, ":"); idx != -1 {
		raw = raw[:idx]
	}

	hType := DetectHostType(raw)
	if hType != HostDomain {
		return "", ErrInvalidDomain
	}

	return raw, nil
}

// ResolveWhoisServer determines the authoritative WHOIS server for a given domain.
func (c *WhoisClient) ResolveWhoisServer(ctx context.Context, domain string) (string, error) {
	// 1. All Indonesian ccTLDs (.id, .co.id, .web.id, .my.id, etc.) use whois.id
	if strings.HasSuffix(domain, ".id") {
		return "whois.id", nil
	}

	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return "", ErrInvalidDomain
	}
	tld := parts[len(parts)-1]

	// 2. Fast lookup in known map
	if srv, ok := knownWhoisServers[tld]; ok {
		return srv, nil
	}

	// 3. Fallback: Query whois.iana.org for the TLD referral
	ianaConn, err := c.dialer(ctx, "tcp", "whois.iana.org:43")
	if err != nil {
		return "", fmt.Errorf("gagal menghubungi IANA WHOIS: %w", err)
	}
	defer ianaConn.Close()

	if _, err := fmt.Fprintf(ianaConn, "%s\r\n", tld); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(ianaConn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(strings.ToLower(line), "whois:") {
			server := strings.TrimSpace(strings.TrimPrefix(line, "whois:"))
			if server != "" {
				return server, nil
			}
		}
	}

	return "", ErrWhoisServerNotFound
}

// Query performs a raw WHOIS query to the specified server.
func (c *WhoisClient) Query(ctx context.Context, server, domain string) (string, error) {
	if !strings.Contains(server, ":") {
		server = net.JoinHostPort(server, "43")
	}

	conn, err := c.dialer(ctx, "tcp", server)
	if err != nil {
		return "", fmt.Errorf("gagal koneksi ke %s: %w", server, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	if _, err := fmt.Fprintf(conn, "%s\r\n", domain); err != nil {
		return "", fmt.Errorf("gagal mengirim kueri WHOIS: %w", err)
	}

	var sb strings.Builder
	buf := make([]byte, 4096)
	totalBytes := 0
	const maxBytes = 65536 // 64 KB limit

	for {
		n, err := conn.Read(buf)
		if n > 0 {
			totalBytes += n
			if totalBytes > maxBytes {
				break
			}
			sb.Write(buf[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return sb.String(), nil
		}
	}

	return sb.String(), nil
}

// Lookup queries and parses domain WHOIS registration data.
func (c *WhoisClient) Lookup(ctx context.Context, rawDomain string) (*WhoisRecord, error) {
	domain, err := CleanDomain(rawDomain)
	if err != nil {
		return nil, err
	}

	server, err := c.ResolveWhoisServer(ctx, domain)
	if err != nil {
		return nil, err
	}

	rawText, err := c.Query(ctx, server, domain)
	if err != nil {
		return nil, err
	}

	record := ParseWhoisResponse(domain, server, rawText)
	return record, nil
}

// ParseWhoisResponse parses raw WHOIS text into a structured WhoisRecord.
func ParseWhoisResponse(domain, server, raw string) *WhoisRecord {
	record := &WhoisRecord{
		Domain:      domain,
		WhoisServer: server,
	}

	lowerRaw := strings.ToLower(raw)

	// Check if domain is not registered / available
	if strings.Contains(lowerRaw, "domain not found") ||
		strings.Contains(lowerRaw, "no match for domain") ||
		strings.Contains(lowerRaw, "not found") ||
		strings.Contains(lowerRaw, "status: available") ||
		strings.Contains(lowerRaw, "no data found") {
		record.IsAvailable = true
		return record
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		if val == "" {
			continue
		}

		switch key {
		case "registrar", "sponsoring registrar":
			if record.Registrar == "" {
				record.Registrar = val
			}
		case "creation date", "created", "registration date":
			if record.CreatedDate == nil {
				record.CreatedDate = parseWhoisDate(val)
			}
		case "registry expiry date", "registrar registration expiration date", "expiration date", "expiry date", "paid-till", "expire":
			if record.ExpiryDate == nil {
				record.ExpiryDate = parseWhoisDate(val)
			}
		case "updated date", "last updated date", "changed":
			if record.UpdatedDate == nil {
				record.UpdatedDate = parseWhoisDate(val)
			}
		case "domain status", "status":
			statusToken := strings.Fields(val)[0] // e.g. "ok" or "clientTransferProhibited"
			if !containsString(record.DomainStatus, statusToken) {
				record.DomainStatus = append(record.DomainStatus, statusToken)
			}
		case "name server", "nserver":
			nsToken := strings.ToLower(strings.Fields(val)[0])
			if !containsString(record.NameServers, nsToken) {
				record.NameServers = append(record.NameServers, nsToken)
			}
		}
	}

	if record.ExpiryDate != nil {
		diff := time.Until(*record.ExpiryDate)
		record.DaysRemaining = int(diff.Hours() / 24)
		if diff < 0 {
			record.IsExpired = true
		}
	}

	return record
}

func parseWhoisDate(str string) *time.Time {
	str = strings.TrimSpace(str)
	if str == "" {
		return nil
	}

	// Supported date layouts
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-Jan-2006",
		"02-Jan-2006 15:04:05",
		"02.01.2006",
		"2006.01.02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, str); err == nil {
			return &t
		}
	}

	return nil
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}
