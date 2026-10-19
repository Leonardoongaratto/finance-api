package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPathsOutsideHealthAndAPIReturnJSONNotFound(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{"/", "/dividas", "/metas/novo", "/assets/app.js", "/favicon.ico"} {
		h.expectErrorCode(h.do(http.MethodGet, path, nil), http.StatusNotFound, "not_found")
	}
	h.expectErrorCode(h.do(http.MethodPost, "/dividas", nil), http.StatusNotFound, "not_found")
	h.expectErrorCode(h.do(http.MethodPost, "/health", nil), http.StatusMethodNotAllowed, "method_not_allowed")
}

func (h *harness) cors(method, path, origin string, extra map[string]string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Host = "127.0.0.1:8080"
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func preflightHeaders(method string) map[string]string {
	return map[string]string{"Access-Control-Request-Method": method, "Access-Control-Request-Headers": "content-type"}
}

func TestCORSPreflightFromAllowedOrigin(t *testing.T) {
	h := newHarness(t)
	rec := h.cors(http.MethodOptions, "/api/accounts", testOrigin, preflightHeaders("POST"))
	h.expect(rec, http.StatusNoContent)
	header := rec.Header()
	if got := header.Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := header.Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, DELETE" {
		t.Fatalf("allow-methods = %q", got)
	}
	if got := header.Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Fatalf("allow-headers = %q", got)
	}
	if !strings.Contains(header.Get("Vary"), "Origin") {
		t.Fatalf("vary = %q", header.Get("Vary"))
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("preflight body = %q", rec.Body.String())
	}
}

func TestCORSActualRequestFromAllowedOrigin(t *testing.T) {
	h := newHarness(t)
	for _, origin := range []string{testOrigin, "http://localhost:4173"} {
		rec := h.cors(http.MethodGet, "/api/dashboard", origin, nil)
		h.expect(rec, http.StatusOK)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("allow-origin = %q, want %q", got, origin)
		}
	}
}

func TestCORSDeniedOriginGetsNoCORSHeaders(t *testing.T) {
	h := newHarness(t)
	for _, origin := range []string{"https://evil.example", "http://127.0.0.1:9999", "http://127.0.0.1:5173.evil.example", "null"} {
		for _, rec := range []*httptest.ResponseRecorder{
			h.cors(http.MethodGet, "/api/dashboard", origin, nil),
			h.cors(http.MethodOptions, "/api/accounts", origin, preflightHeaders("POST")),
		} {
			for name := range rec.Header() {
				if strings.HasPrefix(name, "Access-Control-") {
					t.Fatalf("origin %q got %s", origin, name)
				}
			}
		}
	}
}

func TestCORSRequestWithoutOriginHasNoCORSHeaders(t *testing.T) {
	h := newHarness(t)
	rec := h.cors(http.MethodGet, "/health", "", nil)
	h.expect(rec, http.StatusOK)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unexpected allow-origin")
	}
}

func TestCORSNeverAllowsWildcard(t *testing.T) {
	h := newHarnessWithOrigins(t, []string{"*"})
	rec := h.cors(http.MethodGet, "/health", "https://evil.example", nil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin = %q", got)
	}
}

func TestCORSDisabledWhenNoOriginsConfigured(t *testing.T) {
	h := newHarnessWithOrigins(t, nil)
	rec := h.cors(http.MethodGet, "/health", testOrigin, nil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin = %q", got)
	}
}

func TestCORSPreflightStillRespectsTheHostGuard(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/accounts", nil)
	req.Host = "evil.example"
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	h.expectErrorCode(rec, http.StatusForbidden, "forbidden")
}
