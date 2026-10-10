package monitor

import (
	"context"
	"testing"
	"time"
)

func TestCleanDomain(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		expectError bool
	}{
		{"example.com", "example.com", false},
		{"https://kompas.id", "kompas.id", false},
		{"http://sub.domain.co.id:8080/test?foo=bar", "sub.domain.co.id", false},
		{"detik.com/", "detik.com", false},
		{"123.123.123.123", "", true},
		{"", "", true},
		{"invalid_domain", "", true},
	}

	for _, tc := range tests {
		got, err := CleanDomain(tc.input)
		if tc.expectError {
			if err == nil {
				t.Errorf("expected error for CleanDomain(%q), got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for CleanDomain(%q): %v", tc.input, err)
			}
			if got != tc.expected {
				t.Errorf("CleanDomain(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		}
	}
}

func TestResolveWhoisServer(t *testing.T) {
	client := NewWhoisClient(5 * time.Second)
	ctx := context.Background()

	tests := []struct {
		domain   string
		expected string
	}{
		{"kompas.id", "whois.id"},
		{"bca.co.id", "whois.id"},
		{"sekolah.sch.id", "whois.id"},
		{"google.com", "whois.verisign-grs.com"},
		{"github.net", "whois.verisign-grs.com"},
		{"wikipedia.org", "whois.publicinterestregistry.org"},
		{"startup.io", "whois.nic.io"},
		{"test.xyz", "whois.nic.xyz"},
	}

	for _, tc := range tests {
		server, err := client.ResolveWhoisServer(ctx, tc.domain)
		if err != nil {
			t.Errorf("ResolveWhoisServer(%q) error: %v", tc.domain, err)
		}
		if server != tc.expected {
			t.Errorf("ResolveWhoisServer(%q) = %q, want %q", tc.domain, server, tc.expected)
		}
	}
}

func TestParseWhoisResponse_IndonesianDomain(t *testing.T) {
	raw := `Domain Name: KOMPAS.ID
Registry Domain ID: 292576_DOMAIN_ID-ID
Registrar WHOIS Server: 
Registrar URL: https://www.indoreg.co.id
Updated Date: 2026-07-28T00:30:42Z
Creation Date: 2014-06-13T09:17:44Z
Registry Expiry Date: 2027-06-13T23:59:59Z
Registrar: PT Jetcoms Netindo
Registrar IANA ID: 1
Domain Status: ok
Name Server: ANUJ.NS.CLOUDFLARE.COM
Name Server: LAURA.NS.CLOUDFLARE.COM
DNSSEC: signed
`
	rec := ParseWhoisResponse("kompas.id", "whois.id", raw)
	if rec.IsAvailable {
		t.Errorf("expected domain to be registered, got available")
	}
	if rec.Registrar != "PT Jetcoms Netindo" {
		t.Errorf("unexpected registrar: %s", rec.Registrar)
	}
	if rec.ExpiryDate == nil {
		t.Fatalf("expected ExpiryDate, got nil")
	}
	if rec.ExpiryDate.Year() != 2027 {
		t.Errorf("expected year 2027, got %d", rec.ExpiryDate.Year())
	}
	if rec.CreatedDate == nil {
		t.Fatalf("expected CreatedDate, got nil")
	}
	if rec.CreatedDate.Year() != 2014 {
		t.Errorf("expected year 2014, got %d", rec.CreatedDate.Year())
	}
	if len(rec.NameServers) != 2 {
		t.Errorf("expected 2 nameservers, got %d", len(rec.NameServers))
	}
	if len(rec.DomainStatus) != 1 || rec.DomainStatus[0] != "ok" {
		t.Errorf("unexpected domain status: %v", rec.DomainStatus)
	}
}

func TestParseWhoisResponse_Available(t *testing.T) {
	raw := `The queried object does not exist: DOMAIN NOT FOUND
URL of the ICANN Whois Inaccuracy Complaint Form: https://www.icann.org/wicf/
`
	rec := ParseWhoisResponse("notexist123.id", "whois.id", raw)
	if !rec.IsAvailable {
		t.Errorf("expected domain to be available, got registered")
	}
}
