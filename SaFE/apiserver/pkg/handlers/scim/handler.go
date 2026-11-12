/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identity"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
)

const (
	contentType = "application/scim+json"

	schemaUser  = "urn:ietf:params:scim:schemas:core:2.0:User"
	schemaList  = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	schemaError = "urn:ietf:params:scim:api:messages:2.0:Error"

	defaultCount = 100
	maxCount     = 1000

	retryCount = 5
	retryDelay = 200 * time.Millisecond
)

type server struct {
	h     *resources.Handler
	token *Token
}

type scimUser struct {
	Schemas    []string `json:"schemas"`
	ID         string   `json:"id"`
	ExternalID string   `json:"externalId"`
	UserName   string   `json:"userName"`
	Active     bool     `json:"active"`
}

type listResponse struct {
	Schemas      []string   `json:"schemas"`
	TotalResults int        `json:"totalResults"`
	StartIndex   int        `json:"startIndex"`
	ItemsPerPage int        `json:"itemsPerPage"`
	Resources    []scimUser `json:"Resources"`
}

type scimError struct {
	Schemas  []string `json:"schemas"`
	Status   string   `json:"status"`
	ScimType string   `json:"scimType,omitempty"`
	Detail   string   `json:"detail"`
}

type patchBody struct {
	Schemas    []string        `json:"schemas"`
	Operations []patchOp       `json:"Operations"`
	Op         string          `json:"op"`
	Path       string          `json:"path"`
	Value      json.RawMessage `json:"value"`
}

type patchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// Mount registers /scim/v2/Users. The routes use the current SCIM bearer and are outside user authentication.
func Mount(e *gin.Engine, h *resources.Handler, token *Token) {
	s := &server{h: h, token: token}
	g := e.Group("/scim/v2")
	g.Use(s.authenticate)
	g.GET("/Users", s.listUsers)
	g.GET("/Users/:id", s.getUser)
	g.PATCH("/Users/:id", s.patchUser)
	g.DELETE("/Users/:id", s.deleteUser)
}

// authenticate accepts only a bearer token that matches the SCIM secret.
func (s *server) authenticate(c *gin.Context) {
	if !bearerMatches(c.GetHeader("Authorization"), s.token.Load()) {
		writeError(c, http.StatusUnauthorized, "the bearer token is invalid")
		c.Abort()
	}
}

// listUsers returns one page of the users that match filter. startIndex is 1-based. count defaults to 100.
func (s *server) listUsers(c *gin.Context) {
	start, count, err := pageParams(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	list := &v1.UserList{}
	if err = s.h.List(c.Request.Context(), list); err != nil {
		writeError(c, http.StatusInternalServerError, "failed to list users")
		return
	}
	items, err := filterUsers(list.Items, c.Query("filter"))
	if err != nil {
		writeJSON(c, http.StatusBadRequest, scimError{
			Schemas:  []string{schemaError},
			Status:   strconv.Itoa(http.StatusBadRequest),
			ScimType: "invalidFilter",
			Detail:   err.Error(),
		})
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	from := start - 1
	if from > len(items) {
		from = len(items)
	}
	to := from + count
	if to > len(items) {
		to = len(items)
	}
	page := items[from:to]
	resources := make([]scimUser, 0, len(page))
	for i := range page {
		resources = append(resources, toSCIMUser(&page[i]))
	}
	writeJSON(c, http.StatusOK, listResponse{
		Schemas:      []string{schemaList},
		TotalResults: len(items),
		StartIndex:   start,
		ItemsPerPage: len(resources),
		Resources:    resources,
	})
}

// getUser returns one user. An unknown id is 404.
func (s *server) getUser(c *gin.Context) {
	user, err := s.load(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAPIError(c, err, "failed to read the user")
		return
	}
	writeJSON(c, http.StatusOK, toSCIMUser(user))
}

// patchUser applies replace operations on active and on groups.
// groups is the identity's platform groups; spec.roles is rewritten from auth.group_mappings.
func (s *server) patchUser(c *gin.Context) {
	plan, err := parsePatch(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	id := c.Param("id")
	var updated *v1.User
	err = backoff.ConflictRetry(func() error {
		user, gerr := s.load(c.Request.Context(), id)
		if gerr != nil {
			return gerr
		}
		changed := false
		for _, active := range plan.active {
			if applyActive(user, active) {
				changed = true
			}
		}
		if plan.groups != nil {
			rolesChanged, rerr := applyMappedRoles(user, *plan.groups)
			if rerr != nil {
				return rerr
			}
			if rolesChanged {
				changed = true
			}
		}
		if !changed {
			updated = user
			return nil
		}
		if uerr := s.h.Update(c.Request.Context(), user); uerr != nil {
			return uerr
		}
		updated = user
		return nil
	}, retryCount, retryDelay)
	if err != nil {
		writeAPIError(c, err, "failed to update the user")
		return
	}
	klog.Infof("scim patch user %s", id)
	writeJSON(c, http.StatusOK, toSCIMUser(updated))
}

// deleteUser removes managed workspaces and deletes the user. An unknown id is 204.
func (s *server) deleteUser(c *gin.Context) {
	id := c.Param("id")
	user, err := s.load(c.Request.Context(), id)
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.Status(http.StatusNoContent)
			return
		}
		writeAPIError(c, err, "failed to read the user")
		return
	}
	if err = s.h.DeleteUserRecord(c.Request.Context(), user); err != nil {
		if apierrors.IsNotFound(err) {
			c.Status(http.StatusNoContent)
			return
		}
		writeError(c, http.StatusInternalServerError, "failed to delete the user")
		return
	}
	klog.Infof("scim delete user %s", id)
	c.Status(http.StatusNoContent)
}

// load reads the user by id.
func (s *server) load(ctx context.Context, id string) (*v1.User, error) {
	if id == "" {
		return nil, apierrors.NewBadRequest("the user id is empty")
	}
	user := &v1.User{}
	if err := s.h.Get(ctx, client.ObjectKey{Name: id}, user); err != nil {
		return nil, err
	}
	return user, nil
}

// pageParams reads startIndex and count. A non-integer is an error.
func pageParams(c *gin.Context) (start, count int, err error) {
	start, count = 1, defaultCount
	if raw := c.Query("startIndex"); raw != "" {
		n, nerr := strconv.Atoi(raw)
		if nerr != nil {
			return 0, 0, errBadQuery("startIndex")
		}
		if n < 1 {
			n = 1
		}
		start = n
	}
	if raw := c.Query("count"); raw != "" {
		n, nerr := strconv.Atoi(raw)
		if nerr != nil {
			return 0, 0, errBadQuery("count")
		}
		if n < 0 {
			n = 0
		}
		if n > maxCount {
			n = maxCount
		}
		count = n
	}
	return start, count, nil
}

type queryError string

// Error returns the query message.
func (e queryError) Error() string { return string(e) }

// filterUsers returns the users that match a SCIM filter of the form `attr eq "value"`, where attr
// is id, externalId or userName. Attribute names, the operator and the value compare case-insensitively.
// An empty filter matches every user; any other filter is an error.
func filterUsers(users []v1.User, raw string) ([]v1.User, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return users, nil
	}
	fields := strings.SplitN(raw, " ", 3)
	if len(fields) != 3 || !strings.EqualFold(fields[1], "eq") {
		return nil, queryError(`filter must be attr eq "value"`)
	}
	value := strings.TrimSpace(fields[2])
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return nil, queryError("filter value must be a quoted string")
	}
	value, err := strconv.Unquote(value)
	if err != nil {
		return nil, queryError("filter value must be a quoted string")
	}
	var attr func(u *v1.User) string
	switch strings.ToLower(fields[0]) {
	case "id", "externalid":
		attr = func(u *v1.User) string { return u.Name }
	case "username":
		attr = func(u *v1.User) string { return toSCIMUser(u).UserName }
	default:
		return nil, queryError("filter supports id, externalId and userName")
	}
	out := make([]v1.User, 0, 1)
	for i := range users {
		if strings.EqualFold(attr(&users[i]), value) {
			out = append(out, users[i])
		}
	}
	return out, nil
}

// errBadQuery reports a query parameter that is not an integer.
func errBadQuery(name string) error {
	return queryError(name + " must be an integer")
}

// parsedPatch is the replace operations a SCIM PATCH carries.
type parsedPatch struct {
	active []bool
	// groups is set when the patch replaces groups, including with an empty list.
	groups *[]string
}

// parsePatch reads a PatchOp or a single replace operation. active and groups are accepted; anything else is refused.
func parsePatch(c *gin.Context) (parsedPatch, error) {
	var body patchBody
	if err := c.ShouldBindJSON(&body); err != nil {
		return parsedPatch{}, queryError("the patch body is not valid JSON")
	}
	ops := body.Operations
	if len(ops) == 0 && body.Op != "" {
		ops = []patchOp{{Op: body.Op, Path: body.Path, Value: body.Value}}
	}
	if len(ops) == 0 {
		return parsedPatch{}, queryError("the patch has no operations")
	}
	var plan parsedPatch
	for _, op := range ops {
		if active, ok := activeOf(op); ok {
			plan.active = append(plan.active, active)
			continue
		}
		groups, ok := groupsOf(op)
		if !ok {
			return parsedPatch{}, queryError("only replace of active or groups is supported")
		}
		plan.groups = &groups
	}
	return plan, nil
}

// groupsOf reads a replace of groups, a JSON array of group names.
// RFC 7643 defines groups as read-only. This server accepts the replace as an extension.
func groupsOf(op patchOp) ([]string, bool) {
	if !strings.EqualFold(strings.TrimSpace(op.Op), "replace") || !strings.EqualFold(strings.TrimSpace(op.Path), "groups") {
		return nil, false
	}
	var groups []string
	if err := json.Unmarshal(op.Value, &groups); err != nil || groups == nil {
		return nil, false
	}
	return groups, true
}

// applyMappedRoles replaces the platform roles on user with the roles auth.group_mappings gives groups.
// A role that some mapping can grant is dropped when the new groups do not grant it.
// A role that no mapping grants stays. It reports whether spec.roles changed.
func applyMappedRoles(user *v1.User, groups []string) (bool, error) {
	mappings, err := commonconfig.GetAuthGroupMappings()
	if err != nil {
		return false, err
	}
	grantable := map[v1.UserRole]struct{}{}
	for _, m := range mappings {
		for _, role := range m.Roles {
			grantable[v1.UserRole(role)] = struct{}{}
		}
	}
	mapped := extauth.MapGroups(user.Name, groups, mappings, false).Roles
	granted := map[v1.UserRole]struct{}{}
	for _, r := range mapped {
		granted[r] = struct{}{}
	}
	next := make([]v1.UserRole, 0, len(user.Spec.Roles)+len(mapped))
	for _, r := range user.Spec.Roles {
		if v1.IsContainRole(extauth.PlatformRoles, r) {
			continue
		}
		if _, ok := grantable[r]; ok {
			if _, still := granted[r]; !still {
				continue
			}
		}
		next = append(next, r)
	}
	for _, r := range mapped {
		if !v1.IsContainRole(next, r) {
			next = append(next, r)
		}
	}
	if commonuser.IsRolesEqual(user.Spec.Roles, next) {
		return false, nil
	}
	user.Spec.Roles = next
	return true, nil
}

// activeOf reads a replace of active, either a boolean or {"active": <bool>}.
func activeOf(op patchOp) (bool, bool) {
	if !strings.EqualFold(strings.TrimSpace(op.Op), "replace") {
		return false, false
	}
	if len(bytes.TrimSpace(op.Value)) == 0 || bytes.Equal(bytes.TrimSpace(op.Value), []byte("null")) {
		return false, false
	}
	path := strings.TrimSpace(op.Path)
	if path != "" && !strings.EqualFold(path, "active") {
		return false, false
	}
	if path != "" {
		var b bool
		if err := json.Unmarshal(op.Value, &b); err == nil {
			return b, true
		}
	}
	var wrapped struct {
		Active *bool `json:"active"`
	}
	if err := json.Unmarshal(op.Value, &wrapped); err == nil && wrapped.Active != nil {
		return *wrapped.Active, true
	}
	return false, false
}

// applyActive freezes the user and sets the SCIM annotation, or clears both when the annotation is present.
// It returns whether the object changed.
func applyActive(user *v1.User, active bool) bool {
	if active {
		if v1.GetAnnotation(user, identity.DeactivatedAnnotation) == "" {
			return false
		}
		user.Spec.RestrictedType = v1.UserNormal
		v1.RemoveAnnotation(user, identity.DeactivatedAnnotation)
		return true
	}
	changed := user.Spec.RestrictedType != v1.UserFrozen
	user.Spec.RestrictedType = v1.UserFrozen
	if v1.SetAnnotation(user, identity.DeactivatedAnnotation, v1.TrueStr) {
		changed = true
	}
	return changed
}

// toSCIMUser maps a user record to the SCIM attributes this endpoint returns.
func toSCIMUser(user *v1.User) scimUser {
	name := v1.GetUserName(user)
	if name == "" {
		name = user.Name
	}
	return scimUser{
		Schemas:    []string{schemaUser},
		ID:         user.Name,
		ExternalID: user.Name,
		UserName:   name,
		Active:     !user.IsRestricted(),
	}
}

// writeJSON writes a SCIM JSON body.
func writeJSON(c *gin.Context, code int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "failed to encode the response")
		return
	}
	c.Data(code, contentType, data)
}

// writeError writes a SCIM error body.
func writeError(c *gin.Context, code int, detail string) {
	writeJSON(c, code, scimError{
		Schemas: []string{schemaError},
		Status:  strconv.Itoa(code),
		Detail:  detail,
	})
}

// writeAPIError maps a Kubernetes error onto a SCIM status. detail is used for other failures.
func writeAPIError(c *gin.Context, err error, detail string) {
	switch {
	case apierrors.IsNotFound(err):
		writeError(c, http.StatusNotFound, "user not found")
	case apierrors.IsBadRequest(err), apierrors.IsInvalid(err):
		writeError(c, http.StatusBadRequest, err.Error())
	default:
		writeError(c, http.StatusInternalServerError, detail)
	}
}
