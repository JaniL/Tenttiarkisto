package geogate

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// Gate restricts state-changing requests (and the login/registration
// pages) to clients connecting from Finnish IP address space. The
// Finnish prefixes are embedded at build time (see fi_prefixes.go)
// and generated from the RIPE NCC delegated statistics:
//
//	https://ftp.ripe.net/pub/stats/ripencc/delegated-ripencc-extended-latest
type Gate struct {
	enabled       bool
	trustedHeader string
	v4            map[byte][]*net.IPNet
	v6            []*net.IPNet
	logger        *slog.Logger
}

// New builds a gate. extraCIDRs are always allowed in addition to the
// embedded Finnish prefixes (e.g. an office range). trustedHeader names
// the reverse-proxy header carrying the client IP (default
// "X-Forwarded-For"; empty means use RemoteAddr only).
//
// If neither embedded nor extra prefixes are present the gate fails
// open (allows everything): a broken/missing prefix regeneration must
// not lock out the entire application.
func New(enabled bool, trustedHeader string, extraCIDRs []string, logger *slog.Logger) (*Gate, error) {
	g := &Gate{
		enabled:       enabled,
		trustedHeader: trustedHeader,
		logger:        logger,
		v4:            map[byte][]*net.IPNet{},
	}

	all := append([]string{}, fiPrefixes...)
	all = append(all, extraCIDRs...)
	for _, cidr := range all {
		c := strings.TrimSpace(cidr)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("geogate: invalid CIDR %q: %w", c, err)
		}
		g.add(n)
	}
	if g.empty() {
		logger.Warn("geogate: no Finnish prefixes embedded and no extra CIDRs configured; gate fails open until regenerated (run: make geo-update)")
	}
	return g, nil
}

func (g *Gate) add(n *net.IPNet) {
	if n.IP.To4() != nil {
		key := n.IP.To4()[0]
		g.v4[key] = append(g.v4[key], n)
	} else {
		g.v6 = append(g.v6, n)
	}
}

// Enabled reports whether the gate is active.
func (g *Gate) Enabled() bool { return g.enabled }

// empty reports whether no allow prefixes are present (fail-open state).
func (g *Gate) empty() bool {
	if len(g.v6) > 0 {
		return false
	}
	for _, nets := range g.v4 {
		if len(nets) > 0 {
			return false
		}
	}
	return true
}

func (g *Gate) allowed(ip net.IP) bool {
	if g.empty() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		for _, n := range g.v4[ip4[0]] {
			if n.Contains(ip4) {
				return true
			}
		}
		return false
	}
	for _, n := range g.v6 {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP determines the originating client IP. The first (leftmost)
// entry of the trusted forwarded header is used when present, otherwise
// the socket address. The header is only trustworthy behind the edge
// proxy that sets it (Scaleway serverless does).
func clientIP(r *http.Request, trustedHeader string) net.IP {
	if trustedHeader != "" {
		if v := r.Header.Get(trustedHeader); v != "" {
			first := strings.Split(v, ",")[0]
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip
			}
		}
	}
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return net.ParseIP(host)
}

func isWriteMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func isAuthPage(path string) bool {
	return path == "/login" || path == "/register"
}

// Middleware enforces the gate. Blocked requests receive a bare 403
// with an empty body; each block is logged for observability.
func (g *Gate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.enabled {
			ip := clientIP(r, g.trustedHeader)
			blocked := ip == nil ||
				(isWriteMethod(r.Method) && !g.allowed(ip)) ||
				(r.Method == http.MethodGet && isAuthPage(r.URL.Path) && !g.allowed(ip))
			if blocked {
				var ipStr string
				if ip != nil {
					ipStr = ip.String()
				}
				g.logger.Warn("geogate: blocked non-Finnish request",
					"ip", ipStr, "method", r.Method, "path", r.URL.Path)
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
