/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	// sandboxDetailKind is the workload kind the identity assertion tests use.
	sandboxDetailKind = "Sandbox"
	// workspaceResource is the resource of the workspace membership check.
	workspaceResource = "workspaces"
	// workspaceUseVerb is the verb of the workspace membership check.
	workspaceUseVerb = "use"
	// jwksTTL is how long a fetched key set is kept. A key the set does not contain is fetched again at once.
	jwksTTL = 5 * time.Minute
	// jwksBodyLimit caps the key set document.
	jwksBodyLimit = 1 << 16
)

// errUnknownKID is returned when the assertion names a key the current set does not contain.
var errUnknownKID = errors.New("signature key is not in the key set")

// cachedJWKS is the last key set fetched for one URL.
type cachedJWKS struct {
	url string
	set jose.JSONWebKeySet
	at  time.Time
}

var (
	jwksMu     sync.Mutex
	jwksCached cachedJWKS
	jwksClient = &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || len(via) >= 2 {
				return fmt.Errorf("identity jwks url must be https")
			}
			return nil
		},
	}
)

// identityClaims is the part of the identity assertion the admission check reads.
type identityClaims struct {
	jwt.Claims
	Act struct {
		Sub string `json:"sub"`
	} `json:"act"`
	Groups               []string         `json:"groups"`
	AuthorizationDetails []map[string]any `json:"authorization_details"`
}

// validateSandboxIdentity verifies the identity assertion of a Workload created by the sandbox control plane
// and checks that the asserted user may use the Workload's workspace. Other requesters pass unchecked.
func (v *WorkloadValidator) validateSandboxIdentity(ctx context.Context, req admission.Request,
	workload, oldWorkload *v1.Workload) error {
	if !commonconfig.IsSandboxAdmissionEnable() {
		return nil
	}
	cfg := commonconfig.GetSandboxAdmissionConfig()
	if cfg.Creator == "" || req.UserInfo.Username != cfg.Creator {
		return nil
	}
	if cfg.Annotation == "" {
		return commonerrors.NewForbidden("sandbox admission annotation is not configured")
	}
	token := v1.GetAnnotation(workload, cfg.Annotation)
	if token == "" {
		return commonerrors.NewForbidden(fmt.Sprintf("annotation %s is required", cfg.Annotation))
	}
	oldToken := ""
	if oldWorkload != nil {
		oldToken = v1.GetAnnotation(oldWorkload, cfg.Annotation)
	}
	claims, err := verifyIdentityAssertion(ctx, cfg, token, token == oldToken, workload)
	if err != nil {
		return commonerrors.NewForbidden(fmt.Sprintf("identity assertion is invalid: %v", err))
	}
	return v.checkWorkspaceAccess(ctx, claims, workload.Spec.Workspace)
}

// verifyIdentityAssertion checks the assertion signature, header, standard claims, subject, actor and the
// authorization detail against the Workload. A token that an UPDATE keeps is checked one second before its
// expiry, so a later update is not refused because the token has since expired. A key id the cached set
// does not contain is fetched once more, so a key rotation is accepted before the cache expires.
func verifyIdentityAssertion(ctx context.Context, cfg commonconfig.SandboxAdmissionConfig, token string, unchanged bool,
	workload *v1.Workload) (*identityClaims, error) {
	if cfg.Issuer == "" || cfg.Actor == "" || (strings.TrimSpace(cfg.JWKS) == "" && cfg.JWKSURL == "") {
		return nil, fmt.Errorf("identity verification is not configured")
	}
	keys, fromURL, err := admissionKeys(ctx, cfg, false)
	if err != nil {
		return nil, err
	}
	claims, err := checkAssertion(keys, cfg, token, unchanged, workload)
	if err == nil || !fromURL || !errors.Is(err, errUnknownKID) {
		return claims, err
	}
	keys, _, err = admissionKeys(ctx, cfg, true)
	if err != nil {
		return nil, err
	}
	return checkAssertion(keys, cfg, token, unchanged, workload)
}

// admissionKeys returns the key set cfg names. Inline JSON is used as given. A URL is fetched and kept
// for jwksTTL; force fetches again. fromURL reports that a missing key id can be retried against the URL.
func admissionKeys(ctx context.Context, cfg commonconfig.SandboxAdmissionConfig, force bool) (jose.JSONWebKeySet, bool, error) {
	if strings.TrimSpace(cfg.JWKS) != "" {
		var keys jose.JSONWebKeySet
		if err := json.Unmarshal([]byte(cfg.JWKS), &keys); err != nil || len(keys.Keys) == 0 {
			return jose.JSONWebKeySet{}, false, fmt.Errorf("identity jwks is not readable")
		}
		return keys, false, nil
	}
	if !force {
		jwksMu.Lock()
		fresh := jwksCached.url == cfg.JWKSURL && time.Since(jwksCached.at) < jwksTTL && len(jwksCached.set.Keys) > 0
		set := jwksCached.set
		jwksMu.Unlock()
		if fresh {
			return set, true, nil
		}
	}
	set, err := fetchJWKS(ctx, cfg.JWKSURL, cfg.CAFile)
	if err != nil {
		jwksMu.Lock()
		defer jwksMu.Unlock()
		if jwksCached.url == cfg.JWKSURL && len(jwksCached.set.Keys) > 0 {
			return jwksCached.set, true, nil
		}
		return jose.JSONWebKeySet{}, true, err
	}
	jwksMu.Lock()
	jwksCached = cachedJWKS{url: cfg.JWKSURL, set: set, at: time.Now()}
	jwksMu.Unlock()
	return set, true, nil
}

// fetchJWKS reads the key set at raw. Only https is accepted, and the body is capped.
// caFile, when set, is a PEM bundle added to the system trust store. Verification stays on.
func fetchJWKS(ctx context.Context, raw, caFile string) (jose.JSONWebKeySet, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("identity jwks url must be https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	client, err := jwksHTTPClient(caFile)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("identity jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return jose.JSONWebKeySet{}, fmt.Errorf("identity jwks: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, jwksBodyLimit))
	if err != nil {
		return jose.JSONWebKeySet{}, fmt.Errorf("identity jwks: %w", err)
	}
	var keys jose.JSONWebKeySet
	if err = json.Unmarshal(body, &keys); err != nil || len(keys.Keys) == 0 {
		return jose.JSONWebKeySet{}, fmt.Errorf("identity jwks is not readable")
	}
	return keys, nil
}

// checkAssertion checks one assertion against keys.
func checkAssertion(keys jose.JSONWebKeySet, cfg commonconfig.SandboxAdmissionConfig, token string, unchanged bool,
	workload *v1.Workload) (*identityClaims, error) {
	tok, err := jwt.ParseSigned(token, []jose.SignatureAlgorithm{jose.SignatureAlgorithm(cfg.Algorithm)})
	if err != nil {
		return nil, fmt.Errorf("token is not a signed JWT with algorithm %s", cfg.Algorithm)
	}
	header := tok.Headers[0]
	if typ, _ := header.ExtraHeaders[jose.HeaderType].(string); typ != cfg.TokenType {
		return nil, fmt.Errorf("token type is not %s", cfg.TokenType)
	}
	if header.KeyID != "" && len(keys.Key(header.KeyID)) == 0 {
		return nil, errUnknownKID
	}
	candidates := keys.Keys
	if header.KeyID != "" {
		candidates = keys.Key(header.KeyID)
	}
	var claims identityClaims
	verified := false
	for _, key := range candidates {
		claims = identityClaims{}
		if tok.Claims(key, &claims) == nil {
			verified = true
			break
		}
	}
	if !verified {
		return nil, fmt.Errorf("signature does not verify")
	}
	if claims.Expiry == nil {
		return nil, fmt.Errorf("exp is missing")
	}
	now := time.Now()
	if unchanged {
		now = claims.Expiry.Time().Add(-time.Second)
	}
	// iat is not part of the check, so a small clock difference does not refuse a fresh token.
	claims.IssuedAt = nil
	expected := jwt.Expected{Issuer: cfg.Issuer, AnyAudience: []string{cfg.Audience}, Time: now}
	if err = claims.ValidateWithLeeway(expected, 0); err != nil {
		return nil, fmt.Errorf("claims are not valid: %v", err)
	}
	if claims.Subject == "" || claims.Subject != v1.GetUserId(workload) {
		return nil, fmt.Errorf("sub does not match label %s", v1.UserIdLabel)
	}
	if claims.Act.Sub != cfg.Actor {
		return nil, fmt.Errorf("act.sub is not the sandbox control plane")
	}
	gpus, err := workloadGpuCount(workload)
	if err != nil {
		return nil, err
	}
	if !hasMatchingDetail(claims.AuthorizationDetails, cfg, workload, gpus) {
		return nil, fmt.Errorf("no authorization detail of type %s matches the workload", cfg.DetailType)
	}
	return &claims, nil
}

// hasMatchingDetail reports whether one authorization detail of the configured type names this Workload.
func hasMatchingDetail(details []map[string]any, cfg commonconfig.SandboxAdmissionConfig, workload *v1.Workload, gpus float64) bool {
	for _, d := range details {
		got, ok := d["gpus"].(float64)
		if d["type"] == cfg.DetailType && d["workspace"] == workload.Spec.Workspace &&
			d["kind"] == workload.SpecKind() && (cfg.Kind == "" || d["kind"] == cfg.Kind) &&
			d["name"] == workload.Name && ok && got == gpus {
			return true
		}
	}
	return false
}

// jwksHTTPClient returns the shared client, or one that also trusts caFile.
func jwksHTTPClient(caFile string) (*http.Client, error) {
	if strings.TrimSpace(caFile) == "" {
		return jwksClient, nil
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("identity jwks ca: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("identity jwks ca is not readable")
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("identity jwks transport is not available")
	}
	transport := base.Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	return &http.Client{Timeout: jwksClient.Timeout, Transport: transport, CheckRedirect: jwksClient.CheckRedirect}, nil
}

// workloadGpuCount sums the gpu count times the replica over the Workload resources; an unset replica counts as one.
func workloadGpuCount(workload *v1.Workload) (float64, error) {
	total := 0.0
	for _, r := range workload.Spec.Resources {
		if r.GPU == "" {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(r.GPU), 64)
		if err != nil {
			return 0, fmt.Errorf("workload gpu %q is not a number", r.GPU)
		}
		replica := r.Replica
		if replica == 0 {
			replica = 1
		}
		total += n * float64(replica)
	}
	return total, nil
}

// checkWorkspaceAccess asks the API server whether the asserted user, with the asserted groups, may use the
// workspace. A denial is a forbidden error; a failed review is an internal error the caller may retry.
func (v *WorkloadValidator) checkWorkspaceAccess(ctx context.Context, claims *identityClaims, workspace string) error {
	prefix := commonconfig.GetRBACSubjectPrefix()
	groups := make([]string, 0, len(claims.Groups))
	for _, g := range claims.Groups {
		groups = append(groups, prefix+g)
	}
	review := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   prefix + claims.Subject,
			Groups: groups,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Group:    v1.SchemeGroupVersion.Group,
				Resource: workspaceResource,
				Name:     workspace,
				Verb:     workspaceUseVerb,
			},
		},
	}
	if err := v.Client.Create(ctx, review); err != nil {
		return commonerrors.NewInternalError(fmt.Sprintf(
			"workspace membership of user %q could not be checked, retry later: %v", claims.Subject, err))
	}
	if !review.Status.Allowed {
		return commonerrors.NewForbidden(fmt.Sprintf("user %q is not a member of workspace %q", claims.Subject, workspace))
	}
	return nil
}
