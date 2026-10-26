package geogate

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testGate(t *testing.T, enabled bool, extra []string, header string) *Gate {
	t.Helper()
	g, err := New(enabled, header, extra, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func noopHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// do builds a request; remote must be a bare IP (v4 or v6).
func do(t *testing.T, g *Gate, method, path, remote, forwarded string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if ip := net.ParseIP(remote); ip != nil {
		if ip.To4() != nil {
			req.RemoteAddr = remote + ":1234"
		} else {
			req.RemoteAddr = "[" + remote + "]:1234"
		}
	} else {
		req.RemoteAddr = remote
	}
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	rec := httptest.NewRecorder()
	g.Middleware(http.HandlerFunc(noopHandler)).ServeHTTP(rec, req)
	return rec
}

func TestGateBlocksNonFinnishWrites(t *testing.T) {
	g := testGate(t, true, nil, "X-Forwarded-For")
	g.add(mustCIDR(t, "81.0.0.0/8"))

	if rec := do(t, g, http.MethodPost, "/exams", "8.8.8.8", "8.8.8.8"); rec.Code != http.StatusForbidden {
		t.Errorf("non-FI write: got %d, want 403", rec.Code)
	}
	if rec := do(t, g, http.MethodPost, "/exams", "81.1.2.3", "81.1.2.3"); rec.Code != http.StatusOK {
		t.Errorf("FI write: got %d, want 200", rec.Code)
	}
	for _, m := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if rec := do(t, g, m, "/exams/1", "8.8.8.8", "8.8.8.8"); rec.Code != http.StatusForbidden {
			t.Errorf("non-FI %s: got %d, want 403", m, rec.Code)
		}
	}
}

func TestGateEmptyBodyOnBlock(t *testing.T) {
	g := testGate(t, true, nil, "")
	g.add(mustCIDR(t, "81.0.0.0/8"))
	rec := do(t, g, http.MethodPost, "/login", "8.8.8.8", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("403 must have empty body, got %q", rec.Body.String())
	}
}

func TestGateAllowsReadsFromAnywhere(t *testing.T) {
	g := testGate(t, true, nil, "")
	g.add(mustCIDR(t, "81.0.0.0/8"))
	for _, tc := range []struct{ m, p string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/subjects"},
		{http.MethodGet, "/subjects/1"},
		{http.MethodGet, "/courses"},
		{http.MethodGet, "/exams"},
		{http.MethodGet, "/exams/1"},
		{http.MethodGet, "/exams/1/file"},
		{http.MethodGet, "/healthz"},
		{http.MethodHead, "/"},
		{http.MethodOptions, "/"},
	} {
		if rec := do(t, g, tc.m, tc.p, "8.8.8.8", ""); rec.Code != http.StatusOK {
			t.Errorf("non-FI %s %s: got %d, want 200", tc.m, tc.p, rec.Code)
		}
	}
}

func TestGateBlocksAuthPagesForNonFinnish(t *testing.T) {
	g := testGate(t, true, nil, "")
	g.add(mustCIDR(t, "81.0.0.0/8"))
	for _, path := range []string{"/login", "/register"} {
		if rec := do(t, g, http.MethodGet, path, "8.8.8.8", ""); rec.Code != http.StatusForbidden {
			t.Errorf("non-FI GET %s: got %d, want 403", path, rec.Code)
		}
		if rec := do(t, g, http.MethodGet, path, "81.1.2.3", ""); rec.Code != http.StatusOK {
			t.Errorf("FI GET %s: got %d, want 200", path, rec.Code)
		}
	}
}

func TestGateDisabledIsNoop(t *testing.T) {
	g := testGate(t, false, nil, "")
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if rec := do(t, g, m, "/exams", "8.8.8.8", ""); rec.Code != http.StatusOK {
			t.Errorf("disabled gate %s: got %d, want 200", m, rec.Code)
		}
	}
	if rec := do(t, g, http.MethodGet, "/login", "8.8.8.8", ""); rec.Code != http.StatusOK {
		t.Errorf("disabled gate GET /login: got %d, want 200", rec.Code)
	}
}

func TestGateExtraCIDRsOverride(t *testing.T) {
	g := testGate(t, true, []string{"203.0.113.0/24"}, "")
	if rec := do(t, g, http.MethodPost, "/exams", "203.0.113.7", ""); rec.Code != http.StatusOK {
		t.Errorf("extra CIDR write: got %d, want 200", rec.Code)
	}
	if rec := do(t, g, http.MethodPost, "/exams", "203.0.114.7", ""); rec.Code != http.StatusForbidden {
		t.Errorf("outside extra CIDR: got %d, want 403", rec.Code)
	}
}

func TestGateIPv6(t *testing.T) {
	g := testGate(t, true, nil, "")
	g.add(mustCIDR(t, "2001:708::/32"))

	if rec := do(t, g, http.MethodPost, "/exams", "2001:708::1", ""); rec.Code != http.StatusOK {
		t.Errorf("FI IPv6 write: got %d, want 200", rec.Code)
	}
	if rec := do(t, g, http.MethodPost, "/exams", "2620:0:2d0::1", ""); rec.Code != http.StatusForbidden {
		t.Errorf("non-FI IPv6 write: got %d, want 403", rec.Code)
	}
}

func TestGateEmptyPrefixListAllowsAll(t *testing.T) {
	// a build with no generated prefix data must never lock out traffic
	g := testGate(t, true, nil, "")
	g.v4 = map[byte][]*net.IPNet{}
	g.v6 = nil
	if rec := do(t, g, http.MethodPost, "/exams", "8.8.8.8", ""); rec.Code != http.StatusOK {
		t.Errorf("empty prefix set: got %d, want 200 (fail-open)", rec.Code)
	}
}

func TestClientIPForwardedHeader(t *testing.T) {
	// trusted header: first (leftmost) entry wins
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "51.15.15.15:9999"
	req.Header.Set("X-Forwarded-For", "81.1.2.3, 51.15.15.15")
	if got := clientIP(req, "X-Forwarded-For"); got.String() != "81.1.2.3" {
		t.Errorf("XFF first entry: got %s", got)
	}

	// malformed header falls back to RemoteAddr
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.RemoteAddr = "81.9.9.9:4321"
	req2.Header.Set("X-Forwarded-For", "not-an-ip, 8.8.8.8")
	if got := clientIP(req2, "X-Forwarded-For"); got.String() != "81.9.9.9" {
		t.Errorf("malformed XFF: got %s, want fallback 81.9.9.9", got)
	}

	// no header: RemoteAddr
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "82.4.4.4:1000"
	if got := clientIP(req3, "X-Forwarded-For"); got.String() != "82.4.4.4" {
		t.Errorf("no XFF: got %s, want 82.4.4.4", got)
	}

	// empty header config: RemoteAddr only
	req4 := httptest.NewRequest(http.MethodGet, "/", nil)
	req4.RemoteAddr = "83.3.3.3:1000"
	req4.Header.Set("X-Forwarded-For", "84.4.4.4")
	if got := clientIP(req4, ""); got.String() != "83.3.3.3" {
		t.Errorf("header disabled: got %s, want 83.3.3.3", got)
	}
}

func TestGateFailClosedOnUnparsableIP(t *testing.T) {
	g := testGate(t, true, nil, "")
	g.add(mustCIDR(t, "81.0.0.0/8"))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "garbage"
	rec := httptest.NewRecorder()
	g.Middleware(http.HandlerFunc(noopHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("undeterminable IP: got %d, want 403 (fail closed)", rec.Code)
	}
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return n
}
