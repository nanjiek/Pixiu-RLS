package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/nanjiek/pixiu-rls/internal/config"
	"github.com/nanjiek/pixiu-rls/internal/core"
	"github.com/nanjiek/pixiu-rls/internal/rules"
	"github.com/nanjiek/pixiu-rls/internal/types"
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
	srv       *http.Server // �?内部封装 http.Server
}

const (
	errCodeBadRequest    = 400000
	errCodeForbidden     = 403000
	errCodeNotFound      = 404000
	errCodeInternal      = 500000
	errCodeRateLimit     = 429000
	errCodeRateBlacklist = 429001
	errCodeRateQuota     = 429002
)

type allowContext struct {
	rule config.Rule
	dec  types.Decision
}

type allowHandlerFunc func(r *http.Request) (*allowContext, *ErrorResponse, int)

func NewServer(cfg config.ServerCfg, ruleCache *rules.Cache, engine *core.Engine) *Server {
	return &Server{
		cfg:       cfg,
		ruleCache: ruleCache,
		engine:    engine,
	}
}

func (s *Server) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/v1/allow", allowMiddleware(s.allowLogic)).Methods(http.MethodPost)
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

func (s *Server) allowLogic(r *http.Request) (*allowContext, *ErrorResponse, int) {
	var req AllowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, &ErrorResponse{
			Code:    errCodeBadRequest,
			Message: "Invalid request body",
			Detail:  &ErrorDetail{Reason: err.Error()},
		}, http.StatusBadRequest
	}

	if req.RuleID == "" {
		return nil, &ErrorResponse{
			Code:    errCodeBadRequest,
			Message: "ruleId is required",
		}, http.StatusBadRequest
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
		return nil, &ErrorResponse{
			Code:    errCodeNotFound,
			Message: "Rule not found",
			Detail:  &ErrorDetail{RuleID: req.RuleID},
		}, http.StatusNotFound
	}
	if !rule.Enabled {
		return nil, &ErrorResponse{
			Code:    errCodeForbidden,
			Message: "Rule is disabled",
			Detail:  &ErrorDetail{RuleID: req.RuleID},
		}, http.StatusForbidden
	}

	dec, err := s.engine.Allow(r.Context(), rule, dims, time.Now())
	if err != nil {
		detail := &ErrorDetail{Reason: err.Error(), RuleID: rule.RuleID}
		return nil, &ErrorResponse{
			Code:    errCodeInternal,
			Message: "Allow check failed",
			Detail:  detail,
		}, http.StatusInternalServerError
	}

	return &allowContext{
		rule: rule,
		dec:  dec,
	}, nil, 0
}

func allowMiddleware(next allowHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, apiErr, status := next(r)
		if apiErr != nil {
			writeError(w, status, apiErr)
			return
		}
		if ctx == nil {
			writeError(w, http.StatusInternalServerError, &ErrorResponse{
				Code:    errCodeInternal,
				Message: "Internal Server Error",
			})
			return
		}
		if !ctx.dec.Allowed {
			renderDenied(w, ctx.dec, &ctx.rule)
			return
		}
		renderAllowed(w, ctx.dec, &ctx.rule)
	}
}

func renderAllowed(w http.ResponseWriter, dec types.Decision, rule *config.Rule) {
	setRateLimitHeaders(w, dec, rule, 0)
	writeJSON(w, http.StatusOK, AllowResponse{
		Allowed:      dec.Allowed,
		Remaining:    dec.Remaining,
		RetryAfterMs: dec.RetryAfterMs,
		Reason:       dec.Reason,
	})
}

func renderDenied(w http.ResponseWriter, dec types.Decision, rule *config.Rule) {
	retryAfterSec := retryAfterSeconds(dec.RetryAfterMs)
	setRateLimitHeaders(w, dec, rule, retryAfterSec)
	if retryAfterSec > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSec, 10))
	}
	detail := &ErrorDetail{
		Reason:     dec.Reason,
		RetryAfter: retryAfterSec,
	}
	if rule != nil {
		detail.RuleID = rule.RuleID
	}
	writeError(w, http.StatusTooManyRequests, &ErrorResponse{
		Code:    rateLimitCode(dec.Reason),
		Message: "Too Many Requests",
		Detail:  detail,
	})
}

func setRateLimitHeaders(w http.ResponseWriter, dec types.Decision, rule *config.Rule, retryAfterSec int64) {
	if dec.Remaining >= 0 {
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(maxInt64(dec.Remaining, 0), 10))
	}
	if rule != nil {
		if rule.Limit > 0 {
			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(rule.Limit, 10))
		}
		if rule.RuleID != "" {
			w.Header().Set("X-RateLimit-Rule", rule.RuleID)
		}
	}
	if retryAfterSec > 0 {
		reset := time.Now().Add(time.Duration(retryAfterSec) * time.Second).Unix()
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
	}
}

func retryAfterSeconds(ms int64) int64 {
	if ms <= 0 {
		return 1
	}
	return (ms + 999) / 1000
}

func rateLimitCode(reason string) int {
	reason = strings.ToLower(reason)
	switch {
	case strings.Contains(reason, "blacklist"):
		return errCodeRateBlacklist
	case strings.Contains(reason, "quota_exceeded"):
		return errCodeRateQuota
	default:
		return errCodeRateLimit
	}
}

func (s *Server) createRuleHandler(w http.ResponseWriter, r *http.Request) {
	var req RuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, &ErrorResponse{
			Code:    errCodeBadRequest,
			Message: "Invalid request body",
			Detail:  &ErrorDetail{Reason: err.Error()},
		})
		return
	}
	rule := config.Rule{
		RuleID: req.RuleID, Enabled: req.Enabled, Algo: req.Algo,
		Limit: req.Limit, WindowMs: req.WindowMs, Burst: req.Burst,
		Dims: req.Dims, Quota: req.Quota,
	}
	if err := s.ruleCache.Upsert(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, &ErrorResponse{
			Code:    errCodeInternal,
			Message: "Failed to create rule",
			Detail:  &ErrorDetail{Reason: err.Error(), RuleID: req.RuleID},
		})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "success", "rule_id": req.RuleID})
}
func (s *Server) getRuleHandler(w http.ResponseWriter, r *http.Request) {
	ruleID := mux.Vars(r)["id"]
	rule, ok := s.ruleCache.Get(ruleID)
	if !ok {
		writeError(w, http.StatusNotFound, &ErrorResponse{
			Code:    errCodeNotFound,
			Message: "Rule not found",
			Detail:  &ErrorDetail{RuleID: ruleID},
		})
		return
	}
	writeJSON(w, http.StatusOK, rule)
}
func (s *Server) updateRuleHandler(w http.ResponseWriter, r *http.Request) {
	ruleID := mux.Vars(r)["id"]
	var req RuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, &ErrorResponse{
			Code:    errCodeBadRequest,
			Message: "Invalid request body",
			Detail:  &ErrorDetail{Reason: err.Error()},
		})
		return
	}
	req.RuleID = ruleID
	rule := config.Rule{
		RuleID: req.RuleID, Enabled: req.Enabled, Algo: req.Algo,
		Limit: req.Limit, WindowMs: req.WindowMs, Burst: req.Burst,
		Dims: req.Dims, Quota: req.Quota,
	}
	if err := s.ruleCache.Upsert(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, &ErrorResponse{
			Code:    errCodeInternal,
			Message: "Failed to update rule",
			Detail:  &ErrorDetail{Reason: err.Error(), RuleID: ruleID},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "rule_id": ruleID})
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, resp *ErrorResponse) {
	writeJSON(w, status, resp)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
