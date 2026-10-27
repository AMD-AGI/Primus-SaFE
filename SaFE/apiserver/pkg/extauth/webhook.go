/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"bytes"
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
	tokenReviewAPIVersion = "authentication.k8s.io/v1"
	tokenReviewKind       = "TokenReview"

	defaultWebhookTimeout = 5 * time.Second
	defaultCacheTTL       = 30 * time.Second
	defaultNegativeTTL    = 5 * time.Second
	// DefaultStaleIfError is how long after its TTL a cached accepted credential
	// still counts while the authenticator is unreachable.
	DefaultStaleIfError = 30 * time.Minute

	maxCacheEntries  = 10000
	maxWebhookBody   = 1 << 20
	extraEmail       = "email"
	extraActor       = "actor"
	extraScopes      = "scopes"
	extraJti         = "jti"
	extraExpiry      = "exp"
	extraRestricted  = "restricted"
	extraDisplayName = "name"
)

// WebhookConfig configures a webhook TokenReview authenticator.
type WebhookConfig struct {
	Name string
	URL  string
	// CABundle is PEM text, or the path of a PEM file, that signs the webhook's certificate.
	CABundle string
	Timeout  time.Duration
	// CacheTTL is how long an accepted credential is used without asking again.
	CacheTTL time.Duration
	// CacheNegativeTTL is how long a rejected credential is refused without asking again.
	// Only an answer of "not authenticated" is cached this way, never an error.
	CacheNegativeTTL time.Duration
	// StaleIfError lets an accepted credential keep working for this long after
	// CacheTTL while the webhook is unreachable or failing. Zero uses
	// DefaultStaleIfError; a negative value turns it off.
	StaleIfError time.Duration

	HTTPClient *http.Client
	Now        func() time.Time
}

// WebhookAuthenticator asks an external service who a credential belongs to.
//
// The request is a TokenReview: spec.token carries the bearer token. Two
// non-standard spec fields carry what an opaque credential needs: "cookie" and
// "clientIP". The response is a TokenReview whose status.user gives uid (the SaFE
// user id), username (display name), groups, and extra. Recognised extra keys:
// email, name, actor, scopes, jti, exp (unix seconds), restricted.
type WebhookAuthenticator struct {
	cfg   WebhookConfig
	group singleflight.Group

	mu    sync.Mutex
	cache map[string]*cacheEntry
}

type cacheEntry struct {
	id      *Identity // nil: a cached rejection
	fetched time.Time
}

// NewWebhookAuthenticator builds an authenticator.
func NewWebhookAuthenticator(cfg WebhookConfig) (*WebhookAuthenticator, error) {
	if cfg.URL == "" {
		return nil, errors.New("webhook authenticator: url is required")
	}
	if cfg.Name == "" {
		cfg.Name = "webhook"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultWebhookTimeout
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
	if cfg.HTTPClient == nil {
		tr := &http.Transport{Proxy: nil}
		if cfg.CABundle != "" {
			pool, err := certPool(cfg.CABundle)
			if err != nil {
				return nil, fmt.Errorf("webhook authenticator: ca_bundle: %w", err)
			}
			tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		}
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout, Transport: tr}
	}
	return &WebhookAuthenticator{cfg: cfg, cache: map[string]*cacheEntry{}}, nil
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

// Name implements Authenticator.
func (w *WebhookAuthenticator) Name() string { return w.cfg.Name }

// Authenticate implements Authenticator.
func (w *WebhookAuthenticator) Authenticate(ctx context.Context, cred Credentials) (*Identity, error) {
	if cred.Empty() {
		return nil, ErrNotApplicable
	}
	key := cacheKey(cred)
	now := w.cfg.Now()

	w.mu.Lock()
	e := w.cache[key]
	w.mu.Unlock()
	if e != nil {
		age := now.Sub(e.fetched)
		switch {
		case e.id == nil && age < w.cfg.CacheNegativeTTL:
			return nil, ErrUnauthenticated
		case e.id != nil && age < w.cfg.CacheTTL && !expired(e.id, now):
			return e.id, nil
		}
	}

	// One call per credential at a time, so a burst of requests with the same
	// credential is a single review.
	v, err, _ := w.group.Do(key, func() (any, error) {
		return w.review(ctx, cred)
	})
	if err == nil {
		id, _ := v.(*Identity)
		w.store(key, id)
		if id == nil {
			return nil, ErrUnauthenticated
		}
		return id, nil
	}

	// The webhook could not answer. An accepted credential seen recently enough is
	// still honoured, within the stale_if_error window; anything else is "unavailable",
	// which is neither a pass nor a cached rejection.
	if e != nil && e.id != nil && w.cfg.StaleIfError > 0 &&
		now.Sub(e.fetched) < w.cfg.CacheTTL+w.cfg.StaleIfError && !expired(e.id, now) {
		apimetrics.AuthStaleServedTotal.WithLabelValues(w.Name()).Inc()
		klog.Warningf("authenticator %s unavailable, serving user %s from cache (age %s): %v",
			w.Name(), e.id.ID, now.Sub(e.fetched).Round(time.Second), err)
		return e.id, nil
	}
	return nil, &UnavailableError{Authenticator: w.Name(), Err: err}
}

func expired(id *Identity, now time.Time) bool { return !id.Expiry.IsZero() && now.After(id.Expiry) }

func cacheKey(c Credentials) string {
	h := sha256.New()
	for _, part := range []string{c.Bearer, c.Cookie, c.UserType} {
		h.Write([]byte(strconv.Itoa(len(part))))
		h.Write([]byte{':'})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (w *WebhookAuthenticator) store(key string, id *Identity) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.cache) >= maxCacheEntries {
		// Entries older than the longest time they can be used are dead weight.
		limit := w.cfg.Now().Add(-(w.cfg.CacheTTL + max(w.cfg.StaleIfError, 0)))
		for k, e := range w.cache {
			if e.fetched.Before(limit) {
				delete(w.cache, k)
			}
		}
		for k := range w.cache {
			if len(w.cache) < maxCacheEntries {
				break
			}
			delete(w.cache, k)
		}
	}
	w.cache[key] = &cacheEntry{id: id, fetched: w.cfg.Now()}
}

type tokenReview struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Spec       struct {
		Token    string `json:"token,omitempty"`
		Cookie   string `json:"cookie,omitempty"`
		ClientIP string `json:"clientIP,omitempty"`
		UserType string `json:"userType,omitempty"`
	} `json:"spec"`
	Status struct {
		Authenticated bool   `json:"authenticated"`
		Error         string `json:"error,omitempty"`
		User          struct {
			Username string              `json:"username"`
			UID      string              `json:"uid"`
			Groups   []string            `json:"groups"`
			Extra    map[string][]string `json:"extra"`
		} `json:"user"`
	} `json:"status"`
}

// review asks the webhook. It returns (nil, nil) for a definite "not authenticated",
// and an error whenever the answer cannot be trusted as one.
func (w *WebhookAuthenticator) review(ctx context.Context, cred Credentials) (*Identity, error) {
	var in tokenReview
	in.APIVersion, in.Kind = tokenReviewAPIVersion, tokenReviewKind
	in.Spec.Token, in.Spec.Cookie, in.Spec.ClientIP, in.Spec.UserType = cred.Bearer, cred.Cookie, cred.ClientIP, cred.UserType
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	// The caller's context is cancelled when its request is; the review is shared by
	// every request waiting on it, so it has a lifetime of its own.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, w.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := w.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxWebhookBody))
	if err != nil {
		return nil, err
	}
	// A TokenReview is answered with 200 (or 201). Anything else, including 401 and
	// 403 from the webhook itself, means the review did not happen.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("webhook answered status %d", resp.StatusCode)
	}
	var out tokenReview
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("webhook answer is not a TokenReview: %w", err)
	}
	if out.Kind != tokenReviewKind {
		return nil, errors.New("webhook answer is not a TokenReview")
	}
	if !out.Status.Authenticated {
		return nil, nil
	}
	return w.identity(&out)
}

func (w *WebhookAuthenticator) identity(r *tokenReview) (*Identity, error) {
	u := r.Status.User
	if u.UID == "" {
		return nil, errors.New("webhook accepted the credential without a uid")
	}
	first := func(k string) string {
		if v := u.Extra[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	id := &Identity{
		ID: u.UID, Name: u.Username, Groups: u.Groups,
		Email: first(extraEmail), Actor: first(extraActor), Jti: first(extraJti),
		Scopes: u.Extra[extraScopes],
	}
	if n := first(extraDisplayName); n != "" {
		id.Name = n
	}
	if id.Name == "" {
		id.Name = id.ID
	}
	id.Restricted = first(extraRestricted) == "true"
	if e := first(extraExpiry); e != "" {
		sec, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return nil, errors.New("webhook answer has a malformed exp")
		}
		id.Expiry = time.Unix(sec, 0)
	}
	if len(id.Scopes) == 1 && strings.Contains(id.Scopes[0], " ") {
		id.Scopes = strings.Fields(id.Scopes[0])
	}
	return id, nil
}
