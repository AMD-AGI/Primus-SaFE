/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

const (
	// maxAuditBodySize is the maximum body size to capture for audit logs (8KB)
	maxAuditBodySize = 8192
	// auditBufferSize is the capacity of the audit log buffer channel
	auditBufferSize = 1000
	// auditBatchSize is the number of logs to batch before writing
	auditBatchSize = 50
	// auditFlushInterval is the interval to flush audit logs even if batch is not full
	auditFlushInterval = 5 * time.Second
)

// auditResponseWriter wraps gin.ResponseWriter to capture response body
type auditResponseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

// auditLogBuffer is a singleton buffer for batching audit logs
type auditLogBuffer struct {
	ch     chan *dbclient.AuditLog
	client *dbclient.Client
	once   sync.Once
}

var auditBuffer *auditLogBuffer

// initAuditBuffer initializes the audit log buffer and starts the background worker
func initAuditBuffer(client *dbclient.Client) *auditLogBuffer {
	buf := &auditLogBuffer{
		ch:     make(chan *dbclient.AuditLog, auditBufferSize),
		client: client,
	}
	buf.once.Do(func() {
		go buf.flushWorker()
	})
	return buf
}

// send adds an audit log to the buffer, returns false if buffer is full
func (b *auditLogBuffer) send(log *dbclient.AuditLog) bool {
	select {
	case b.ch <- log:
		return true
	default:
		// Buffer is full, log warning
		klog.Warning("audit log buffer full, dropping log",
			"userId", log.UserId,
			"method", log.HttpMethod,
			"path", log.RequestPath)
		return false
	}
}

// flushWorker is a background goroutine that batches and writes audit logs
func (b *auditLogBuffer) flushWorker() {
	ticker := time.NewTicker(auditFlushInterval)
	defer ticker.Stop()

	batch := make([]*dbclient.AuditLog, 0, auditBatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		b.writeBatch(batch)
		batch = batch[:0]
	}

	for {
		select {
		case log, ok := <-b.ch:
			if !ok {
				// Channel closed, flush remaining and exit
				flush()
				return
			}
			batch = append(batch, log)
			if len(batch) >= auditBatchSize {
				flush()
			}
		case <-ticker.C:
			// Periodic flush for low-traffic scenarios
			flush()
		}
	}
}

// writeBatch writes a batch of audit logs to the database using batch insert
func (b *auditLogBuffer) writeBatch(batch []*dbclient.AuditLog) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := b.client.BatchInsertAuditLogs(ctx, batch)
	if err != nil {
		klog.ErrorS(err, "failed to batch insert audit logs", "count", len(batch))
		// Fallback to individual inserts if batch fails
		for _, log := range batch {
			if err := b.client.InsertAuditLog(ctx, log); err != nil {
				klog.ErrorS(err, "failed to insert audit log",
					"userId", log.UserId,
					"method", log.HttpMethod,
					"path", log.RequestPath)
			}
		}
	} else {
		klog.V(4).Infof("batch inserted %d audit logs to database", len(batch))
	}
}

func (w *auditResponseWriter) Write(b []byte) (int, error) {
	if w.body.Len() < maxAuditBodySize {
		remaining := maxAuditBodySize - w.body.Len()
		if len(b) <= remaining {
			w.body.Write(b)
		} else {
			w.body.Write(b[:remaining])
		}
	}
	return w.ResponseWriter.Write(b)
}

// AuditLog creates a middleware that logs write operations (POST, PUT, PATCH, DELETE) to the database.
// It uses a buffered channel and background worker to batch writes for better performance.
func AuditLog() gin.HandlerFunc {
	if !commonconfig.IsDBEnable() {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	client := dbclient.NewClient()
	if client == nil {
		klog.Warning("audit middleware: database client not initialized")
		return func(c *gin.Context) {
			c.Next()
		}
	}

	// Initialize the audit buffer singleton (only once)
	if auditBuffer == nil {
		auditBuffer = initAuditBuffer(client)
		klog.Info("audit log buffer initialized with batch size", auditBatchSize, "flush interval", auditFlushInterval)
	}

	return func(c *gin.Context) {
		method := c.Request.Method
		if !isWriteOperation(method) {
			c.Next()
			return
		}

		// Skip login/logout - they have dedicated audit logging in handler
		if isAuthPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		startTime := time.Now()

		var requestBody string
		if c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				if len(bodyBytes) > maxAuditBodySize {
					requestBody = string(bodyBytes[:maxAuditBodySize]) + "...(truncated)"
				} else {
					requestBody = string(bodyBytes)
				}
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}

		bodyWriter := &auditResponseWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = bodyWriter

		c.Next()

		latencyMs := time.Since(startTime).Milliseconds()
		if latencyMs == 0 {
			latencyMs = 1 // Ensure minimum 1ms for display
		}

		resourceType := extractResourceType(c.Request.URL.Path)

		userId, _ := c.Get(common.UserId)
		userName, _ := c.Get(common.UserName)
		userType, _ := c.Get(common.UserType)

		// Ensure we have user identification for audit trail
		userIdStr := toStringValue(userId)
		userNameStr := toStringValue(userName)
		userTypeStr := toStringValue(userType)

		// Fallback: if userName is empty but userId exists, use userId as userName
		if userNameStr == "" && userIdStr != "" {
			userNameStr = userIdStr
		}
		// Fallback: if userId is empty, mark as anonymous (e.g., failed auth attempts)
		if userIdStr == "" {
			userIdStr = "anonymous"
			userNameStr = "anonymous"
			userTypeStr = "unknown"
		}

		traceId := c.Writer.Header().Get("X-Trace-Id")

		log := &dbclient.AuditLog{
			UserId:         userIdStr,
			UserName:       toNullString(userNameStr),
			UserType:       toNullString(userTypeStr),
			ClientIP:       toNullString(c.ClientIP()),
			HttpMethod:     method,
			RequestPath:    c.Request.URL.Path,
			ResourceType:   toNullString(resourceType),
			RequestBody:    toNullString(sanitizeBody(requestBody)),
			ResponseStatus: c.Writer.Status(),
			ResponseBody:   toNullString(sanitizeBody(truncateString(bodyWriter.body.String(), maxAuditBodySize))),
			LatencyMs:      sql.NullInt64{Int64: latencyMs, Valid: true},
			TraceId:        toNullString(traceId),
			CreateTime:     pq.NullTime{Time: time.Now().UTC(), Valid: true},
		}

		// Non-blocking send to buffer - this is very fast
		auditBuffer.send(log)
	}
}

// isWriteOperation checks if the HTTP method is a write operation
func isWriteOperation(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

// extractResourceType extracts resource type from the request path
// For example: /api/v1/workloads/my-workload -> workloads
// For example: /api/v1/cd/deployments/33/approve -> deployments
// For example: /api/v1/clusters/my-cluster/addons -> addons
func extractResourceType(path string) string {
	// Common patterns: /api/v1/{resource_type}/{resource_name}/...
	// Or with module prefix: /api/v1/{module}/{resource_type}/{resource_name}/...
	// Or nested resources: /api/v1/{parent_type}/{parent_name}/{nested_type}
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Skip api version prefix (e.g., "api/v1") and module prefix (e.g., "cd")
	startIdx := 0
	for i, part := range parts {
		if part == "api" || part == "v1" || part == "v2" || isModulePrefix(part) {
			startIdx = i + 1
			continue
		}
		break
	}

	if startIdx >= len(parts) {
		return ""
	}

	// Check for nested resource pattern: /{parent_type}/{parent_name}/{nested_type}
	// Look for nested resource types in the path (after position startIdx+1)
	for i := startIdx + 2; i < len(parts); i++ {
		if isNestedResourceType(parts[i]) {
			return parts[i]
		}
	}

	// Default: use first resource after api version
	return parts[startIdx]
}

// isModulePrefix checks if a string is a known module prefix
func isModulePrefix(s string) bool {
	modules := map[string]bool{
		"cd": true, // CD (Continuous Deployment) module
	}
	return modules[strings.ToLower(s)]
}

// isNestedResourceType checks if a string is a known nested resource type
// These are resources that appear as sub-resources under a parent resource
// For example: /api/v1/clusters/:name/addons -> addons is a nested resource
func isNestedResourceType(s string) bool {
	nestedTypes := map[string]bool{
		"addons": true, // Cluster addons: /api/v1/clusters/:name/addons
	}
	return nestedTypes[strings.ToLower(s)]
}

// isOperationKeyword checks if a string is a known operation keyword
func isOperationKeyword(s string) bool {
	operations := map[string]bool{
		"delete": true, "stop": true, "clone": true, "retry": true,
		"logs": true, "export": true, "verify": true, "status": true,
		"approve": true, "rollback": true, "description": true, // CD and publickey operations
	}
	return operations[strings.ToLower(s)]
}

// isAuthPath checks if the path is a login/logout path that has dedicated audit logging
func isAuthPath(path string) bool {
	return strings.HasSuffix(path, "/login") || strings.HasSuffix(path, "/logout")
}

// sanitizeBody removes sensitive information from request body
func sanitizeBody(body string) string {
	if body == "" {
		return ""
	}

	// Remove password fields
	sensitivePatterns := []*regexp.Regexp{
		regexp.MustCompile(`"password"\s*:\s*"[^"]*"`),
		regexp.MustCompile(`"token"\s*:\s*"[^"]*"`),
		regexp.MustCompile(`"secret"\s*:\s*"[^"]*"`),
		regexp.MustCompile(`"apiKey"\s*:\s*"[^"]*"`),
		regexp.MustCompile(`"api_key"\s*:\s*"[^"]*"`),
	}

	result := body
	for _, pattern := range sensitivePatterns {
		result = pattern.ReplaceAllString(result, `"[REDACTED]"`)
	}

	return result
}

// truncateString truncates a string to the specified maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

// toNullString converts an interface{} to sql.NullString
func toNullString(v interface{}) sql.NullString {
	if v == nil {
		return sql.NullString{Valid: false}
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// toStringValue converts an interface{} to string, returns empty string if nil
func toStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
