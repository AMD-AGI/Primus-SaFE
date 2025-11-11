package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func HandleTracing() gin.HandlerFunc {
	return func(c *gin.Context) {
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
