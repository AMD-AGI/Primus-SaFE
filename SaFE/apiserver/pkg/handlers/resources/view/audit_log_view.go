/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

// ListAuditLogRequest represents query parameters for listing audit logs
type ListAuditLogRequest struct {
	// Offset is the pagination offset
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit is the pagination limit (max 100)
	Limit int `form:"limit" binding:"omitempty,min=1,max=100"`
	// SortBy is the field to sort by (e.g., createdAt, userId)
	SortBy string `form:"sortBy" binding:"omitempty"`
	// Order is the sort order (desc or asc)
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// UserId is the optional filter by user ID (exact match)
	UserId string `form:"userId" binding:"omitempty"`
	// UserName is the optional filter by user name (partial match)
	UserName string `form:"userName" binding:"omitempty"`
	// UserType is the optional filter by user type (comma-separated for multiple values, e.g., "default,sso,apikey")
	UserType string `form:"userType" binding:"omitempty"`
	// ResourceType is the optional filter by resource type (comma-separated for multiple values, e.g., "workloads,apikeys")
	ResourceType string `form:"resourceType" binding:"omitempty"`
	// HttpMethod is the optional filter by HTTP method (comma-separated for multiple values, e.g., "POST,DELETE")
	HttpMethod string `form:"httpMethod" binding:"omitempty"`
	// RequestPath is the optional filter by request path (partial match)
	RequestPath string `form:"requestPath" binding:"omitempty"`
	// Keyword is a general search parameter that matches against both user_name and
	// request_path (OR logic). Intended for the frontend single search box use case:
	// e.g., ?keyword=deepseek-lite matches any log whose user name OR request path
	// contains that string. Takes precedence over userName and requestPath when set.
	Keyword string `form:"keyword" binding:"omitempty"`
	// StartTime is the optional start time filter (RFC3339 format)
	StartTime string `form:"startTime" binding:"omitempty"`
	// EndTime is the optional end time filter (RFC3339 format)
	EndTime string `form:"endTime" binding:"omitempty"`
	// ResponseStatus is the optional filter by response HTTP status code
	ResponseStatus *int `form:"responseStatus" binding:"omitempty,min=100,max=599"`
}

// ListAuditLogResponse represents the response for listing audit logs
type ListAuditLogResponse struct {
	// TotalCount is the total number of audit logs matching the query
	TotalCount int `json:"totalCount"`
	// Items is the list of audit logs
	Items []AuditLogItem `json:"items"`
}

// AuditLogItem represents a single audit log entry in the response
type AuditLogItem struct {
	// Id is the unique identifier of the audit log
	Id int64 `json:"id"`
	// UserId is the ID of the user who performed the operation
	UserId string `json:"userId"`
	// UserName is the name of the user who performed the operation
	UserName string `json:"userName,omitempty"`
	// UserType is the type of user (e.g., "default", "sso", "apiKey")
	UserType string `json:"userType,omitempty"`
	// ClientIP is the IP address of the client
	ClientIP string `json:"clientIp,omitempty"`
	// Action is the operation performed with resource type (e.g., "create workload", "delete node", "login auth")
	Action string `json:"action,omitempty"`
	// HttpMethod is the HTTP method used (POST, PUT, PATCH, DELETE)
	HttpMethod string `json:"httpMethod"`
	// RequestPath is the requested URL path
	RequestPath string `json:"requestPath"`
	// ResourceType is the type of resource being operated on
	ResourceType string `json:"resourceType,omitempty"`
	// RequestBody is the request body (sensitive data may be redacted)
	RequestBody string `json:"requestBody,omitempty"`
	// ResponseStatus is the HTTP response status code
	ResponseStatus int `json:"responseStatus"`
	// LatencyMs is the request processing time in milliseconds
	LatencyMs int64 `json:"latencyMs,omitempty"`
	// TraceId is the distributed tracing ID
	TraceId string `json:"traceId,omitempty"`
	// CreateTime is when the audit log was created (RFC3339 format)
	CreateTime string `json:"createTime"`
}

// CreateAuditLogRequest is one operation that another SaFE service performed for a user and that belongs in SaFE's
// audit log (POST /api/v1/auditlogs, internal services only). The fields are those of AuditLogItem; CreateTime is
// when the operation happened (RFC3339), not when it is reported.
type CreateAuditLogRequest struct {
	// UserId is the ID of the user who performed the operation (required)
	UserId string `json:"userId"`
	// UserName is the name of the user who performed the operation
	UserName string `json:"userName"`
	// UserType is the type of user (e.g., "default", "sso")
	UserType string `json:"userType"`
	// ClientIP is the IP address of the client
	ClientIP string `json:"clientIp"`
	// HttpMethod is the HTTP method of the operation (required, e.g., "POST")
	HttpMethod string `json:"httpMethod"`
	// RequestPath is the URL path of the operation (required, starts with "/")
	RequestPath string `json:"requestPath"`
	// ResourceType is the type of resource operated on (e.g., "apikey")
	ResourceType string `json:"resourceType"`
	// Action is the operation with its resource type (e.g., "create apikey")
	Action string `json:"action"`
	// RequestBody is the request body; credentials in it are redacted before it is stored
	RequestBody string `json:"requestBody"`
	// ResponseStatus is the HTTP status the operation was answered with (required)
	ResponseStatus int `json:"responseStatus"`
	// ResponseBody is the response body; credentials in it are redacted before it is stored
	ResponseBody string `json:"responseBody"`
	// LatencyMs is how long the operation took, in milliseconds (absent: not recorded)
	LatencyMs *int64 `json:"latencyMs"`
	// TraceId is the distributed tracing ID
	TraceId string `json:"traceId"`
	// CreateTime is when the operation happened (required, RFC3339)
	CreateTime string `json:"createTime"`
}
