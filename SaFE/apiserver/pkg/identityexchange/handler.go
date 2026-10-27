/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const maxFieldLen = 256

// RegisterPublicRoutes registers the protocol endpoints, which authenticate the
// client themselves. They live outside /api/v1 so the issuer URL and the JWKS
// address do not change if the service is later moved out of the apiserver.
func (s *Service) RegisterPublicRoutes(e gin.IRoutes) {
	e.POST("/oauth/token", s.HandleToken)
	e.GET("/.well-known/openid-configuration", s.HandleDiscovery)
	e.GET("/.well-known/jwks.json", s.HandleJWKS)
}

// HandleToken is the RFC 8693 token endpoint.
func (s *Service) HandleToken(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if err := c.Request.ParseForm(); err != nil {
		writeOAuthError(c, &OAuthError{Code: ErrInvalidRequest, Status: http.StatusBadRequest})
		return
	}
	// Credentials come from HTTP Basic (preferred) or the form body, never both.
	form := c.Request.PostForm
	id, secret, hasBasic := c.Request.BasicAuth()
	if hasBasic && (form.Get("client_id") != "" || form.Get("client_secret") != "") {
		writeOAuthError(c, &OAuthError{Code: ErrInvalidRequest, Description: "multiple client authentication methods", Status: http.StatusBadRequest})
		return
	}
	if !hasBasic {
		id, secret = form.Get("client_id"), form.Get("client_secret")
	}
	resp, oerr := s.Exchange(c.Request.Context(), ExchangeRequest{
		ClientId:           id,
		ClientSecret:       secret,
		ClientIP:           c.ClientIP(),
		GrantType:          form.Get("grant_type"),
		SubjectToken:       form.Get("subject_token"),
		SubjectTokenType:   form.Get("subject_token_type"),
		Audience:           form.Get("audience"),
		Scope:              form.Get("scope"),
		RequestedTokenType: form.Get("requested_token_type"),
	})
	if oerr != nil {
		writeOAuthError(c, oerr)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func writeOAuthError(c *gin.Context, e *OAuthError) {
	if e.Status == http.StatusUnauthorized {
		c.Header("WWW-Authenticate", `Basic realm="oauth"`)
	}
	c.AbortWithStatusJSON(e.Status, e)
}

// HandleDiscovery serves the OIDC discovery document.
func (s *Service) HandleDiscovery(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, s.Discovery())
}

// HandleJWKS serves the public signing keys.
func (s *Service) HandleJWKS(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, s.JWKS())
}

// ---- identity bindings: the user confirms which external identity is theirs ----

// BindingRequest creates a binding for the authenticated user.
type BindingRequest struct {
	ClientId string `json:"clientId"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
}

// BindingView is a binding as returned to its owner.
type BindingView struct {
	Id           int64  `json:"id"`
	ClientId     string `json:"clientId"`
	Issuer       string `json:"issuer"`
	Subject      string `json:"subject"`
	CreationTime string `json:"creationTime"`
}

// CreateBinding binds an external identity to the calling user. The caller is
// the user's own session or API key: Forbidden keeps exchanged tokens away, so
// a service client cannot create the binding it needs.
func (s *Service) CreateBinding(c *gin.Context) {
	handleJSON(c, func() (any, error) {
		userId := c.GetString(common.UserId)
		if userId == "" {
			return nil, commonerrors.NewUnauthorized("user not authenticated")
		}
		var req BindingRequest
		if err := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<16)).Decode(&req); err != nil {
			return nil, commonerrors.NewBadRequest("invalid request body")
		}
		req.ClientId, req.Issuer, req.Subject = strings.TrimSpace(req.ClientId), strings.TrimSpace(req.Issuer), strings.TrimSpace(req.Subject)
		if req.ClientId == "" || req.Issuer == "" || req.Subject == "" {
			return nil, commonerrors.NewBadRequest("clientId, issuer and subject are required")
		}
		if len(req.ClientId) > 64 || len(req.Issuer) > maxFieldLen || len(req.Subject) > maxFieldLen || strings.ContainsAny(req.Issuer, "|") {
			return nil, commonerrors.NewBadRequest("clientId, issuer or subject is invalid")
		}
		client, err := s.store.GetServiceClient(c.Request.Context(), req.ClientId)
		if err != nil || client.Disabled {
			return nil, commonerrors.NewNotFoundWithMessage("service client not found")
		}
		b := &dbclient.IdentityBinding{
			ClientId: req.ClientId, Issuer: req.Issuer, Subject: req.Subject, UserId: userId,
			CreationTime: pq.NullTime{Time: s.cfg.Now().UTC(), Valid: true},
		}
		if err = s.store.InsertIdentityBinding(c.Request.Context(), b); err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				return nil, commonerrors.NewAlreadyExist("this identity is already bound")
			}
			klog.ErrorS(err, "failed to create identity binding", "userId", userId)
			return nil, commonerrors.NewInternalError("failed to create binding")
		}
		return bindingView(b), nil
	})
}

// ListBindings lists the calling user's bindings.
func (s *Service) ListBindings(c *gin.Context) {
	handleJSON(c, func() (any, error) {
		list, err := s.store.ListIdentityBindingsByUser(c.Request.Context(), c.GetString(common.UserId))
		if err != nil {
			return nil, commonerrors.NewInternalError("failed to list bindings")
		}
		out := make([]BindingView, 0, len(list))
		for _, b := range list {
			out = append(out, bindingView(b))
		}
		return out, nil
	})
}

// DeleteBinding revokes one of the calling user's bindings. Tokens already
// issued for it stop working at once, since validation re-checks the binding.
func (s *Service) DeleteBinding(c *gin.Context) {
	handleJSON(c, func() (any, error) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return nil, commonerrors.NewBadRequest("invalid id")
		}
		ok, err := s.store.DeleteIdentityBinding(c.Request.Context(), id, c.GetString(common.UserId))
		if err != nil {
			return nil, commonerrors.NewInternalError("failed to delete binding")
		}
		if !ok {
			return nil, commonerrors.NewNotFoundWithMessage("binding not found")
		}
		return nil, nil
	})
}

func bindingView(b *dbclient.IdentityBinding) BindingView {
	v := BindingView{Id: b.Id, ClientId: b.ClientId, Issuer: b.Issuer, Subject: b.Subject}
	if b.CreationTime.Valid {
		v.CreationTime = b.CreationTime.Time.UTC().Format(time.RFC3339)
	}
	return v
}

// ---- service client administration (system administrators only) ----

// CreateClientRequest registers a service client.
type CreateClientRequest struct {
	Name             string   `json:"name"`
	AllowedAudiences []string `json:"allowedAudiences"`
	AllowedScopes    []string `json:"allowedScopes"`
	Whitelist        []string `json:"whitelist"`
	MaxTtlSeconds    int      `json:"maxTtlSeconds"`
}

// ClientView is a service client as returned to administrators (never the secret hash).
type ClientView struct {
	ClientId         string   `json:"clientId"`
	Name             string   `json:"name"`
	AuthMethod       string   `json:"authMethod"`
	AllowedAudiences []string `json:"allowedAudiences"`
	AllowedScopes    []string `json:"allowedScopes"`
	Whitelist        []string `json:"whitelist"`
	MaxTtlSeconds    int      `json:"maxTtlSeconds"`
	Disabled         bool     `json:"disabled"`
	CreatedBy        string   `json:"createdBy"`
	// Secret is returned once, at creation. The field is named "secret" so the
	// audit middleware redacts it from the stored response body.
	Secret string `json:"secret,omitempty"`
}

// CreateClient registers a service client and returns its secret once.
func (s *Service) CreateClient(c *gin.Context) {
	handleJSON(c, func() (any, error) {
		if err := s.requireAdmin(c); err != nil {
			return nil, err
		}
		var req CreateClientRequest
		if err := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<16)).Decode(&req); err != nil {
			return nil, commonerrors.NewBadRequest("invalid request body")
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" || len(req.Name) > 100 || len(req.AllowedAudiences) == 0 || len(req.AllowedScopes) == 0 {
			return nil, commonerrors.NewBadRequest("name, allowedAudiences and allowedScopes are required")
		}
		if req.MaxTtlSeconds < 0 || req.MaxTtlSeconds > int(MaxTTL.Seconds()) {
			return nil, commonerrors.NewBadRequest("maxTtlSeconds must be between 0 and 900")
		}
		if req.MaxTtlSeconds == 0 {
			req.MaxTtlSeconds = int(DefaultTTL.Seconds())
		}
		wl, err := normalizeWhitelist(req.Whitelist)
		if err != nil {
			return nil, commonerrors.NewBadRequest(err.Error())
		}
		clientId, secret, err := GenerateClientCredentials()
		if err != nil {
			return nil, commonerrors.NewInternalError("failed to generate credentials")
		}
		rec := &dbclient.ServiceClient{
			ClientId: clientId, Name: req.Name, SecretHash: HashClientSecret(secret), AuthMethod: AuthMethodSecretBasic,
			AllowedAudiences: mustJSON(req.AllowedAudiences), AllowedScopes: mustJSON(req.AllowedScopes), Whitelist: mustJSON(wl),
			MaxTtlSeconds: req.MaxTtlSeconds, CreatedBy: c.GetString(common.UserId),
			CreationTime: pq.NullTime{Time: s.cfg.Now().UTC(), Valid: true},
		}
		if err = s.store.InsertServiceClient(c.Request.Context(), rec); err != nil {
			klog.ErrorS(err, "failed to create service client")
			return nil, commonerrors.NewInternalError("failed to create service client")
		}
		v := clientView(rec)
		v.Secret = secret
		return v, nil
	})
}

// ListClients lists the service clients.
func (s *Service) ListClients(c *gin.Context) {
	handleJSON(c, func() (any, error) {
		if err := s.requireAdmin(c); err != nil {
			return nil, err
		}
		list, err := s.store.ListServiceClients(c.Request.Context())
		if err != nil {
			return nil, commonerrors.NewInternalError("failed to list service clients")
		}
		out := make([]ClientView, 0, len(list))
		for _, rec := range list {
			out = append(out, clientView(rec))
		}
		return out, nil
	})
}

// DisableClient disables a service client; EnableClient re-enables it.
func (s *Service) DisableClient(c *gin.Context) { s.setDisabled(c, true) }

// EnableClient re-enables a service client.
func (s *Service) EnableClient(c *gin.Context) { s.setDisabled(c, false) }

func (s *Service) setDisabled(c *gin.Context, disabled bool) {
	handleJSON(c, func() (any, error) {
		if err := s.requireAdmin(c); err != nil {
			return nil, err
		}
		ok, err := s.store.SetServiceClientDisabled(c.Request.Context(), c.Param("clientId"), disabled)
		if err != nil {
			return nil, commonerrors.NewInternalError("failed to update service client")
		}
		if !ok {
			return nil, commonerrors.NewNotFoundWithMessage("service client not found")
		}
		return nil, nil
	})
}

func (s *Service) requireAdmin(c *gin.Context) error {
	if s.adminCheck == nil {
		return commonerrors.NewForbidden("system administrator required")
	}
	return s.adminCheck(c)
}

func clientView(rec *dbclient.ServiceClient) ClientView {
	return ClientView{
		ClientId: rec.ClientId, Name: rec.Name, AuthMethod: rec.AuthMethod,
		AllowedAudiences: nonNil(jsonList(rec.AllowedAudiences)), AllowedScopes: nonNil(jsonList(rec.AllowedScopes)),
		Whitelist: nonNil(jsonList(rec.Whitelist)), MaxTtlSeconds: rec.MaxTtlSeconds, Disabled: rec.Disabled, CreatedBy: rec.CreatedBy,
	}
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func mustJSON(l []string) string {
	if l == nil {
		l = []string{}
	}
	b, _ := json.Marshal(l)
	return string(b)
}

func normalizeWhitelist(in []string) ([]string, error) {
	out := []string{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !validIPOrCIDR(e) {
			return nil, errors.New("invalid whitelist entry: " + e)
		}
		if !contains(out, e) {
			out = append(out, e)
		}
	}
	return out, nil
}

// handleJSON runs fn and writes the result or the error the way the other
// apiserver endpoints do.
func handleJSON(c *gin.Context, fn func() (any, error)) {
	data, err := fn()
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if data == nil {
		c.Status(http.StatusOK)
		return
	}
	c.JSON(http.StatusOK, data)
}

func validIPOrCIDR(e string) bool {
	if strings.Contains(e, "/") {
		_, _, err := net.ParseCIDR(e)
		return err == nil
	}
	return net.ParseIP(e) != nil
}
