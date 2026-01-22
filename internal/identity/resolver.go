package identity

import (
	"errors"
	"net"
	"net/http"
	"strings"
)

const (
	KindUser   = "user"
	KindIP     = "ip"
	KindAPIKey = "api_key"
)

// ClientKey represents a normalized client identifier.
type ClientKey struct {
	Kind string
	ID   string
	Key  string
}

// Resolver resolves a client key from an HTTP request.
type Resolver struct {
	UserHeader string
	APIKeyHdr  string
	IPHeader   string
}

func NewResolver() *Resolver {
	return &Resolver{
		UserHeader: "X-User-Id",
		APIKeyHdr:  "X-API-Key",
		IPHeader:   "X-Forwarded-For",
	}
}

// Resolve resolves client identity in order: user -> api_key -> ip.
func (r *Resolver) Resolve(req *http.Request) (ClientKey, error) {
	if req == nil {
		return ClientKey{}, errors.New("nil request")
	}

	if user := strings.TrimSpace(req.Header.Get(r.UserHeader)); user != "" {
		return newKey(KindUser, user), nil
	}

	if apiKey := strings.TrimSpace(req.Header.Get(r.APIKeyHdr)); apiKey != "" {
		return newKey(KindAPIKey, apiKey), nil
	}

	if ip := parseForwardedIP(req.Header.Get(r.IPHeader)); ip != "" {
		return newKey(KindIP, ip), nil
	}

	if ip := parseRemoteIP(req.RemoteAddr); ip != "" {
		return newKey(KindIP, ip), nil
	}

	return ClientKey{}, errors.New("no client identity found")
}

func newKey(kind, id string) ClientKey {
	return ClientKey{
		Kind: kind,
		ID:   id,
		Key:  kind + ":" + id,
	}
}

func parseForwardedIP(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func parseRemoteIP(remoteAddr string) string {
	if remoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return remoteAddr
}
