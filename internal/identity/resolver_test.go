package identity

import (
	"net/http"
	"strings"
	"testing"
)

func TestResolveUserHeaderWins(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-User-Id", "user-1")
	req.Header.Set("X-API-Key", "key-1")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	resolver := NewResolver()
	key, err := resolver.Resolve(req)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if key.Kind != KindUser || key.ID != "user-1" {
		t.Fatalf("unexpected key: %#v", key)
	}
	if !strings.HasPrefix(key.Key, "user:") {
		t.Fatalf("unexpected normalized key: %s", key.Key)
	}
}

func TestResolveAPIKey(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-API-Key", "key-1")

	resolver := NewResolver()
	key, err := resolver.Resolve(req)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if key.Kind != KindAPIKey || key.ID != "key-1" {
		t.Fatalf("unexpected key: %#v", key)
	}
}

func TestResolveForwardedIP(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")

	resolver := NewResolver()
	key, err := resolver.Resolve(req)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if key.Kind != KindIP || key.ID != "10.0.0.1" {
		t.Fatalf("unexpected key: %#v", key)
	}
}

func TestResolveRemoteAddr(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	resolver := NewResolver()
	key, err := resolver.Resolve(req)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if key.Kind != KindIP || key.ID != "192.168.1.1" {
		t.Fatalf("unexpected key: %#v", key)
	}
}

func TestResolveEmpty(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = ""

	resolver := NewResolver()
	if _, err := resolver.Resolve(req); err == nil {
		t.Fatal("expected error for missing identity")
	}
}
