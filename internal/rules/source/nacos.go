package source

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

import (
	"gopkg.in/yaml.v3"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
)

const defaultNacosGroup = "DEFAULT_GROUP"

// NacosSource pulls rules from Nacos config center via HTTP.
type NacosSource struct {
	cfg    config.NacosCfg
	client *http.Client
	log    *slog.Logger
}

func NewNacosSource(cfg config.NacosCfg) *NacosSource {
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &NacosSource{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
		log:    slog.Default(),
	}
}

func (s *NacosSource) Fetch(ctx context.Context) (RulesPayload, error) {
	if !s.cfg.Enabled() {
		return RulesPayload{}, errors.New("nacos is disabled")
	}

	reqURL, err := s.buildURL()
	if err != nil {
		return RulesPayload{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return RulesPayload{}, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return RulesPayload{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RulesPayload{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return RulesPayload{}, fmt.Errorf("nacos fetch failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	version := resp.Header.Get("Content-MD5")
	if version == "" {
		sum := md5.Sum(body)
		version = fmt.Sprintf("%x", sum[:])
	}

	rules, err := parseRules(body, s.cfg.Format)
	if err != nil {
		return RulesPayload{}, err
	}

	return RulesPayload{
		Rules:   rules,
		Version: version,
	}, nil
}

func (s *NacosSource) buildURL() (string, error) {
	base, err := url.Parse(s.cfg.Addr)
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/nacos/v1/cs/configs"

	group := s.cfg.Group
	if group == "" {
		group = defaultNacosGroup
	}

	q := base.Query()
	q.Set("dataId", s.cfg.DataID)
	q.Set("group", group)
	if s.cfg.Namespace != "" {
		q.Set("tenant", s.cfg.Namespace)
	}
	if s.cfg.Username != "" {
		q.Set("username", s.cfg.Username)
		q.Set("password", s.cfg.Password)
	}
	base.RawQuery = q.Encode()

	return base.String(), nil
}

func parseRules(raw []byte, format string) ([]config.Rule, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("empty rules payload")
	}

	format = strings.ToLower(strings.TrimSpace(format))

	if format == "json" || format == "" {
		if rules, ok := tryParseJSON(trimmed); ok {
			return rules, nil
		}
		if format == "json" {
			return nil, errors.New("invalid json rules payload")
		}
	}

	if format == "yaml" || format == "" {
		if rules, ok := tryParseYAML(trimmed); ok {
			return rules, nil
		}
		if format == "yaml" {
			return nil, errors.New("invalid yaml rules payload")
		}
	}

	slog.Warn("failed to parse rules payload; unknown format", "format", format)
	return nil, errors.New("unsupported rules payload format")
}

func tryParseJSON(raw []byte) ([]config.Rule, bool) {
	var list []config.Rule
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, true
	}
	var wrapper struct {
		Rules []config.Rule `json:"rules"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Rules != nil {
		return wrapper.Rules, true
	}
	return nil, false
}

func tryParseYAML(raw []byte) ([]config.Rule, bool) {
	var list []config.Rule
	if err := yaml.Unmarshal(raw, &list); err == nil {
		return list, true
	}
	var wrapper struct {
		Rules []config.Rule `yaml:"rules"`
	}
	if err := yaml.Unmarshal(raw, &wrapper); err == nil && wrapper.Rules != nil {
		return wrapper.Rules, true
	}
	return nil, false
}
