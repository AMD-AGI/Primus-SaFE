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
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
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

// Audit creates audit middleware for route groups or routes.
// GET requests are skipped automatically. Action is inferred from HTTP method if not provided.
// POST->create, DELETE->delete, PATCH->update, PUT->replace.
func Audit(resourceType string, action ...string) gin.HandlerFunc {
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
		klog.Infof("audit log buffer initialized with batch size %d, flush interval %v", auditBatchSize, auditFlushInterval)
	}

	// Determine if action is explicitly provided
	var explicitAction string
	if len(action) > 0 && action[0] != "" {
		explicitAction = action[0]
	}

	return func(c *gin.Context) {
		method := c.Request.Method

		// Skip GET requests - read operations should not be audited
		if method == http.MethodGet {
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

		finalAction := explicitAction
		if finalAction == "" {
			finalAction = inferAction(method)
		}
		// Combine action with resourceType (e.g., "create workload")
		// Skip combining for login/logout as they are standalone actions
		if resourceType != "" && finalAction != "login" && finalAction != "logout" {
			finalAction = finalAction + " " + resourceType
		}

		userId, _ := c.Get(common.UserId)
		userName, _ := c.Get(common.UserName)
		userType, _ := c.Get(common.UserType)

		userIdStr := toStringValue(userId)
		userNameStr := toStringValue(userName)
		userTypeStr := toStringValue(userType)

		// Query K8s for user info if userName or userType is empty (internal calls)
		if userNameStr == "" || userTypeStr == "" {
			if user := getUserFromK8s(c.Request.Context(), userIdStr); user != nil {
				if userNameStr == "" {
					userNameStr = v1.GetUserName(user)
				}
				if userTypeStr == "" {
					userTypeStr = string(user.Spec.Type)
				}
			}
		}

		traceId := c.Writer.Header().Get("X-Trace-Id")
		// Clear invalid traceId (all zeros when no tracing backend)
		if isInvalidTraceId(traceId) {
			traceId = ""
		}

		log := &dbclient.AuditLog{
			UserId:         userIdStr,
			UserName:       toNullString(userNameStr),
			UserType:       toNullString(userTypeStr),
			ClientIP:       toNullString(c.ClientIP()),
			HttpMethod:     method,
			RequestPath:    c.Request.URL.Path,
			ResourceType:   toNullString(resourceType),
			Action:         toNullString(finalAction),
			RequestBody:    toNullString(sanitizeBody(requestBody)),
			ResponseStatus: c.Writer.Status(),
			ResponseBody:   toNullString(sanitizeBody(truncateString(bodyWriter.body.String(), maxAuditBodySize))),
			LatencyMs:      sql.NullInt64{Int64: latencyMs, Valid: true},
			TraceId:        toNullString(traceId),
			CreateTime:     pq.NullTime{Time: time.Now().UTC(), Valid: true},
		}

		auditBuffer.send(log)
	}
}

// inferAction determines the action from HTTP method
func inferAction(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodDelete:
		return "delete"
	case http.MethodPatch:
		return "update"
	case http.MethodPut:
		return "replace"
	default:
		return strings.ToLower(method)
	}
}

// isInvalidTraceId checks if traceId is invalid (all zeros when no tracing backend)
func isInvalidTraceId(traceId string) bool {
	if traceId == "" {
		return true
	}
	for _, c := range traceId {
		if c != '0' {
			return false
		}
	}
	return true
}

// sanitizeBody removes sensitive information from request body
func sanitizeBody(body string) string {
	if body == "" {
		return ""
	}

	result := body

	// JSON format: "password": "value"
	jsonPatterns := []*regexp.Regexp{
		regexp.MustCompile(`"password"\s*:\s*"[^"]*"`),
		regexp.MustCompile(`"token"\s*:\s*"[^"]*"`),
		regexp.MustCompile(`"secret"\s*:\s*"[^"]*"`),
		regexp.MustCompile(`"apiKey"\s*:\s*"[^"]*"`),
		regexp.MustCompile(`"api_key"\s*:\s*"[^"]*"`),
	}
	for _, pattern := range jsonPatterns {
		result = pattern.ReplaceAllString(result, `"[REDACTED]"`)
	}

	// Form-urlencoded format: password=value or password=value&
	formPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(^|&)(password|token|secret|apiKey|api_key)=[^&]*`),
	}
	for _, pattern := range formPatterns {
		result = pattern.ReplaceAllString(result, `$1$2=[REDACTED]`)
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

// getUserFromK8s queries K8s to get user information by userId
func getUserFromK8s(ctx context.Context, userId string) *v1.User {
	internalAuth := authority.InternalAuthInstance()
	if internalAuth == nil {
		return nil
	}

	user := &v1.User{}
	if err := internalAuth.Get(ctx, client.ObjectKey{Name: userId}, user); err != nil {
		klog.ErrorS(err, "failed to get user from K8s for audit", "userId", userId)
		return nil
	}
	return user
}
