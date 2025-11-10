package middleware

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func HandleTracing() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		// Get trace ID and span ID for logging
		spanContext := span.SpanContext()
		traceID := spanContext.TraceID().String()
		spanID := spanContext.SpanID().String()

		log.Infof("🔍 [TRACE] Created span for %s, TraceID=%s, SpanID=%s",
			operationName, traceID, spanID)

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
			statusCode := c.Writer.Status()
			span.SetAttributes(semconv.HTTPStatusCode(statusCode))

			if statusCode >= 400 {
				span.SetStatus(codes.Error, "HTTP error")
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.End()
			log.Infof("🔍 [TRACE] Finished span for %s, TraceID=%s, SpanID=%s, Status=%d",
				operationName, traceID, spanID, statusCode)
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
