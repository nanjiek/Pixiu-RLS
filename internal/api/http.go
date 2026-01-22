package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"time"
)

import (
	"github.com/gorilla/mux"
)

import (
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/core"
	"github.com/nanjiek/pixiu-rls/internal/rules"
)

type RuleRequest struct {
	RuleID   string          `json:"rule_id"`
	Enabled  bool            `json:"enabled"`
	Algo     string          `json:"algo"`
	Limit    int64           `json:"limit"`
	WindowMs int64           `json:"window_ms"`
	Burst    int64           `json:"burst"`
	Dims     []string        `json:"dims"`
	Quota    config.QuotaCfg `json:"quota"`
}

type Server struct {
	cfg       config.ServerCfg
	ruleCache *rules.Cache
	engine    *core.Engine
	srv       *http.Server // ← 内部封装 http.Server
}

func NewServer(cfg config.ServerCfg, ruleCache *rules.Cache, engine *core.Engine) *Server {
	return &Server{
		cfg:       cfg,
		ruleCache: ruleCache,
		engine:    engine,
	}
}

func (s *Server) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/v1/allow", s.allowHandler).Methods(http.MethodPost)
	r.HandleFunc("/v1/rules", s.createRuleHandler).Methods(http.MethodPost)
	r.HandleFunc("/v1/rules/{id}", s.getRuleHandler).Methods(http.MethodGet)
	r.HandleFunc("/v1/rules/{id}", s.updateRuleHandler).Methods(http.MethodPut)
	// 先别注册 DELETE，等你实现了 Cache.Delete 再放开
	// r.HandleFunc("/v1/rules/{id}", s.deleteRuleHandler).Methods(http.MethodDelete)
}

func (s *Server) ListenAndServe() error {
	r := mux.NewRouter()
	s.RegisterRoutes(r)
	s.srv = &http.Server{
		Addr:              s.cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// ---------------- Handlers ----------------

func (s *Server) allowHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req AllowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResp(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.RuleID == "" {
		errResp(w, http.StatusBadRequest, "ruleId is required")
		return
	}

	dims := req.Dims
	if dims == nil {
		dims = make(map[string]string)
	}
	if _, ok := dims["ip"]; !ok {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		dims["ip"] = ip
	}
	if _, ok := dims["route"]; !ok {
		dims["route"] = r.URL.Path
	}

	rule, exists := s.ruleCache.Get(req.RuleID)
	if !exists {
		errResp(w, http.StatusNotFound, "rule not found: "+req.RuleID)
		return // ✅ 必须 return
	}
	if !rule.Enabled {
		errResp(w, http.StatusForbidden, "rule is disabled: "+req.RuleID)
		return
	}

	dec, err := s.engine.Allow(r.Context(), rule, dims, time.Now())
	if err != nil {
		errResp(w, http.StatusInternalServerError, "allow check failed: "+err.Error())
		return
	}

	if !dec.Allowed {
		if dec.RetryAfterMs > 0 {
			w.Header().Set("Retry-After", strconv.FormatInt((dec.RetryAfterMs+999)/1000, 10))
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}

	_ = json.NewEncoder(w).Encode(AllowResponse{
		Allowed:      dec.Allowed,
		Remaining:    dec.Remaining,
		RetryAfterMs: dec.RetryAfterMs,
		Reason:       dec.Reason,
	})
}

func (s *Server) createRuleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req RuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResp(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	rule := config.Rule{
		RuleID: req.RuleID, Enabled: req.Enabled, Algo: req.Algo,
		Limit: req.Limit, WindowMs: req.WindowMs, Burst: req.Burst,
		Dims: req.Dims, Quota: req.Quota,
	}
	if err := s.ruleCache.Upsert(r.Context(), rule); err != nil {
		errResp(w, http.StatusInternalServerError, "failed to create rule: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "rule_id": req.RuleID})
}

func (s *Server) getRuleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ruleID := mux.Vars(r)["id"]
	rule, ok := s.ruleCache.Get(ruleID)
	if !ok {
		errResp(w, http.StatusNotFound, "rule not found: "+ruleID)
		return
	}
	_ = json.NewEncoder(w).Encode(rule)
}

func (s *Server) updateRuleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ruleID := mux.Vars(r)["id"]
	var req RuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResp(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	req.RuleID = ruleID
	rule := config.Rule{
		RuleID: req.RuleID, Enabled: req.Enabled, Algo: req.Algo,
		Limit: req.Limit, WindowMs: req.WindowMs, Burst: req.Burst,
		Dims: req.Dims, Quota: req.Quota,
	}
	if err := s.ruleCache.Upsert(r.Context(), rule); err != nil {
		errResp(w, http.StatusInternalServerError, "failed to update rule: "+err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "rule_id": ruleID})
}

func errResp(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
