/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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
	// identityAttestationAnnotation carries the identity assertion the sandbox control plane attaches.
	identityAttestationAnnotation = "agent-sandbox.io/identity-attestation"
	// sandboxDetailKind is the workload kind an authorization detail must name.
	sandboxDetailKind = "Sandbox"
	// workspaceResource is the resource of the workspace membership check.
	workspaceResource = "workspaces"
	// workspaceUseVerb is the verb of the workspace membership check.
	workspaceUseVerb = "use"
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
	token := v1.GetAnnotation(workload, identityAttestationAnnotation)
	if token == "" {
		return commonerrors.NewForbidden(fmt.Sprintf("annotation %s is required", identityAttestationAnnotation))
	}
	oldToken := ""
	if oldWorkload != nil {
		oldToken = v1.GetAnnotation(oldWorkload, identityAttestationAnnotation)
	}
	claims, err := verifyIdentityAssertion(cfg, token, token == oldToken, workload)
	if err != nil {
		return commonerrors.NewForbidden(fmt.Sprintf("identity assertion is invalid: %v", err))
	}
	return v.checkWorkspaceAccess(ctx, claims, workload.Spec.Workspace)
}

// verifyIdentityAssertion checks the assertion signature, header, standard claims, subject, actor and the
// authorization detail against the Workload. A token that an UPDATE keeps is checked one second before its
// expiry, so a later update is not refused because the token has since expired.
func verifyIdentityAssertion(cfg commonconfig.SandboxAdmissionConfig, token string, unchanged bool,
	workload *v1.Workload) (*identityClaims, error) {
	if cfg.JWKS == "" || cfg.Issuer == "" || cfg.Actor == "" {
		return nil, fmt.Errorf("identity verification is not configured")
	}
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal([]byte(cfg.JWKS), &keys); err != nil {
		return nil, fmt.Errorf("identity jwks is not readable")
	}
	tok, err := jwt.ParseSigned(token, []jose.SignatureAlgorithm{jose.SignatureAlgorithm(cfg.Algorithm)})
	if err != nil {
		return nil, fmt.Errorf("token is not a signed JWT with algorithm %s", cfg.Algorithm)
	}
	header := tok.Headers[0]
	if typ, _ := header.ExtraHeaders[jose.HeaderType].(string); typ != cfg.TokenType {
		return nil, fmt.Errorf("token type is not %s", cfg.TokenType)
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
	if !hasMatchingDetail(claims.AuthorizationDetails, cfg.DetailType, workload, gpus) {
		return nil, fmt.Errorf("no authorization detail of type %s matches the workload", cfg.DetailType)
	}
	return &claims, nil
}

// hasMatchingDetail reports whether one authorization detail of the given type names this Workload.
func hasMatchingDetail(details []map[string]any, detailType string, workload *v1.Workload, gpus float64) bool {
	for _, d := range details {
		got, ok := d["gpus"].(float64)
		if d["type"] == detailType && d["workspace"] == workload.Spec.Workspace &&
			d["kind"] == sandboxDetailKind && d["kind"] == workload.SpecKind() &&
			d["name"] == workload.Name && ok && got == gpus {
			return true
		}
	}
	return false
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
