package config

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port             string
	DatabaseURL      string
	SessionSecret    string
	S3Endpoint       string
	S3Bucket         string
	S3AccessKey      string
	S3SecretKey      string
	S3Region         string
	GeoGateEnabled   bool
	GeoGateExtraCIDR []string
	TrustedIPHeader  string
	CookieSecure     bool
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func Load() (Config, error) {
	cfg := Config{
		Port:            env("PORT", "8080"),
		DatabaseURL:     env("DATABASE_URL", "postgres://localhost:5432/tenttiarkisto?sslmode=disable"),
		SessionSecret:   env("SESSION_SECRET", ""),
		S3Endpoint:      env("S3_ENDPOINT", "https://s3.fr-par.scw.cloud"),
		S3Bucket:        env("S3_BUCKET", "tenttiarkisto-exams"),
		S3AccessKey:     env("S3_ACCESS_KEY", ""),
		S3SecretKey:     env("S3_SECRET_KEY", ""),
		S3Region:        env("S3_REGION", "fr-par"),
		TrustedIPHeader: env("TRUSTED_FORWARDED_IP_HEADER", "X-Forwarded-For"),
	}

	cookieSecure := strings.ToLower(env("SESSION_COOKIE_SECURE", "true"))
	cfg.CookieSecure = cookieSecure != "0" && cookieSecure != "false" && cookieSecure != "no"

	geoStr := strings.ToLower(env("GEO_GATE_ENABLED", "false"))
	if geoStr == "1" || geoStr == "true" || geoStr == "yes" {
		cfg.GeoGateEnabled = true
	}

	raw := env("GEO_GATE_EXTRA_CIDRS", "")
	for _, c := range strings.Split(raw, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			cfg.GeoGateExtraCIDR = append(cfg.GeoGateExtraCIDR, c)
		}
	}

	if cfg.SessionSecret == "" {
		return cfg, fmt.Errorf("SESSION_SECRET must be set (use a random 32+ byte value)")
	}
	if len(cfg.SessionSecret) < 32 {
		return cfg, fmt.Errorf("SESSION_SECRET must be at least 32 bytes")
	}
	if _, err := url.Parse(cfg.S3Endpoint); err != nil {
		return cfg, fmt.Errorf("invalid S3_ENDPOINT: %w", err)
	}
	if cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
		return cfg, fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}
	return cfg, nil
}

// CSRFKey derives a 32-byte CSRF authentication key from the session secret.
func (c Config) CSRFKey() []byte {
	sum := sha256.Sum256([]byte(c.SessionSecret))
	return sum[:]
}

func (c Config) PortInt() (int, error) {
	p, err := strconv.Atoi(c.Port)
	if err != nil {
		return 0, fmt.Errorf("invalid PORT %q: %w", c.Port, err)
	}
	return p, nil
}
