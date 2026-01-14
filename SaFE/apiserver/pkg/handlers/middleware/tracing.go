// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package middleware

import (
	"bytes"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// maxResponseBodySize is the maximum response body size to capture (4KB)
	maxResponseBodySize = 4096
)

// responseBodyWriter wraps gin.ResponseWriter to capture response body and inject headers
type responseBodyWriter struct {
	gin.ResponseWriter
	body           *bytes.Buffer
	traceId        string
	headerInjected bool
}

func (w *responseBodyWriter) WriteHeader(code int) {
	// Inject X-Trace-Id header for all responses (industry best practice)
	if !w.headerInjected && w.traceId != "" {
		w.Header().Set("X-Trace-Id", w.traceId)
		w.headerInjected = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	// Capture response body (up to maxResponseBodySize)
	if w.body.Len() < maxResponseBodySize {
		remaining := maxResponseBodySize - w.body.Len()
		if len(b) <= remaining {
			w.body.Write(b)
		} else {
			w.body.Write(b[:remaining])
		}
	}
	return w.ResponseWriter.Write(b)
}

// HandleTracing creates a tracing middleware based on the tracing.mode configuration.
// - "all": records all requests
// - "error_only" (default): only records failed requests (status >= 400)
func HandleTracing() gin.HandlerFunc {
	mode := config.GetTracingMode()
	if mode == "all" {
		return HandleTracingAll()
	}
	return HandleTracingErrorOnly()
}

// HandleTracingAll creates a tracing middleware that records all requests.
// Every request will have a span created with full details.
func HandleTracingAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip tracing if not enabled
		if !config.IsTracingEnable() {
			c.Next()
			return
		}

		// Record start time for duration calculation
		startTime := time.Now()

		ctx := c.Request.Context()

		// Extract trace context from HTTP headers
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, &httpHeaderCarrier{header: c.Request.Header})

		// Create span for this request
		operationName := c.Request.Method + " " + c.Request.URL.Path
		tracer := otel.Tracer("primus-safe-apiserver")
		ctx, span := tracer.Start(ctx, operationName,
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithTimestamp(startTime),
		)
		defer span.End()

		// Get traceId for header injection
		traceId := span.SpanContext().TraceID().String()

		// Wrap response writer to capture response body
		bodyWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
			traceId:        traceId,
		}
		c.Writer = bodyWriter

		// Update context in request
		c.Request = c.Request.WithContext(ctx)

		// Execute the request
		c.Next()

		// Calculate request duration
		duration := time.Since(startTime)
		statusCode := c.Writer.Status()

		// Set HTTP-related attributes for all requests
		span.SetAttributes(
			semconv.HTTPMethod(c.Request.Method),
			semconv.HTTPURL(c.Request.URL.String()),
			semconv.HTTPRoute(c.Request.URL.Path),
			semconv.HTTPTarget(c.Request.URL.Path),
			semconv.HTTPStatusCode(statusCode),
			attribute.String("component", "gin-http"),
			attribute.String("http.path", c.Request.URL.Path),
			attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
			attribute.Int64("http.duration_ns", duration.Nanoseconds()),
			attribute.String("trace.id", traceId),
		)

		// For error responses, capture additional details
		if statusCode >= 400 {
			responseBody := bodyWriter.body.String()
			if responseBody != "" {
				if len(responseBody) > maxResponseBodySize {
					responseBody = responseBody[:maxResponseBodySize] + "...(truncated)"
				}
				span.SetAttributes(attribute.String("http.response.body", responseBody))
			}
			span.SetStatus(codes.Error, "HTTP error: "+responseBody)

			if len(c.Errors) > 0 {
				for i, err := range c.Errors {
					span.SetAttributes(attribute.String("gin.error."+strconv.Itoa(i), err.Error()))
				}
				span.RecordError(c.Errors.Last())
			}
		} else {
			span.SetStatus(codes.Ok, "")
		}
	}
}

// HandleTracingErrorOnly creates a tracing middleware that only records failed requests.
// Spans are only created and exported when the HTTP status code is >= 400.
// The response body and traceId are captured for debugging purposes.
func HandleTracingErrorOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip tracing if not enabled
		if !config.IsTracingEnable() {
			c.Next()
			return
		}

		// Record start time for duration calculation
		startTime := time.Now()

		ctx := c.Request.Context()

		// Extract trace context from HTTP headers
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, &httpHeaderCarrier{header: c.Request.Header})

		// Pre-create span to get trace ID (we'll only export it if there's an error)
		operationName := c.Request.Method + " " + c.Request.URL.Path
		tracer := otel.Tracer("primus-safe-apiserver")
		ctx, span := tracer.Start(ctx, operationName,
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithTimestamp(startTime),
		)
		defer span.End()

		// Get traceId for potential header injection
		traceId := span.SpanContext().TraceID().String()

		// Wrap response writer to capture response body and inject X-Trace-Id header
		bodyWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
			traceId:        traceId,
		}
		c.Writer = bodyWriter

		// Update context in request
		c.Request = c.Request.WithContext(ctx)

		// Execute the request
		c.Next()

		// Get status code
		statusCode := c.Writer.Status()

		// Only record span details if request failed (status >= 400)
		if statusCode < 400 {
			// For successful requests, just return (span.End() called by defer)
			return
		}

		// Calculate request duration
		duration := time.Since(startTime)

		// Set HTTP-related attributes
		span.SetAttributes(
			semconv.HTTPMethod(c.Request.Method),
			semconv.HTTPURL(c.Request.URL.String()),
			semconv.HTTPRoute(c.Request.URL.Path),
			semconv.HTTPTarget(c.Request.URL.Path),
			semconv.HTTPStatusCode(statusCode),
			attribute.String("component", "gin-http"),
			attribute.String("http.path", c.Request.URL.Path),
			attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
			attribute.Int64("http.duration_ns", duration.Nanoseconds()),
			attribute.String("trace.id", traceId),
		)

		// Capture response body for error debugging
		responseBody := bodyWriter.body.String()
		if responseBody != "" {
			// Truncate if too long for display
			if len(responseBody) > maxResponseBodySize {
				responseBody = responseBody[:maxResponseBodySize] + "...(truncated)"
			}
			span.SetAttributes(attribute.String("http.response.body", responseBody))
		}

		// Record error status
		span.SetStatus(codes.Error, "HTTP error: "+responseBody)

		// Add error details if available from gin context
		if len(c.Errors) > 0 {
			for i, err := range c.Errors {
				span.SetAttributes(attribute.String("gin.error."+strconv.Itoa(i), err.Error()))
			}
			span.RecordError(c.Errors.Last())
		}
	}
}

// httpHeaderCarrier implements propagation.TextMapCarrier for HTTP headers
type httpHeaderCarrier struct {
	header http.Header
}

func (h *httpHeaderCarrier) Get(key string) string {
	return h.header.Get(key)
}

func (h *httpHeaderCarrier) Set(key, val string) {
	h.header.Set(key, val)
}

func (h *httpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h.header))
	for k := range h.header {
		keys = append(keys, k)
	}
	return keys
}
