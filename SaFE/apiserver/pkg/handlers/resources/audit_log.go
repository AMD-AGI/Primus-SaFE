/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// ListAuditLog handles listing audit logs based on query parameters.
// Supports filtering by user, resource type, time range, and pagination.
// Only admin users can view all audit logs; regular users can only view their own.
func (h *Handler) ListAuditLog(c *gin.Context) {
	handle(c, h.listAuditLog)
}

// listAuditLog implements the audit log listing logic.
// Only admin users can access audit logs.
func (h *Handler) listAuditLog(c *gin.Context) (interface{}, error) {
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("database is not enabled")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Check permissions using RBAC - user must have "list" permission on "auditlogs" resource
	err := h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: "auditlogs",
		Verb:         "list",
		UserId:       userId,
	})
	if err != nil {
		klog.ErrorS(err, "user not authorized to access audit logs", "userId", userId)
		return nil, err
	}

	req, err := parseListAuditLogQuery(c)
	if err != nil {
		return nil, err
	}

	tags := dbclient.GetAuditLogFieldTags()
	var conditions sqrl.And

	// Filter by user info
	if req.UserId != "" {
		conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(tags, "UserId"): req.UserId})
	}
	if req.UserName != "" {
		// Use partial match (ILIKE) for userName to support fuzzy search
		conditions = append(conditions, sqrl.ILike{dbclient.GetFieldTag(tags, "UserName"): "%" + req.UserName + "%"})
	}
	// Support multiple userType values (comma-separated, e.g., ?userType=default,sso)
	if req.UserType != "" {
		userTypes := splitAndTrim(req.UserType)
		if len(userTypes) == 1 {
			conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(tags, "UserType"): userTypes[0]})
		} else if len(userTypes) > 1 {
			conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(tags, "UserType"): userTypes})
		}
	}

	// Add filters
	// Support multiple resourceType values (comma-separated, e.g., ?resourceType=workloads,apikeys)
	if req.ResourceType != "" {
		resourceTypes := splitAndTrim(req.ResourceType)
		if len(resourceTypes) == 1 {
			conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(tags, "ResourceType"): resourceTypes[0]})
		} else if len(resourceTypes) > 1 {
			conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(tags, "ResourceType"): resourceTypes})
		}
	}
	// Support multiple httpMethod values (comma-separated, e.g., ?httpMethod=POST,DELETE)
	if req.HttpMethod != "" {
		httpMethods := splitAndTrim(req.HttpMethod)
		if len(httpMethods) == 1 {
			conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(tags, "HttpMethod"): httpMethods[0]})
		} else if len(httpMethods) > 1 {
			conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(tags, "HttpMethod"): httpMethods})
		}
	}
	if req.RequestPath != "" {
		conditions = append(conditions, sqrl.ILike{dbclient.GetFieldTag(tags, "RequestPath"): "%" + req.RequestPath + "%"})
	}
	if req.ResponseStatus != nil {
		conditions = append(conditions, sqrl.Eq{dbclient.GetFieldTag(tags, "ResponseStatus"): *req.ResponseStatus})
	}

	// Add time range filters
	if req.StartTime != "" {
		startTime, err := time.Parse(time.RFC3339, req.StartTime)
		if err != nil {
			return nil, commonerrors.NewBadRequest("invalid startTime format, expected RFC3339")
		}
		conditions = append(conditions, sqrl.GtOrEq{dbclient.GetFieldTag(tags, "CreateTime"): startTime})
	}
	if req.EndTime != "" {
		endTime, err := time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			return nil, commonerrors.NewBadRequest("invalid endTime format, expected RFC3339")
		}
		conditions = append(conditions, sqrl.LtOrEq{dbclient.GetFieldTag(tags, "CreateTime"): endTime})
	}

	var query sqrl.Sqlizer
	if len(conditions) > 0 {
		query = conditions
	}

	orderBy := buildListAuditLogOrderBy(req, tags)

	totalCount, err := h.dbClient.CountAuditLogs(c.Request.Context(), query)
	if err != nil {
		klog.ErrorS(err, "failed to count audit logs", "userId", userId)
		return nil, commonerrors.NewInternalError("failed to list audit logs")
	}

	records, err := h.dbClient.SelectAuditLogs(c.Request.Context(), query, orderBy, req.Limit, req.Offset)
	if err != nil {
		klog.ErrorS(err, "failed to select audit logs", "userId", userId)
		return nil, commonerrors.NewInternalError("failed to list audit logs")
	}

	items := make([]view.AuditLogItem, 0, len(records))
	for _, record := range records {
		items = append(items, convertToAuditLogItem(record))
	}

	return &view.ListAuditLogResponse{
		TotalCount: totalCount,
		Items:      items,
	}, nil
}

// parseListAuditLogQuery parses query parameters for listing audit logs.
func parseListAuditLogQuery(c *gin.Context) (*view.ListAuditLogRequest, error) {
	query := &view.ListAuditLogRequest{}
	err := c.ShouldBindWith(&query, binding.Query)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.Limit <= 0 {
		query.Limit = view.DefaultQueryLimit
	}
	if query.Order == "" {
		query.Order = dbclient.DESC
	}
	if query.SortBy == "" {
		query.SortBy = dbclient.CreatedTime
	} else {
		query.SortBy = strings.ToLower(query.SortBy)
	}
	return query, nil
}

// buildListAuditLogOrderBy builds the ORDER BY clause for listing audit logs.
func buildListAuditLogOrderBy(req *view.ListAuditLogRequest, dbTags map[string]string) []string {
	var orderBy []string
	if req.SortBy != "" {
		sortBy := dbclient.GetFieldTag(dbTags, req.SortBy)
		if sortBy != "" {
			orderBy = append(orderBy, sortBy+" "+req.Order)
		}
	}
	// Always add create_time as secondary sort
	createTime := dbclient.GetFieldTag(dbTags, "CreateTime")
	if len(orderBy) == 0 || !strings.Contains(orderBy[0], createTime) {
		orderBy = append(orderBy, createTime+" "+dbclient.DESC)
	}
	return orderBy
}

// convertToAuditLogItem converts a database record to an API response item.
func convertToAuditLogItem(record *dbclient.AuditLog) view.AuditLogItem {
	item := view.AuditLogItem{
		Id:             record.Id,
		UserId:         record.UserId,
		HttpMethod:     record.HttpMethod,
		RequestPath:    record.RequestPath,
		ResponseStatus: record.ResponseStatus,
	}

	if record.UserName.Valid && record.UserName.String != "" {
		item.UserName = record.UserName.String
	} else if item.UserId != "" {
		// Fallback for historical data: use userId if userName is empty
		item.UserName = item.UserId
	} else {
		item.UserName = "unknown"
	}
	if record.UserType.Valid && record.UserType.String != "" {
		item.UserType = record.UserType.String
	} else if item.UserId != "" {
		// Fallback for historical data: use "default" if userType is empty
		item.UserType = "default"
	} else {
		item.UserType = "unknown"
	}
	if record.ClientIP.Valid {
		item.ClientIP = record.ClientIP.String
	}
	if record.ResourceType.Valid {
		item.ResourceType = record.ResourceType.String
	}
	if record.RequestBody.Valid {
		item.RequestBody = record.RequestBody.String
	}
	if record.LatencyMs.Valid {
		item.LatencyMs = record.LatencyMs.Int64
	}
	if record.TraceId.Valid {
		item.TraceId = record.TraceId.String
	}
	if record.CreateTime.Valid {
		item.CreateTime = timeutil.FormatRFC3339(record.CreateTime.Time)
	}

	if record.Action.Valid {
		item.Action = record.Action.String
	}

	return item
}

// splitAndTrim splits a comma-separated string and trims whitespace from each element.
// Empty strings are filtered out.
func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
