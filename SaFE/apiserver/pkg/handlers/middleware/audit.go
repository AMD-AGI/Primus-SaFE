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

// writeBatch writes a batch of audit logs to the database
func (b *auditLogBuffer) writeBatch(batch []*dbclient.AuditLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, log := range batch {
		// TODO: If your DB supports batch insert, use it here for better performance
		err := b.client.InsertAuditLog(ctx, log)
		if err != nil {
			klog.ErrorS(err, "failed to insert audit log",
				"userId", log.UserId,
				"method", log.HttpMethod,
				"path", log.RequestPath)
		}
	}
	if len(batch) > 0 {
		klog.V(4).Infof("flushed %d audit logs to database", len(batch))
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
// This is a common industry pattern that balances reliability with performance.
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

		resourceType, resourceName := extractResourceInfo(c.Request.URL.Path)

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
			ResourceName:   toNullString(resourceName),
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

// extractResourceInfo extracts resource type and name from the request path
// For example: /api/v1/workloads/my-workload -> (workloads, my-workload)
func extractResourceInfo(path string) (string, string) {
	// Common patterns: /api/v1/{resource_type}/{resource_name}/...
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Skip api version prefix (e.g., "api/v1")
	startIdx := 0
	for i, part := range parts {
		if part == "api" || part == "v1" || part == "v2" {
			startIdx = i + 1
			continue
		}
		break
	}

	if startIdx >= len(parts) {
		return "", ""
	}

	resourceType := parts[startIdx]
	resourceName := ""
	if startIdx+1 < len(parts) {
		// The next part could be resource name or another operation
		potentialName := parts[startIdx+1]
		// Skip if it looks like an operation (e.g., "delete", "stop", "clone")
		if !isOperationKeyword(potentialName) {
			resourceName = potentialName
		}
	}

	return resourceType, resourceName
}

// isOperationKeyword checks if a string is a known operation keyword
func isOperationKeyword(s string) bool {
	operations := map[string]bool{
		"delete": true, "stop": true, "clone": true, "retry": true,
		"logs": true, "export": true, "verify": true, "status": true,
	}
	return operations[strings.ToLower(s)]
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
