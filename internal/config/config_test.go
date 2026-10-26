package config

import "testing"

func TestLoadRequiresSessionSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("S3_ACCESS_KEY", "x")
	t.Setenv("S3_SECRET_KEY", "y")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when SESSION_SECRET is unset")
	}
}

func TestLoadValidatesSecretLength(t *testing.T) {
	t.Setenv("SESSION_SECRET", "short")
	t.Setenv("S3_ACCESS_KEY", "x")
	t.Setenv("S3_SECRET_KEY", "y")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short SESSION_SECRET")
	}
}

func TestLoadParsesFlags(t *testing.T) {
	t.Setenv("SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("S3_ACCESS_KEY", "SCWXXXX")
	t.Setenv("S3_SECRET_KEY", "secret")
	t.Setenv("GEO_GATE_ENABLED", "1")
	t.Setenv("GEO_GATE_EXTRA_CIDRS", "10.0.0.0/8, 192.168.1.0/24,bad")
	t.Setenv("SESSION_COOKIE_SECURE", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.GeoGateEnabled {
		t.Error("GEO_GATE_ENABLED=1 should enable gate")
	}
	if cfg.CookieSecure {
		t.Error("SESSION_COOKIE_SECURE=false should disable secure cookie")
	}
	if len(cfg.GeoGateExtraCIDR) != 3 {
		t.Errorf("expected 3 raw CIDRs parsed, got %d", len(cfg.GeoGateExtraCIDR))
	}
	if cfg.TrustedIPHeader != "X-Forwarded-For" {
		t.Errorf("default trusted header wrong: %q", cfg.TrustedIPHeader)
	}
	if cfg.S3Endpoint != "https://s3.fr-par.scw.cloud" {
		t.Errorf("default S3 endpoint wrong: %q", cfg.S3Endpoint)
	}
	if len(cfg.CSRFKey()) != 32 {
		t.Errorf("CSRF key must be 32 bytes, got %d", len(cfg.CSRFKey()))
	}
}
