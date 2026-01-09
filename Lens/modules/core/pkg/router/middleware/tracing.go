// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package middleware

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// tracingSampleRateKey is used to store the sample rate in gin.Context
	tracingSampleRateKey = "tracing_sample_rate"
)

// WithTracingRate sets a custom tracing sample rate for specific endpoints
// Sample rate range: 0.0 - 1.0, where 1.0 means 100% sampling, 0.1 means 10% sampling
// Usage example: group.POST("wandb/batch", middleware.WithTracingRate(0.1), logs.ReceiveWandBBatch)
func WithTracingRate(rate float64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Ensure sample rate is within valid range [0.0, 1.0]
		if rate < 0.0 {
			rate = 0.0
		}
		if rate > 1.0 {
			rate = 1.0
		}
		c.Set(tracingSampleRateKey, rate)
		c.Next()
	}
}

// shouldSample determines whether to sample based on the sample rate
func shouldSample(sampleRate float64) bool {
	if sampleRate >= 1.0 {
		return true
	}
	if sampleRate <= 0.0 {
		return false
	}
	return rand.Float64() < sampleRate
}

func HandleTracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if custom sample rate is set
		var shouldTrace bool = true
		if rateValue, exists := c.Get(tracingSampleRateKey); exists {
			if rate, ok := rateValue.(float64); ok {
				shouldTrace = shouldSample(rate)
				// Add sample rate info to response headers (optional, for debugging)
				c.Header("X-Trace-Sample-Rate", strconv.FormatFloat(rate, 'f', 2, 64))
				c.Header("X-Trace-Sampled", strconv.FormatBool(shouldTrace))
			}
		}

		// If sampling decision is to skip, just continue without tracing
		if !shouldTrace {
			c.Next()
			return
		}

		// Record start time for duration calculation
		startTime := time.Now()

		ctx := c.Request.Context()

		// Extract trace context from HTTP headers
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, &httpHeaderCarrier{header: c.Request.Header})

		// Create span
		operationName := c.Request.Method + " " + c.Request.URL.Path
		tracer := otel.Tracer("")
		ctx, span := tracer.Start(ctx, operationName,
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		)

		// Set HTTP-related attributes
		span.SetAttributes(
			semconv.HTTPMethod(c.Request.Method),
			semconv.HTTPURL(c.Request.URL.String()),
			semconv.HTTPRoute(c.Request.URL.Path),
			semconv.HTTPTarget(c.Request.URL.Path),
			attribute.String("component", "gin-http"),
			attribute.String("http.path", c.Request.URL.Path),
		)

		// Set status code and finish span when request completes
		defer func() {
			// Calculate request duration
			duration := time.Since(startTime)

			statusCode := c.Writer.Status()
			span.SetAttributes(
				semconv.HTTPStatusCode(statusCode),
				attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
				attribute.Int64("http.duration_ns", duration.Nanoseconds()),
			)

			if statusCode >= 400 {
				span.SetStatus(codes.Error, "HTTP error")
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.End()
		}()

		// Update context in request
		c.Request = c.Request.WithContext(ctx)

		c.Next()
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
