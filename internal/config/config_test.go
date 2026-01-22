package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWithNacosAndRuleFields(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	data := []byte(`
server:
  httpAddr: ":8080"
redis:
  addr: "127.0.0.1:6379"
  db: 0
  prefix: "pixiu:rls"
  updatesChannel: "pixiu_rls_updates"
features:
  audit: "none"
  localFallback: false
  failPolicy: "fail-open"
nacos:
  addr: "http://127.0.0.1:8848"
  namespace: "ns"
  group: "DEFAULT_GROUP"
  dataId: "pixiu-rules"
  pollIntervalMs: 3000
  timeoutMs: 1500
  failPolicy: "fail-closed"
  format: "yaml"
bootstrapRules:
  - ruleId: "r1"
    match: "/api"
    methods: ["GET", "POST"]
    client: "user"
    priority: 10
    algo: "token_bucket"
    windowMs: 1000
    limit: 100
    burst: 10
    dims: ["ip"]
    enabled: true
`)

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Features.FailPolicy != "fail-open" {
		t.Fatalf("features.failPolicy = %q", cfg.Features.FailPolicy)
	}
	if cfg.Nacos.Addr == "" || cfg.Nacos.DataID == "" {
		t.Fatalf("nacos config not parsed")
	}
	if len(cfg.BootstrapRules) != 1 {
		t.Fatalf("bootstrapRules = %d", len(cfg.BootstrapRules))
	}
	rule := cfg.BootstrapRules[0]
	if rule.Priority != 10 || rule.Client != "user" {
		t.Fatalf("rule fields not parsed")
	}
	if len(rule.Methods) != 2 {
		t.Fatalf("rule methods not parsed")
	}
}

func TestLoadExpandsEnv(t *testing.T) {
	t.Setenv("NACOS_USER", "user1")
	t.Setenv("NACOS_PASS", "pass1")

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	data := []byte(`
nacos:
  addr: "http://127.0.0.1:8848"
  dataId: "rules"
  username: "${NACOS_USER}"
  password: "${NACOS_PASS}"
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Nacos.Username != "user1" || cfg.Nacos.Password != "pass1" {
		t.Fatalf("env not expanded: %q/%q", cfg.Nacos.Username, cfg.Nacos.Password)
	}
}
