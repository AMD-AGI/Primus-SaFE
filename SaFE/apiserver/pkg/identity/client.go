/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package identity calls the identity service for SCIM registration and for
// freeze forwarding.
package identity

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// scimCertDir is the mounted ingress TLS secret. install.sh writes it under the fixed name
// global.tls_secret ("default"). A missing file registers the SCIM target without a certificate.
var scimCertDir = "/etc/secrets/scim-tls"

const (
	// DeactivatedAnnotation marks a user frozen by a SCIM patch that set active to false.
	// A later patch sets the user active only while this annotation is present, and then removes it.
	DeactivatedAnnotation = "primus-safe.scim.deactivated"

	requestTimeout = 10 * time.Second
)

// ConfigError is a registration or call failure that a retry will not change
// until the configuration changes.
type ConfigError struct {
	Msg string
}

// Error returns the configuration message.
func (e *ConfigError) Error() string { return e.Msg }

type scimTarget struct {
	URL         string `json:"url"`
	BearerToken string `json:"bearerToken"`
	Certificate string `json:"certificate,omitempty"`
}

type statusBody struct {
	Status string `json:"status"`
}

// SetStatus sets the identity status to "active" or "disabled". The caller's bearer
// is forwarded. A refusal is 502 and an unreachable service is 503.
func SetStatus(ctx context.Context, userID, status, bearer string) error {
	if bearer == "" {
		return badGateway("the request has no credential to forward to the identity service")
	}
	endpoint, err := userURL(userID, "status")
	if err != nil {
		return mapCall("updating the user status", 0, err)
	}
	code, err := do(ctx, http.MethodPut, endpoint, statusBody{Status: status}, bearerAuth(bearer))
	if err == nil && code >= 200 && code < 300 {
		return nil
	}
	return mapCall("updating the user status", code, err)
}

// RegisterTarget upserts the SCIM target for this apiserver's identity client.
// The request uses HTTP Basic with the introspection client id and secret.
func RegisterTarget(ctx context.Context, publicURL, token string) error {
	publicURL, err := normalizePublic(publicURL)
	if err != nil {
		return err
	}
	base, err := ResolveBase()
	if err != nil {
		return err
	}
	clientID, secret, err := introspectionSecret()
	if err != nil {
		return err
	}
	endpoint, err := url.JoinPath(base, "oauth/scim-target")
	if err != nil {
		return &ConfigError{Msg: "cannot build the scim target URL"}
	}
	cert, err := targetCertificate()
	if err != nil {
		return err
	}
	code, err := do(ctx, http.MethodPut, endpoint, scimTarget{URL: publicURL, BearerToken: token, Certificate: cert}, func(r *http.Request) {
		r.SetBasicAuth(clientID, secret)
	})
	if err != nil {
		return err
	}
	if code >= 200 && code < 300 {
		return nil
	}
	return fmt.Errorf("identity service answered %d to scim target registration", code)
}

// ResolveBase returns the identity service root. auth.scim.identity_url wins.
// Otherwise the root is the first introspection authenticator's URL or issuer
// with a /oauth/... suffix removed.
func ResolveBase() (string, error) {
	if raw := commonconfig.GetSCIMIdentityURL(); raw != "" {
		return normalizeBase(raw)
	}
	cfgs, err := commonconfig.GetAuthenticators()
	if err != nil {
		return "", &ConfigError{Msg: "cannot read auth.authenticators"}
	}
	var last *ConfigError
	for _, c := range cfgs {
		if !strings.EqualFold(c.Type, "introspection") {
			continue
		}
		raw := strings.TrimSpace(c.URL)
		if raw == "" {
			raw = strings.TrimSpace(c.Issuer)
		}
		if raw == "" {
			continue
		}
		base, derr := deriveBase(raw)
		if derr != nil {
			var cfg *ConfigError
			if errors.As(derr, &cfg) {
				last = cfg
			}
			continue
		}
		return base, nil
	}
	if last != nil {
		return "", last
	}
	return "", &ConfigError{Msg: "identity base URL is not configured; set auth.scim.identity_url"}
}

// deriveBase removes a /oauth/ path from an introspection URL or issuer.
func deriveBase(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !absoluteHTTP(u) {
		return "", &ConfigError{Msg: fmt.Sprintf("cannot derive the identity base from %q; set auth.scim.identity_url", raw)}
	}
	p := u.Path
	if i := strings.Index(p, "/oauth/"); i >= 0 {
		p = p[:i]
	} else if strings.Trim(p, "/") != "" {
		return "", &ConfigError{Msg: fmt.Sprintf("cannot derive the identity base from %q; set auth.scim.identity_url", raw)}
	} else {
		p = ""
	}
	u.Path = strings.TrimRight(p, "/")
	u.RawQuery, u.Fragment = "", ""
	u.User = nil
	return strings.TrimRight(u.String(), "/"), nil
}

// normalizeBase checks auth.scim.identity_url and drops a trailing slash.
func normalizeBase(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !absoluteHTTP(u) {
		return "", &ConfigError{Msg: "auth.scim.identity_url must be an absolute http(s) URL"}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	u.User = nil
	return strings.TrimRight(u.String(), "/"), nil
}

// targetCertificate reads tls.crt from the mounted ingress TLS secret, and ca.crt when it is present.
// A missing tls.crt registers the target without a certificate.
func targetCertificate() (string, error) {
	crt, err := os.ReadFile(filepath.Join(scimCertDir, "tls.crt"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(crt)) == "" {
		return "", nil
	}
	var b strings.Builder
	b.Write(crt)
	if !strings.HasSuffix(string(crt), "\n") {
		b.WriteByte('\n')
	}
	ca, err := os.ReadFile(filepath.Join(scimCertDir, "ca.crt"))
	if os.IsNotExist(err) {
		return b.String(), nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(ca)) != "" {
		b.Write(ca)
	}
	return b.String(), nil
}

// normalizePublic checks auth.scim.public_url and drops a trailing slash.
func normalizePublic(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &ConfigError{Msg: "auth.scim.public_url is empty"}
	}
	u, err := url.Parse(raw)
	if err != nil || !absoluteHTTP(u) {
		return "", &ConfigError{Msg: "auth.scim.public_url must be an absolute http(s) URL"}
	}
	return strings.TrimRight(raw, "/"), nil
}

// absoluteHTTP reports whether u is an http or https URL with a host.
func absoluteHTTP(u *url.URL) bool {
	return u != nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

// userURL joins the identity base with /api/v1/identities/{id} and any extra segments.
func userURL(userID string, extra ...string) (string, error) {
	base, err := ResolveBase()
	if err != nil {
		return "", err
	}
	parts := append([]string{base, "api/v1/identities", userID}, extra...)
	return url.JoinPath(parts[0], parts[1:]...)
}

// introspectionSecret reads the client id and secret of the first introspection authenticator.
func introspectionSecret() (string, string, error) {
	cfgs, err := commonconfig.GetAuthenticators()
	if err != nil {
		return "", "", &ConfigError{Msg: "cannot read auth.authenticators"}
	}
	for _, c := range cfgs {
		if !strings.EqualFold(c.Type, "introspection") || (c.ClientID == "" && c.ClientIDFile == "") || c.ClientSecretFile == "" {
			continue
		}
		id, rerr := extauth.ResolveClientID(c.ClientID, c.ClientIDFile)
		if rerr != nil {
			return "", "", rerr
		}
		secret, rerr := extauth.ReadClientSecret(c.ClientSecretFile)
		if rerr != nil {
			return "", "", rerr
		}
		return id, secret, nil
	}
	return "", "", &ConfigError{Msg: "no introspection client_id and client_secret_file are configured"}
}

// bearerAuth sets Authorization to the forwarded bearer token.
func bearerAuth(token string) func(*http.Request) {
	return func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	}
}

// do sends one JSON request. setAuth sets Authorization. A non-nil error is a transport failure.
func do(ctx context.Context, method, endpoint string, body any, setAuth func(*http.Request)) (int, error) {
	cli, err := httpClient()
	if err != nil {
		return 0, err
	}
	var reader io.Reader
	if body != nil {
		data, merr := json.Marshal(body)
		if merr != nil {
			return 0, merr
		}
		reader = bytes.NewReader(data)
	}
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, endpoint, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if setAuth != nil {
		setAuth(req)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}

var (
	httpClientsMu sync.Mutex
	httpClients   = map[string]*http.Client{}
)

// httpClient returns the shared client for the introspection authenticator's CA bundle.
// One client is kept per bundle, so connections are reused. An empty bundle uses the default roots.
func httpClient() (*http.Client, error) {
	key, pool, err := clientCA()
	if err != nil {
		return nil, err
	}
	httpClientsMu.Lock()
	defer httpClientsMu.Unlock()
	if c := httpClients[key]; c != nil {
		return c, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if pool != nil {
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	c := &http.Client{
		Timeout:   requestTimeout + 2*time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	httpClients[key] = c
	return c, nil
}

// clientCA returns the cache key and certificate pool of the first introspection authenticator
// that sets ca_bundle. An empty key and a nil pool mean the default roots.
func clientCA() (string, *x509.CertPool, error) {
	cfgs, err := commonconfig.GetAuthenticators()
	if err != nil {
		return "", nil, &ConfigError{Msg: "cannot read auth.authenticators"}
	}
	for _, c := range cfgs {
		bundle := strings.TrimSpace(c.CABundle)
		if !strings.EqualFold(c.Type, "introspection") || bundle == "" {
			continue
		}
		pool, err := extauth.CertPool(bundle)
		if err != nil {
			return "", nil, err
		}
		return bundle, pool, nil
	}
	return "", nil, nil
}

// mapCall turns a transport or HTTP failure into 503 or 502.
func mapCall(op string, status int, err error) error {
	if err != nil {
		var cfg *ConfigError
		if errors.As(err, &cfg) {
			return apierrors.NewServiceUnavailable(cfg.Error())
		}
		return apierrors.NewServiceUnavailable("identity service is unavailable: " + err.Error())
	}
	if status >= 500 {
		return apierrors.NewServiceUnavailable(fmt.Sprintf("identity service is unavailable (%s returned %d)", op, status))
	}
	return badGateway(fmt.Sprintf("identity service refused %s (%d)", op, status))
}

// badGateway is the 502 returned when the identity service refuses the call.
func badGateway(msg string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusBadGateway,
		Reason:  metav1.StatusReason("BadGateway"),
		Message: msg,
	}}
}
