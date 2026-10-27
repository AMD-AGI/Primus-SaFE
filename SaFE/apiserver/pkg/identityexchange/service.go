/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

const (
	GrantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
	TokenTypeAccessToken   = "urn:ietf:params:oauth:token-type:access_token"
	// SubjectTokenTypeUserRef is the custom subject token type: the service
	// client names the user it acts for. Its value is "external:<issuer>|<subject>",
	// "email:<address>" or "safe_user_id:<id>". It is impersonation, so every
	// form still requires a binding the user confirmed.
	SubjectTokenTypeUserRef = "urn:primus:params:oauth:token-type:user-ref"

	AuthMethodSecretBasic = "client_secret_basic"
	AuthMethodPrivateKey  = "private_key_jwt" // reserved, not implemented yet

	ScopeAPI  = "safe.api"
	ScopeRead = "safe.read"

	// UserType is the user type recorded for requests authenticated with an exchanged token.
	UserType = "exchange"

	DefaultTTL = 10 * time.Minute
	MaxTTL     = 15 * time.Minute
	// ClientIdPrefix prefixes generated client ids.
	ClientIdPrefix = "svc-"

	maxAuditField = 256
)

// OAuth error codes (RFC 6749 section 5.2, RFC 8693 section 2.2.2, RFC 8707).
const (
	ErrInvalidRequest     = "invalid_request"
	ErrInvalidClient      = "invalid_client"
	ErrInvalidGrant       = "invalid_grant"
	ErrUnsupportedGrant   = "unsupported_grant_type"
	ErrInvalidScope       = "invalid_scope"
	ErrInvalidTarget      = "invalid_target"
	ErrServerError        = "server_error"
	ErrAccessDeniedClient = "access_denied"
)

// OAuthError is the error body of the token endpoint.
type OAuthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description,omitempty"`
	Status      int    `json:"-"`
	// reason is the internal detail, recorded in the audit log but never sent to the caller.
	reason string
}

func (e *OAuthError) Error() string { return e.Code + ": " + e.Description }

// Config configures a Service.
type Config struct {
	// Issuer is the issuer URL placed in tokens and the discovery document.
	Issuer string
	// Audience identifies this apiserver; tokens for it carry it in "aud".
	Audience string
	// Now overrides the clock in tests.
	Now func() time.Time
}

// Service implements the token exchange and the validation of exchanged tokens.
type Service struct {
	cfg   Config
	keys  *KeySet
	store Store
	users UserStore
	// adminCheck authorizes service client administration (system administrators).
	adminCheck func(c *gin.Context) error
}

// NewService builds a Service.
func NewService(cfg Config, keys *KeySet, store Store, users UserStore) (*Service, error) {
	if cfg.Issuer == "" || cfg.Audience == "" {
		return nil, errors.New("issuer and audience are required")
	}
	if keys == nil || store == nil || users == nil {
		return nil, errors.New("keys, store and users are required")
	}
	cfg.Issuer = strings.TrimRight(cfg.Issuer, "/")
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg, keys: keys, store: store, users: users}, nil
}

// Audience returns the audience that identifies this apiserver.
func (s *Service) Audience() string { return s.cfg.Audience }

// ExchangeRequest is a parsed token endpoint request.
type ExchangeRequest struct {
	ClientId           string
	ClientSecret       string
	ClientIP           string
	GrantType          string
	SubjectToken       string
	SubjectTokenType   string
	Audience           string
	Scope              string
	RequestedTokenType string
}

// TokenResponse is the RFC 8693 section 2.2.1 response.
type TokenResponse struct {
	AccessToken     string `json:"access_token"`
	IssuedTokenType string `json:"issued_token_type"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int64  `json:"expires_in"`
	Scope           string `json:"scope,omitempty"`
}

// HashClientSecret returns the stored form of a client secret. Secrets are 32
// random bytes, so an unsalted SHA-256 is sufficient and lets the lookup stay
// a constant-time comparison.
func HashClientSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// GenerateClientCredentials returns a new client id and secret.
func GenerateClientCredentials() (clientId, secret string, err error) {
	b := make([]byte, 8+32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	return ClientIdPrefix + hex.EncodeToString(b[:8]), hex.EncodeToString(b[8:]), nil
}

// Exchange performs the token exchange. Every outcome is audited. Once the
// client has authenticated, every failure about the subject is the same
// invalid_grant, so the response does not reveal whether a user exists or is bound.
func (s *Service) Exchange(ctx context.Context, req ExchangeRequest) (*TokenResponse, *OAuthError) {
	var userId, jti string
	resp, oerr := s.exchange(ctx, req, &userId, &jti)
	s.auditExchange(ctx, req, userId, jti, oerr)
	return resp, oerr
}

func (s *Service) exchange(ctx context.Context, req ExchangeRequest, userId, jti *string) (*TokenResponse, *OAuthError) {
	if req.GrantType != GrantTypeTokenExchange {
		return nil, &OAuthError{Code: ErrUnsupportedGrant, Status: 400, reason: "grant_type"}
	}
	client, oerr := s.authenticateClient(ctx, req)
	if oerr != nil {
		return nil, oerr
	}
	if req.SubjectTokenType != SubjectTokenTypeUserRef {
		return nil, &OAuthError{Code: ErrInvalidRequest, Description: "unsupported subject_token_type", Status: 400, reason: "subject_token_type"}
	}
	if req.SubjectToken == "" {
		return nil, &OAuthError{Code: ErrInvalidRequest, Description: "subject_token is required", Status: 400, reason: "subject_token"}
	}
	if req.RequestedTokenType != "" && req.RequestedTokenType != TokenTypeAccessToken {
		return nil, &OAuthError{Code: ErrInvalidRequest, Description: "unsupported requested_token_type", Status: 400, reason: "requested_token_type"}
	}
	audiences := jsonList(client.AllowedAudiences)
	if req.Audience == "" || !contains(audiences, req.Audience) {
		return nil, &OAuthError{Code: ErrInvalidTarget, Description: "audience not allowed for this client", Status: 400, reason: "audience"}
	}
	scopes, ok := s.grantedScopes(client, req.Scope)
	if !ok {
		return nil, &OAuthError{Code: ErrInvalidScope, Status: 400, reason: "scope"}
	}

	id, reason := s.resolveSubject(ctx, client.ClientId, req.SubjectToken)
	if reason != "" {
		*userId = id
		return nil, &OAuthError{Code: ErrInvalidGrant, Status: 400, reason: reason}
	}
	*userId = id

	now := s.cfg.Now().UTC()
	ttl := DefaultTTL
	if client.MaxTtlSeconds > 0 && time.Duration(client.MaxTtlSeconds)*time.Second < ttl {
		ttl = time.Duration(client.MaxTtlSeconds) * time.Second
	}
	if ttl > MaxTTL {
		ttl = MaxTTL
	}
	tokenId, err := randomId()
	if err != nil {
		return nil, &OAuthError{Code: ErrServerError, Status: 500, reason: "jti"}
	}
	*jti = tokenId
	claims := AccessClaims{
		Claims: jwt.Claims{
			Issuer:    s.cfg.Issuer,
			Subject:   id,
			Audience:  jwt.Audience{req.Audience},
			Expiry:    jwt.NewNumericDate(now.Add(ttl)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        tokenId,
		},
		ClientId: client.ClientId,
		Act:      &Actor{Sub: client.ClientId},
		Scope:    strings.Join(scopes, " "),
	}
	raw, err := s.keys.issueToken(claims)
	if err != nil {
		klog.ErrorS(err, "failed to sign exchanged token")
		return nil, &OAuthError{Code: ErrServerError, Status: 500, reason: "sign"}
	}
	return &TokenResponse{
		AccessToken:     raw,
		IssuedTokenType: TokenTypeAccessToken,
		TokenType:       "Bearer",
		ExpiresIn:       int64(ttl.Seconds()),
		Scope:           claims.Scope,
	}, nil
}

// authenticateClient checks the client credentials, status and source address.
func (s *Service) authenticateClient(ctx context.Context, req ExchangeRequest) (*dbclient.ServiceClient, *OAuthError) {
	fail := func(reason string) (*dbclient.ServiceClient, *OAuthError) {
		return nil, &OAuthError{Code: ErrInvalidClient, Description: "client authentication failed", Status: 401, reason: reason}
	}
	if req.ClientId == "" || req.ClientSecret == "" {
		return fail("missing_credentials")
	}
	client, err := s.store.GetServiceClient(ctx, req.ClientId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Hash anyway so an unknown id costs the same as a wrong secret.
			_ = subtle.ConstantTimeCompare([]byte(HashClientSecret(req.ClientSecret)), []byte(HashClientSecret("")))
			return fail("unknown_client")
		}
		klog.ErrorS(err, "failed to load service client", "clientId", req.ClientId)
		return nil, &OAuthError{Code: ErrServerError, Status: 500, reason: "client_store"}
	}
	if client.AuthMethod != "" && client.AuthMethod != AuthMethodSecretBasic {
		return fail("unsupported_auth_method")
	}
	if subtle.ConstantTimeCompare([]byte(HashClientSecret(req.ClientSecret)), []byte(client.SecretHash)) != 1 {
		return fail("bad_secret")
	}
	if client.Disabled {
		return fail("client_disabled")
	}
	if !ipAllowed(client.Whitelist, req.ClientIP) {
		return fail("ip_not_allowed")
	}
	return client, nil
}

// grantedScopes returns the scopes to grant: the requested ones, which must be
// within the client's allowed set, or all allowed ones when none is requested.
func (s *Service) grantedScopes(client *dbclient.ServiceClient, requested string) ([]string, bool) {
	allowed := jsonList(client.AllowedScopes)
	if strings.TrimSpace(requested) == "" {
		return allowed, len(allowed) > 0
	}
	var out []string
	for _, sc := range strings.Fields(requested) {
		if !contains(allowed, sc) {
			return nil, false
		}
		if !contains(out, sc) {
			out = append(out, sc)
		}
	}
	return out, len(out) > 0
}

// resolveSubject maps the subject token to a user id and checks every
// condition for acting for that user. A non-empty reason is the internal cause
// of a refusal.
func (s *Service) resolveSubject(ctx context.Context, clientId, subjectToken string) (userId, reason string) {
	kind, value, ok := strings.Cut(subjectToken, ":")
	if !ok || value == "" {
		return "", "malformed_subject"
	}
	switch kind {
	case "external":
		issuer, subject, ok := strings.Cut(value, "|")
		if !ok || issuer == "" || subject == "" {
			return "", "malformed_subject"
		}
		b, err := s.store.GetIdentityBinding(ctx, clientId, issuer, subject)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", "no_binding"
			}
			return "", "binding_store"
		}
		userId = b.UserId
	case "email":
		email := strings.ToLower(strings.TrimSpace(value))
		userId = commonuser.GenerateUserIdByName(email)
		// The id is a hash of the login email, so confirm the record really carries
		// this address: an alias or a recycled mailbox must not match by accident.
		user, err := s.users.GetUser(ctx, userId)
		if err != nil || strings.ToLower(v1.GetUserEmail(user)) != email {
			return userId, "email_mismatch"
		}
		fallthrough
	case "safe_user_id":
		if kind == "safe_user_id" {
			userId = value
		}
		has, err := s.store.HasIdentityBindingForUser(ctx, clientId, userId)
		if err != nil {
			return userId, "binding_store"
		}
		if !has {
			return userId, "no_binding"
		}
	default:
		return "", "malformed_subject"
	}
	user, err := s.users.GetUser(ctx, userId)
	if err != nil {
		return userId, "user_not_found"
	}
	if user.IsRestricted() {
		return userId, "user_restricted"
	}
	return userId, ""
}

// Principal is a validated exchanged token.
type Principal struct {
	UserId   string
	UserName string
	Roles    []v1.UserRole
	ClientId string
	Jti      string
	Scopes   []string
	Audience []string
	Expiry   time.Time
}

// IsExchangeToken reports whether a bearer token is an access token issued by
// this service. It decides only which authentication branch handles the token;
// it does not validate it. (Without a database, SSO tokens are bare JWTs too, so
// "looks like a JWT" is not enough: the type header and issuer decide.)
func (s *Service) IsExchangeToken(raw string) bool {
	iss, ok := unverifiedIssuer(raw)
	return ok && strings.TrimRight(iss, "/") == s.cfg.Issuer
}

// ValidateAccessToken verifies an exchanged token for the audience and checks
// it is still backed by an enabled client, a live binding and an existing user.
// Role changes and freezing apply at the authorization step, so they are not
// part of the token. An empty audience means this apiserver's.
func (s *Service) ValidateAccessToken(ctx context.Context, raw, audience string) (*Principal, error) {
	if audience == "" {
		audience = s.cfg.Audience
	}
	c, err := s.keys.verifyToken(raw, s.cfg.Issuer, audience, s.cfg.Now())
	if err != nil {
		return nil, err
	}
	client, err := s.store.GetServiceClient(ctx, c.ClientId)
	if err != nil {
		return nil, fmt.Errorf("client lookup: %w", err)
	}
	if client.Disabled {
		return nil, errors.New("client disabled")
	}
	has, err := s.store.HasIdentityBindingForUser(ctx, c.ClientId, c.Subject)
	if err != nil {
		return nil, fmt.Errorf("binding lookup: %w", err)
	}
	if !has {
		return nil, errors.New("binding revoked")
	}
	user, err := s.users.GetUser(ctx, c.Subject)
	if err != nil {
		return nil, fmt.Errorf("user lookup: %w", err)
	}
	return &Principal{
		UserId:   c.Subject,
		UserName: v1.GetUserName(user),
		Roles:    user.Spec.Roles,
		ClientId: c.ClientId,
		Jti:      c.ID,
		Scopes:   strings.Fields(c.Scope),
		Audience: c.Audience,
		Expiry:   c.Expiry.Time(),
	}, nil
}

// JWKS returns the public signing keys.
func (s *Service) JWKS() jose.JSONWebKeySet { return s.keys.JWKS() }

// Discovery returns the OIDC discovery / RFC 8414 metadata document.
func (s *Service) Discovery() map[string]any {
	return map[string]any{
		"issuer":                                s.cfg.Issuer,
		"token_endpoint":                        s.cfg.Issuer + "/oauth/token",
		"jwks_uri":                              s.cfg.Issuer + "/.well-known/jwks.json",
		"grant_types_supported":                 []string{GrantTypeTokenExchange},
		"response_types_supported":              []string{"token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{s.keys.Algorithm()},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"scopes_supported":                      []string{ScopeAPI, ScopeRead},
	}
}

func (s *Service) auditExchange(ctx context.Context, req ExchangeRequest, userId, jti string, oerr *OAuthError) {
	status, result, reason := 200, "ok", ""
	if oerr != nil {
		status, result, reason = oerr.Status, "refused", oerr.reason
	}
	body, _ := json.Marshal(map[string]string{
		"subjectType": trunc(req.SubjectTokenType), "subject": trunc(req.SubjectToken),
		"audience": trunc(req.Audience), "scope": trunc(req.Scope), "result": result, "reason": reason,
	})
	if userId == "" {
		userId = "-"
	}
	log := &dbclient.AuditLog{
		UserId:         trunc(userId),
		UserType:       sql.NullString{String: UserType, Valid: true},
		ClientIP:       sql.NullString{String: trunc(req.ClientIP), Valid: req.ClientIP != ""},
		HttpMethod:     "POST",
		RequestPath:    "/oauth/token",
		ResourceType:   sql.NullString{String: "token-exchange", Valid: true},
		Action:         sql.NullString{String: "exchange", Valid: true},
		RequestBody:    sql.NullString{String: string(body), Valid: true},
		ResponseStatus: status,
		CreateTime:     pq.NullTime{Time: s.cfg.Now().UTC(), Valid: true},
		ActorClientId:  sql.NullString{String: trunc(req.ClientId), Valid: req.ClientId != ""},
		TokenJti:       sql.NullString{String: jti, Valid: jti != ""},
	}
	if err := s.store.InsertAuditLog(ctx, log); err != nil {
		klog.ErrorS(err, "failed to write token exchange audit log", "clientId", req.ClientId)
	}
}

func trunc(v string) string {
	if len(v) > maxAuditField {
		return v[:maxAuditField]
	}
	return v
}

func randomId() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func jsonList(s string) []string {
	var out []string
	if s == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ipAllowed applies the client's source address allow-list (IPs or CIDRs); an
// empty list means no restriction.
func ipAllowed(whitelistJSON, clientIP string) bool {
	list := jsonList(whitelistJSON)
	if len(list) == 0 {
		return true
	}
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, entry := range list {
		if strings.Contains(entry, "/") {
			if _, n, err := net.ParseCIDR(entry); err == nil && n.Contains(ip) {
				return true
			}
		} else if e := net.ParseIP(entry); e != nil && e.Equal(ip) {
			return true
		}
	}
	return false
}

// SetAdminCheck sets the authorization used by the service client administration
// endpoints. Without one they refuse every caller.
func (s *Service) SetAdminCheck(f func(c *gin.Context) error) { s.adminCheck = f }
