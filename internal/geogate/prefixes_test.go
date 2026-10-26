package geogate

import (
	"io"
	"log/slog"
	"net"
	"testing"
)

// TestEmbeddedFinnishPrefixes sanity-checks the generated prefix set
// against long-standing Finnish allocations (Funet etc.) and obvious
// foreign addresses. The Finnish allocations used here have been
// stable since the 1990s, so regenerating the data should not break
// this test. If it does, inspect the generated file before committing.
func TestEmbeddedFinnishPrefixes(t *testing.T) {
	g, err := New(true, "X-Forwarded-For", nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	fiIPs := []string{
		"193.166.0.10", // Funet (FI since 1993)
		"130.233.0.10", // Funet / Aalto
		"128.214.0.10", // University of Helsinki
		"2001:708::10", // Funet IPv6
	}
	for _, s := range fiIPs {
		if !g.allowed(net.ParseIP(s)) {
			t.Errorf("expected Finnish IP %s to be allowed by embedded prefixes", s)
		}
	}

	foreignIPs := []string{
		"8.8.8.8",      // US (Google)
		"1.1.1.1",      // AU (Cloudflare)
		"9.9.9.9",      // DE (Quad9)
		"2606:4700::1", // US (Cloudflare v6)
	}
	for _, s := range foreignIPs {
		if g.allowed(net.ParseIP(s)) {
			t.Errorf("expected foreign IP %s to be blocked", s)
		}
	}
}
