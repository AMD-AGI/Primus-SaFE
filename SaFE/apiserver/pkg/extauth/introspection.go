/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"k8s.io/klog/v2"

	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
)

const (
	defaultIntrospectTimeout = 5 * time.Second
	defaultCacheTTL          = 30 * time.Second
	defaultNegativeTTL       = 5 * time.Second
	// DefaultStaleIfError is how long after its TTL a cached accepted credential
	// still counts while the authenticator is unreachable.
	DefaultStaleIfError = 30 * time.Minute

	maxCacheEntries     = 10000
	maxIntrospectBody   = 1 << 20
	maxClientSecretSize = 4096
)

// IntrospectionConfig configures an RFC 7662 token introspection authenticator.
type IntrospectionConfig struct {
	Name string
	// URL is the introspection endpoint. When it is empty, Issuer's discovery document
	// (/.well-known/openid-configuration) names it, looked up on first use.
	URL string
	// Issuer, when set, is checked against the discovery document and against the "iss" of
	// every answer that has one: an answer about a token of another issuer is a rejection.
	Issuer string
	// CABundle is PEM text, or the path of a PEM file, that signs the server's certificate.
	CABundle string
	// ClientID and ClientSecretFile authenticate this apiserver to the introspection endpoint
	// (HTTP Basic). The secret is read from the file, a mounted Secret, on every call, so a
	// rotated Secret is picked up without a restart; it is never logged.
	ClientID         string
	ClientSecretFile string
	Timeout          time.Duration
	// CacheTTL is how long an accepted credential is used without asking again.
	CacheTTL time.Duration
	// CacheNegativeTTL is how long a rejected credential is refused without asking again.
	// Only an answer of "not active" is cached this way, never an error.
	CacheNegativeTTL time.Duration
	// StaleIfError lets an accepted credential keep working for this long after
	// CacheTTL while the endpoint is unreachable or failing. Zero uses
	// DefaultStaleIfError; a negative value turns it off.
	StaleIfError time.Duration

	HTTPClient *http.Client
	Now        func() time.Time
}

// IntrospectionAuthenticator asks an OAuth 2.0 token introspection endpoint (RFC 7662)
// who a credential belongs to.
//
// The request carries the bearer token, or the session cookie's value when there is no
// bearer token, as "token". Two non-standard fields carry what an opaque credential needs:
// "user_type" (the SaFE user type of a session) and "client_ip" (the caller's address, for
// an API key's allow list). The answer is a JSON object. "active": false rejects the
// credential. For an active one, "sub" is the SaFE user id; the identity is described by
// username, name, email, groups, roles (both groups and roles are group names for the
// configured group mappings), scope, jti, exp, an optional "restricted" flag, and "act",
// the party a delegated credential acts through ({"client_id": ...} or {"sub": ...}).
type IntrospectionAuthenticator struct {
	cfg   IntrospectionConfig
	group singleflight.Group

	mu    sync.Mutex
	cache map[string]*cacheEntry

	epMu     sync.Mutex
	endpoint string // discovered once, when cfg.URL is empty
}

type cacheEntry struct {
	id      *Identity // nil: a cached rejection
	fetched time.Time
}

// NewIntrospectionAuthenticator builds an authenticator. A configuration that cannot work,
// including a client secret file that cannot be read now, is an error.
func NewIntrospectionAuthenticator(cfg IntrospectionConfig) (*IntrospectionAuthenticator, error) {
	if cfg.URL == "" && cfg.Issuer == "" {
		return nil, errors.New("introspection authenticator: url or issuer is required")
	}
	if cfg.ClientID == "" || cfg.ClientSecretFile == "" {
		return nil, errors.New("introspection authenticator: client_id and client_secret_file are required")
	}
	if _, err := readClientSecret(cfg.ClientSecretFile); err != nil {
		return nil, fmt.Errorf("introspection authenticator: client_secret_file: %w", err)
	}
	if cfg.Name == "" {
		cfg.Name = "introspection"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultIntrospectTimeout
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
	}
	if cfg.CacheNegativeTTL <= 0 {
		cfg.CacheNegativeTTL = defaultNegativeTTL
	}
	if cfg.StaleIfError == 0 {
		cfg.StaleIfError = DefaultStaleIfError
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Issuer = strings.TrimRight(cfg.Issuer, "/")
	if cfg.HTTPClient == nil {
		tr := &http.Transport{Proxy: nil}
		if cfg.CABundle != "" {
			pool, err := certPool(cfg.CABundle)
			if err != nil {
				return nil, fmt.Errorf("introspection authenticator: ca_bundle: %w", err)
			}
			tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		}
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout, Transport: tr}
	}
	return &IntrospectionAuthenticator{cfg: cfg, cache: map[string]*cacheEntry{}}, nil
}

func certPool(bundle string) (*x509.CertPool, error) {
	data := []byte(bundle)
	if !strings.Contains(bundle, "-----BEGIN") {
		var err error
		if data, err = os.ReadFile(bundle); err != nil {
			return nil, err
		}
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, errors.New("no certificate found")
	}
	return pool, nil
}

// readClientSecret reads the secret from its file. Trailing whitespace (the newline an
// editor or `echo` leaves) is not part of the secret. The error never contains the secret.
func readClientSecret(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxClientSecretSize+1))
	if err != nil {
		return "", err
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", errors.New("the file is empty")
	}
	if len(data) > maxClientSecretSize {
		return "", errors.New("the file is too large to be a client secret")
	}
	return secret, nil
}

// Name implements Authenticator.
func (a *IntrospectionAuthenticator) Name() string { return a.cfg.Name }

// Authenticate implements Authenticator.
func (a *IntrospectionAuthenticator) Authenticate(ctx context.Context, cred Credentials) (*Identity, error) {
	if cred.Empty() {
		return nil, ErrNotApplicable
	}
	key := cacheKey(cred)
	now := a.cfg.Now()

	a.mu.Lock()
	e := a.cache[key]
	a.mu.Unlock()
	if e != nil {
		age := now.Sub(e.fetched)
		switch {
		case e.id == nil && age < a.cfg.CacheNegativeTTL:
			return nil, ErrUnauthenticated
		case e.id != nil && age < a.cfg.CacheTTL && !expired(e.id, now):
			return e.id, nil
		}
	}

	// One call per credential at a time, so a burst of requests with the same
	// credential is a single introspection.
	v, err, _ := a.group.Do(key, func() (any, error) {
		return a.introspect(ctx, cred)
	})
	if err == nil {
		id, _ := v.(*Identity)
		a.store(key, id)
		if id == nil {
			return nil, ErrUnauthenticated
		}
		return id, nil
	}

	// The endpoint could not answer. An accepted credential seen recently enough is
	// still honoured, within the stale_if_error window; anything else is "unavailable",
	// which is neither a pass nor a cached rejection.
	// The clock is read again: a call that timed out took its whole timeout, and the
	// window is measured from now, not from when the request began.
	now = a.cfg.Now()
	if e != nil && e.id != nil && a.cfg.StaleIfError > 0 &&
		now.Sub(e.fetched) < a.cfg.CacheTTL+a.cfg.StaleIfError && !expired(e.id, now) {
		apimetrics.AuthStaleServedTotal.WithLabelValues(a.Name()).Inc()
		klog.Warningf("authenticator %s unavailable, serving user %s from cache (age %s): %v",
			a.Name(), e.id.ID, now.Sub(e.fetched).Round(time.Second), err)
		return e.id, nil
	}
	return nil, &UnavailableError{Authenticator: a.Name(), Err: err}
}

func expired(id *Identity, now time.Time) bool { return !id.Expiry.IsZero() && now.After(id.Expiry) }

func cacheKey(c Credentials) string {
	h := sha256.New()
	// The client address is part of the key: the authenticator may apply address
	// policy (an API key's allow list), so one address's acceptance is not another's.
	// The user type is too: the same string is a different session under another type.
	for _, part := range []string{c.Bearer, c.Cookie, c.UserType, c.ClientIP} {
		h.Write([]byte(strconv.Itoa(len(part))))
		h.Write([]byte{':'})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (a *IntrospectionAuthenticator) store(key string, id *Identity) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.cache) >= maxCacheEntries {
		// Entries older than the longest time they can be used are dead weight.
		limit := a.cfg.Now().Add(-(a.cfg.CacheTTL + max(a.cfg.StaleIfError, 0)))
		for k, e := range a.cache {
			if e.fetched.Before(limit) {
				delete(a.cache, k)
			}
		}
		for k := range a.cache {
			if len(a.cache) < maxCacheEntries {
				break
			}
			delete(a.cache, k)
		}
	}
	a.cache[key] = &cacheEntry{id: id, fetched: a.cfg.Now()}
}

// introspectionAnswer is an RFC 7662 response with the extension members this
// authenticator reads. Active is a pointer so that an answer without it is told from "false".
type introspectionAnswer struct {
	Active     *bool       `json:"active"`
	Issuer     string      `json:"iss"`
	Sub        string      `json:"sub"`
	Username   string      `json:"username"`
	Name       string      `json:"name"`
	Email      string      `json:"email"`
	Groups     []string    `json:"groups"`
	Roles      []string    `json:"roles"`
	Scope      string      `json:"scope"`
	Jti        string      `json:"jti"`
	Exp        json.Number `json:"exp"`
	Restricted bool        `json:"restricted"`
	Act        *struct {
		ClientID string `json:"client_id"`
		Sub      string `json:"sub"`
	} `json:"act"`
}

// introspect asks the endpoint. It returns (nil, nil) for a definite "not active", and
// an error whenever the answer cannot be trusted as one.
func (a *IntrospectionAuthenticator) introspect(ctx context.Context, cred Credentials) (*Identity, error) {
	// The caller's context is cancelled when its request is; the call is shared by
	// every request waiting on it, so it has a lifetime of its own.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.cfg.Timeout)
	defer cancel()
	endpoint, err := a.introspectionURL(rctx)
	if err != nil {
		return nil, err
	}
	secret, err := readClientSecret(a.cfg.ClientSecretFile)
	if err != nil {
		return nil, fmt.Errorf("client secret: %w", err)
	}
	token := cred.Bearer
	if token == "" {
		token = cred.Cookie
	}
	form := url.Values{"token": {token}}
	if cred.UserType != "" {
		form.Set("user_type", cred.UserType)
	}
	if cred.ClientIP != "" {
		form.Set("client_ip", cred.ClientIP)
	}
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(a.cfg.ClientID, secret)
	resp, err := a.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIntrospectBody))
	if err != nil {
		return nil, err
	}
	// An introspection is answered with 200. Anything else, including 401 and 403 for
	// this apiserver's own credentials, means the introspection did not happen.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspection endpoint answered status %d", resp.StatusCode)
	}
	var out introspectionAnswer
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("introspection answer is not a JSON object: %w", err)
	}
	if out.Active == nil {
		return nil, errors.New("introspection answer has no active member")
	}
	if !*out.Active {
		return nil, nil
	}
	return a.identity(&out)
}

// introspectionURL returns the configured endpoint, or the one the issuer's discovery
// document names. A discovery that failed is retried on the next call.
func (a *IntrospectionAuthenticator) introspectionURL(ctx context.Context) (string, error) {
	if a.cfg.URL != "" {
		return a.cfg.URL, nil
	}
	a.epMu.Lock()
	defer a.epMu.Unlock()
	if a.endpoint != "" {
		return a.endpoint, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.Issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := a.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discovery answered status %d", resp.StatusCode)
	}
	var doc struct {
		Issuer     string `json:"issuer"`
		Introspect string `json:"introspection_endpoint"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxIntrospectBody)).Decode(&doc); err != nil {
		return "", fmt.Errorf("discovery document: %w", err)
	}
	if strings.TrimRight(doc.Issuer, "/") != a.cfg.Issuer {
		return "", errors.New("discovery document is for another issuer")
	}
	if doc.Introspect == "" {
		return "", errors.New("discovery document has no introspection_endpoint")
	}
	a.endpoint = doc.Introspect
	return a.endpoint, nil
}

func (a *IntrospectionAuthenticator) identity(r *introspectionAnswer) (*Identity, error) {
	if r.Sub == "" {
		return nil, errors.New("introspection accepted the credential without a sub")
	}
	if a.cfg.Issuer != "" && r.Issuer != "" && strings.TrimRight(r.Issuer, "/") != a.cfg.Issuer {
		// A token of another issuer is not one this authenticator may vouch for.
		klog.Warningf("authenticator %s: answer for issuer %q, expected %q; credential refused", a.Name(), r.Issuer, a.cfg.Issuer)
		return nil, nil
	}
	id := &Identity{
		ID: r.Sub, Name: r.Name, Email: r.Email, Jti: r.Jti, Restricted: r.Restricted,
		Groups: unionOf(r.Groups, r.Roles), Scopes: strings.Fields(r.Scope),
	}
	if id.Name == "" {
		id.Name = r.Username
	}
	if id.Name == "" {
		id.Name = id.ID
	}
	if id.Email == "" && strings.Contains(r.Username, "@") {
		id.Email = r.Username
	}
	if r.Act != nil {
		// A delegated credential. One that says so but not for whom must not pass as an
		// ordinary one: that would lift the restrictions on delegated credentials.
		id.Actor = r.Act.ClientID
		if id.Actor == "" {
			id.Actor = r.Act.Sub
		}
		if id.Actor == "" {
			return nil, errors.New("introspection answer has an act member without an actor")
		}
	}
	if r.Exp != "" {
		sec, err := parseUnix(r.Exp)
		if err != nil {
			return nil, errors.New("introspection answer has a malformed exp")
		}
		id.Expiry = time.Unix(sec, 0)
	}
	return id, nil
}

// parseUnix reads an exp that is an integer or, as some servers write it, a whole or fractional number.
func parseUnix(n json.Number) (int64, error) {
	if sec, err := n.Int64(); err == nil {
		return sec, nil
	}
	f, err := n.Float64()
	if err != nil {
		return 0, err
	}
	return int64(f), nil
}

func unionOf(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lists {
		for _, s := range l {
			if s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}
