package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

func TestFetchJSONList(t *testing.T) {
	payload := `[{"ruleId":"r1","algo":"token_bucket","windowMs":1000,"limit":10,"enabled":true}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-MD5", "v1")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	src := NewNacosSource(config.NacosCfg{
		Addr:   server.URL,
		DataID: "rules",
		Group:  "DEFAULT_GROUP",
		Format: "json",
	})

	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if got.Version != "v1" {
		t.Fatalf("version = %q", got.Version)
	}
	if len(got.Rules) != 1 || got.Rules[0].RuleID != "r1" {
		t.Fatalf("unexpected rules: %#v", got.Rules)
	}
}

func TestFetchYAMLWrapper(t *testing.T) {
	payload := "rules:\n  - ruleId: r2\n    algo: sliding_window\n    windowMs: 1000\n    limit: 5\n    enabled: true\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	src := NewNacosSource(config.NacosCfg{
		Addr:   server.URL,
		DataID: "rules",
		Group:  "DEFAULT_GROUP",
		Format: "yaml",
	})

	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(got.Rules) != 1 || got.Rules[0].RuleID != "r2" {
		t.Fatalf("unexpected rules: %#v", got.Rules)
	}
	if got.Version == "" {
		t.Fatalf("expected version to be set")
	}
}

func TestFetchAutoDetect(t *testing.T) {
	payload := "- ruleId: r3\n  algo: token_bucket\n  windowMs: 1000\n  limit: 10\n  enabled: true\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	src := NewNacosSource(config.NacosCfg{
		Addr:   server.URL,
		DataID: "rules",
		Group:  "DEFAULT_GROUP",
	})

	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(got.Rules) != 1 || got.Rules[0].RuleID != "r3" {
		t.Fatalf("unexpected rules: %#v", got.Rules)
	}
}
