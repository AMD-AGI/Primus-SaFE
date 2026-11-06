/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	// maxCreateAuditLogBody bounds the request: the two bodies are cut at 8KB each when stored.
	maxCreateAuditLogBody = 64 << 10
	// maxAuditLogClockSkew is how far in the future a reported operation may be dated.
	maxAuditLogClockSkew = 5 * time.Minute
)

// internalTokenValid reports whether token is SaFE's internal service token (X-Internal-Token). A variable so that
// tests can stand in for the token Secret.
var internalTokenValid = func(token string) bool {
	return authority.InternalAuthInstance().Validate(token)
}

var auditHTTPMethod = regexp.MustCompile(`^[A-Z]{1,16}$`)

// CreateAuditLog records an operation that another SaFE service performed for a user, so that SaFE's audit log stays
// the one place an administrator reads: the API key routes, for example, are served by the identity service, which
// reports each create and delete here. Only internal services may call it (X-Internal-Token). The record is stored
// before the answer, so a 200 means it is in the audit log and the caller may forget it.
func (h *Handler) CreateAuditLog(c *gin.Context) {
	handle(c, h.createAuditLog)
}

func (h *Handler) createAuditLog(c *gin.Context) (interface{}, error) {
	if !internalTokenValid(c.GetHeader(authority.InternalAuthTokenHeader)) {
		return nil, commonerrors.NewForbidden("internal authentication required")
	}
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("database is not enabled")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCreateAuditLogBody)
	var req view.CreateAuditLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, commonerrors.NewRequestEntityTooLargeError(fmt.Sprintf("the request body is larger than %d bytes", maxCreateAuditLogBody))
		}
		return nil, commonerrors.NewBadRequest("invalid request body: " + err.Error())
	}
	record, err := auditLogFromRequest(&req, time.Now())
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	if err = h.dbClient.InsertAuditLog(c.Request.Context(), record); err != nil {
		klog.ErrorS(err, "failed to insert an audit log reported by an internal service",
			"userId", record.UserId, "action", record.Action.String, "requestPath", record.RequestPath)
		return nil, commonerrors.NewInternalError("failed to store the audit log")
	}
	klog.InfoS("audit log reported by an internal service", "userId", record.UserId, "action", record.Action.String,
		"requestPath", record.RequestPath, "responseStatus", record.ResponseStatus, "clientIp", c.ClientIP())
	return nil, nil
}

// auditLogFromRequest checks a reported operation against what an audit_logs row can hold and builds the row. A value
// that does not fit is refused rather than cut, so that what is stored is what was reported.
func auditLogFromRequest(req *view.CreateAuditLogRequest, now time.Time) (*dbclient.AuditLog, error) {
	limits := []struct {
		name, value string
		max         int
		required    bool
	}{
		{"userId", req.UserId, 64, true},
		{"userName", req.UserName, 256, false},
		{"userType", req.UserType, 32, false},
		{"clientIp", req.ClientIP, 64, false},
		{"requestPath", req.RequestPath, 512, true},
		{"resourceType", req.ResourceType, 64, false},
		{"action", req.Action, 64, false},
		{"traceId", req.TraceId, 64, false},
	}
	for _, l := range limits {
		if l.required && strings.TrimSpace(l.value) == "" {
			return nil, fmt.Errorf("%s is required", l.name)
		}
		if utf8.RuneCountInString(l.value) > l.max {
			return nil, fmt.Errorf("%s is longer than %d characters", l.name, l.max)
		}
	}
	if !auditHTTPMethod.MatchString(req.HttpMethod) {
		return nil, errors.New("httpMethod must be an upper-case HTTP method")
	}
	if !strings.HasPrefix(req.RequestPath, "/") {
		return nil, errors.New("requestPath must start with /")
	}
	if req.ResponseStatus < 100 || req.ResponseStatus > 599 {
		return nil, errors.New("responseStatus must be an HTTP status code")
	}
	if req.LatencyMs < 0 {
		return nil, errors.New("latencyMs must not be negative")
	}
	if req.CreateTime == "" {
		return nil, errors.New("createTime is required")
	}
	createTime, err := time.Parse(time.RFC3339, req.CreateTime)
	if err != nil {
		return nil, errors.New("invalid createTime format, expected RFC3339")
	}
	if createTime.After(now.Add(maxAuditLogClockSkew)) {
		return nil, errors.New("createTime is in the future")
	}
	return &dbclient.AuditLog{
		UserId:         req.UserId,
		UserName:       auditNullString(req.UserName),
		UserType:       auditNullString(req.UserType),
		ClientIP:       auditNullString(req.ClientIP),
		HttpMethod:     req.HttpMethod,
		RequestPath:    req.RequestPath,
		ResourceType:   auditNullString(req.ResourceType),
		Action:         auditNullString(req.Action),
		RequestBody:    auditNullString(middleware.SanitizeAuditBody(req.RequestBody)),
		ResponseStatus: req.ResponseStatus,
		ResponseBody:   auditNullString(middleware.SanitizeAuditBody(req.ResponseBody)),
		LatencyMs:      sql.NullInt64{Int64: req.LatencyMs, Valid: req.LatencyMs > 0},
		TraceId:        auditNullString(req.TraceId),
		CreateTime:     pq.NullTime{Time: createTime.UTC(), Valid: true},
	}, nil
}

func auditNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
