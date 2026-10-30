/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"k8s.io/klog/v2"
)

const (
	defaultClockSkew    = 30 * time.Second
	defaultJWKSCacheTTL = 10 * time.Minute
	// jwksMinRefresh bounds how often an unknown key id can trigger a fetch, so a
	// stream of garbage tokens cannot turn into a stream of requests to the issuer.
	jwksMinRefresh  = 10 * time.Second
	maxJWKSBodySize = 1 << 20
)

// signatureAlgorithms are the algorithms accepted for JWTs. "none" and the HMAC
// family are not among them: a token signed with a shared secret proves nothing here.
var signatureAlgorithms = []jose.SignatureAlgorithm{
	jose.ES256, jose.ES384, jose.ES512, jose.RS256, jose.RS384, jose.RS512, jose.PS256, jose.PS384, jose.PS512, jose.EdDSA,
}

// OIDCConfig configures an OIDC/JWT authenticator.
type OIDCConfig struct {
	Name string
	// Issuer is the exact "iss" value expected.
	Issuer string
	// Audience must be one of the token's "aud" values. It is required: a token
	// minted for another service must not be accepted here.
	Audience string
	// JWKSURL is where the signing keys are; empty means the jwks_uri of the
	// issuer's discovery document.
	JWKSURL string
	// JWKSCacheTTL is how long fetched keys are used before a refresh is attempted.
	// A failed refresh keeps the old keys.
	JWKSCacheTTL time.Duration
	// ClockSkew is the leeway for exp, nbf and iat.
	ClockSkew time.Duration

	IDClaim       string
	UsernameClaim string
	EmailClaim    string
	GroupsClaim   string

	HTTPClient *http.Client
	// Now overrides the clock in tests.
	Now func() time.Time
}

// OIDCAuthenticator validates JWTs issued by one OIDC issuer.
type OIDCAuthenticator struct {
	cfg OIDCConfig

	mu          sync.Mutex
	keys        *jose.JSONWebKeySet
	fetchedAt   time.Time
	lastAttempt time.Time
	jwksURL     string
}

// NewOIDCAuthenticator builds an authenticator. Issuer and audience are required.
func NewOIDCAuthenticator(cfg OIDCConfig) (*OIDCAuthenticator, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("oidc authenticator: issuer is required")
	}
	if cfg.Audience == "" {
		return nil, errors.New("oidc authenticator: audience is required")
	}
	if cfg.Name == "" {
		cfg.Name = "oidc:" + cfg.Issuer
	}
	if cfg.JWKSCacheTTL <= 0 {
		cfg.JWKSCacheTTL = defaultJWKSCacheTTL
	}
	if cfg.ClockSkew < 0 {
		return nil, errors.New("oidc authenticator: clock_skew must not be negative")
	}
	if cfg.ClockSkew == 0 {
		cfg.ClockSkew = defaultClockSkew
	}
	if cfg.IDClaim == "" {
		cfg.IDClaim = "sub"
	}
	if cfg.UsernameClaim == "" {
		cfg.UsernameClaim = "preferred_username"
	}
	if cfg.EmailClaim == "" {
		cfg.EmailClaim = "email"
	}
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "groups"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &OIDCAuthenticator{cfg: cfg, jwksURL: cfg.JWKSURL}, nil
}

// Name implements Authenticator.
func (a *OIDCAuthenticator) Name() string { return a.cfg.Name }

// Authenticate implements Authenticator. A bearer JWT whose issuer is not ours is
// not ours to judge (ErrNotApplicable), so several issuers can share a chain.
func (a *OIDCAuthenticator) Authenticate(ctx context.Context, cred Credentials) (*Identity, error) {
	raw := cred.Bearer
	if strings.Count(raw, ".") != 2 {
		return nil, ErrNotApplicable
	}
	tok, err := jwt.ParseSigned(raw, signatureAlgorithms)
	if err != nil || len(tok.Headers) != 1 {
		return nil, ErrNotApplicable
	}
	var peek struct {
		Issuer string `json:"iss"`
	}
	if err = tok.UnsafeClaimsWithoutVerification(&peek); err != nil || peek.Issuer != a.cfg.Issuer {
		return nil, ErrNotApplicable
	}

	key, err := a.verificationKey(ctx, tok.Headers[0].KeyID)
	if err != nil {
		return nil, &UnavailableError{Authenticator: a.Name(), Err: err}
	}
	if key == nil {
		return nil, fmt.Errorf("%w: unknown signing key", ErrUnauthenticated)
	}
	if key.Algorithm != "" && key.Algorithm != tok.Headers[0].Algorithm {
		return nil, fmt.Errorf("%w: algorithm does not match the key", ErrUnauthenticated)
	}
	var claims map[string]any
	var std jwt.Claims
	if err = tok.Claims(key.Key, &std, &claims); err != nil {
		return nil, fmt.Errorf("%w: bad signature", ErrUnauthenticated)
	}
	expected := jwt.Expected{Issuer: a.cfg.Issuer, AnyAudience: []string{a.cfg.Audience}, Time: a.cfg.Now()}
	if err = std.ValidateWithLeeway(expected, a.cfg.ClockSkew); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	if std.Expiry == nil {
		return nil, fmt.Errorf("%w: no expiry", ErrUnauthenticated)
	}
	return a.identity(std, claims)
}

func (a *OIDCAuthenticator) identity(std jwt.Claims, claims map[string]any) (*Identity, error) {
	id := &Identity{
		ID:     claimString(claims, a.cfg.IDClaim),
		Name:   claimString(claims, a.cfg.UsernameClaim),
		Email:  claimString(claims, a.cfg.EmailClaim),
		Groups: claimStrings(claims, a.cfg.GroupsClaim),
		Jti:    std.ID,
		Expiry: std.Expiry.Time(),
	}
	if id.ID == "" {
		return nil, fmt.Errorf("%w: no %q claim", ErrUnauthenticated, a.cfg.IDClaim)
	}
	if id.Name == "" {
		id.Name = id.ID
	}
	// RFC 8693 "act": the party acting for the subject. The actor is act.client_id, else act.sub, the
	// same reading as the introspection authenticator, so one token names one actor whichever
	// authenticator accepts it: a token whose act.client_id and act.sub differ is the client_id's.
	if act, ok := claims["act"]; ok {
		m, isMap := act.(map[string]any)
		actor, _ := m["client_id"].(string)
		if actor == "" {
			actor, _ = m["sub"].(string)
		}
		if !isMap || actor == "" {
			// A malformed delegation must not be read as a direct login.
			return nil, fmt.Errorf("%w: malformed act claim", ErrUnauthenticated)
		}
		id.Actor = actor
		id.Scopes = tokenScopes(claims)
	}
	if r, ok := claims["restricted"].(bool); ok {
		id.Restricted = r
	}
	return id, nil
}

// verificationKey returns the key for a key id. A key id the cache does not know
// triggers one refresh (rate limited); a refresh that fails keeps the old keys, and
// only reports unavailability when there is nothing to verify with.
func (a *OIDCAuthenticator) verificationKey(ctx context.Context, kid string) (*jose.JSONWebKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.cfg.Now()
	stale := a.keys == nil || now.Sub(a.fetchedAt) > a.cfg.JWKSCacheTTL
	if k := a.find(kid); k != nil && !stale {
		return k, nil
	}
	var fetchErr error
	if a.keys == nil || now.Sub(a.lastAttempt) >= jwksMinRefresh {
		a.lastAttempt = now
		if set, err := a.fetchKeys(ctx); err != nil {
			fetchErr = err
			klog.ErrorS(err, "failed to refresh signing keys", "authenticator", a.Name())
		} else {
			a.keys, a.fetchedAt = set, now
		}
	}
	if k := a.find(kid); k != nil {
		return k, nil
	}
	if a.keys == nil {
		return nil, fmt.Errorf("signing keys not available: %w", fetchErr)
	}
	return nil, nil
}

func (a *OIDCAuthenticator) find(kid string) *jose.JSONWebKey {
	if a.keys == nil {
		return nil
	}
	if kid == "" {
		if len(a.keys.Keys) == 1 {
			return &a.keys.Keys[0]
		}
		return nil
	}
	if ks := a.keys.Key(kid); len(ks) > 0 {
		return &ks[0]
	}
	return nil
}

func (a *OIDCAuthenticator) fetchKeys(ctx context.Context) (*jose.JSONWebKeySet, error) {
	url := a.jwksURL
	if url == "" {
		var doc struct {
			JWKSURI string `json:"jwks_uri"`
		}
		if err := a.getJSON(ctx, strings.TrimRight(a.cfg.Issuer, "/")+"/.well-known/openid-configuration", &doc); err != nil {
			return nil, fmt.Errorf("discovery: %w", err)
		}
		if doc.JWKSURI == "" {
			return nil, errors.New("discovery document has no jwks_uri")
		}
		url = doc.JWKSURI
	}
	var set jose.JSONWebKeySet
	if err := a.getJSON(ctx, url, &set); err != nil {
		return nil, err
	}
	if len(set.Keys) == 0 {
		return nil, errors.New("key set is empty")
	}
	if a.jwksURL == "" {
		a.jwksURL = url
	}
	return &set, nil
}

func (a *OIDCAuthenticator) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := a.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBodySize))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func claimString(claims map[string]any, name string) string {
	s, _ := claims[name].(string)
	return s
}

// claimStrings reads a string-list claim; a single string is a list of one.
func claimStrings(claims map[string]any, name string) []string {
	switch v := claims[name].(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// tokenScopes reads "scope" (space separated, RFC 8693) or "scp" (a list).
func tokenScopes(claims map[string]any) []string {
	if s, ok := claims["scope"].(string); ok {
		return strings.Fields(s)
	}
	return claimStrings(claims, "scp")
}
