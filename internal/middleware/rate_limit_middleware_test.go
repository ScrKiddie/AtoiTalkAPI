package middleware

import (
	"net/http"
	"testing"
)

func TestGetIPUntrustedRemote(t *testing.T) {
	m := &RateLimitMiddleware{
		trustedProxyCIDRs: parseTrustedProxyCIDRs([]string{"10.0.0.0/8"}),
	}

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.20:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")

	got := m.getIP(req)
	want := "198.51.100.20"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestGetIPTrustedProxyUsesRightMostUntrusted(t *testing.T) {
	m := &RateLimitMiddleware{
		trustedProxyCIDRs: parseTrustedProxyCIDRs([]string{"10.0.0.0/8"}),
	}

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2, 198.51.100.10")

	got := m.getIP(req)
	want := "198.51.100.10"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestGetIPTrustedProxySkipsTrustedChain(t *testing.T) {
	m := &RateLimitMiddleware{
		trustedProxyCIDRs: parseTrustedProxyCIDRs([]string{"10.0.0.0/8"}),
	}

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.1.1.1")

	got := m.getIP(req)
	want := "203.0.113.10"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestGetIPTrustedProxyFallbackToXRealIP(t *testing.T) {
	m := &RateLimitMiddleware{
		trustedProxyCIDRs: parseTrustedProxyCIDRs([]string{"10.0.0.0/8"}),
	}

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Real-IP", "198.51.100.11")

	got := m.getIP(req)
	want := "198.51.100.11"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
