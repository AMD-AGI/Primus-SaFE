package trace

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	tracerProvider *sdktrace.TracerProvider
)

// InitTracer initializes OpenTelemetry tracer
// Uses environment variables for configuration, compatible with OpenTelemetry standard environment variables
func InitTracer(serviceName string) error {
	log.Infof("Starting OpenTelemetry tracer initialization for service: %s", serviceName)
	ctx := context.Background()

	// Read OTLP endpoint
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// Fallback to legacy Jaeger environment variable (for compatibility)
		jaegerHost := os.Getenv("JAEGER_AGENT_HOST")
		if jaegerHost == "" {
			jaegerHost = "localhost"
		}
		// OTLP uses gRPC port 4317, not Jaeger Agent's 6831
		endpoint = fmt.Sprintf("%s:4317", jaegerHost)
		log.Infof("OTEL_EXPORTER_OTLP_ENDPOINT not set, using fallback endpoint: %s", endpoint)
	} else {
		log.Infof("Using OTEL_EXPORTER_OTLP_ENDPOINT: %s", endpoint)
	}

	// Read sampling configuration
	samplingRatio := 1.0 // Default: 100% sampling
	if ratioStr := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); ratioStr != "" {
		if ratio, err := strconv.ParseFloat(ratioStr, 64); err == nil {
			samplingRatio = ratio
			log.Infof("Using sampling ratio from OTEL_TRACES_SAMPLER_ARG: %.2f", samplingRatio)
		}
	} else if paramStr := os.Getenv("JAEGER_SAMPLER_PARAM"); paramStr != "" {
		// Compatible with legacy Jaeger environment variable
		if ratio, err := strconv.ParseFloat(paramStr, 64); err == nil {
			samplingRatio = ratio
			log.Infof("Using sampling ratio from JAEGER_SAMPLER_PARAM: %.2f", samplingRatio)
		}
	} else {
		log.Infof("Using default sampling ratio: %.2f", samplingRatio)
	}

	// Read sampler type
	samplerType := os.Getenv("OTEL_TRACES_SAMPLER")
	if samplerType == "" {
		samplerType = "traceidratio" // Default: use trace ID ratio sampling
		log.Infof("OTEL_TRACES_SAMPLER not set, using default: %s", samplerType)
	} else {
		log.Infof("Using sampler type: %s", samplerType)
	}

	// Create OTLP gRPC exporter
	log.Infof("Creating OTLP gRPC exporter, connecting to endpoint: %s", endpoint)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Errorf("Failed to create gRPC connection to %s: %v", endpoint, err)
		return fmt.Errorf("failed to create gRPC connection to %s: %w", endpoint, err)
	}
	log.Infof("Successfully established gRPC connection to %s", endpoint)

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		log.Errorf("Failed to create OTLP trace exporter: %v", err)
		return fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}
	log.Infof("Successfully created OTLP trace exporter")

	// Create resource (service identification information)
	log.Infof("Creating resource with service name: %s", serviceName)
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			// Service information
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
			// Environment information
			attribute.String("environment", getEnvOrDefault("ENVIRONMENT", "production")),
			attribute.String("cluster.name", getEnvOrDefault("DEFAULT_CLUSTER_NAME", "default")),
			// Deployment information
			attribute.String("k8s.namespace.name", getEnvOrDefault("POD_NAMESPACE", "default")),
			attribute.String("k8s.pod.name", getEnvOrDefault("POD_NAME", "unknown")),
			attribute.String("k8s.node.name", getEnvOrDefault("NODE_NAME", "unknown")),
		),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		log.Errorf("Failed to create resource: %v", err)
		return fmt.Errorf("failed to create resource: %w", err)
	}
	log.Infof("Successfully created resource")

	// Select sampler
	var sampler sdktrace.Sampler
	switch samplerType {
	case "always_on":
		sampler = sdktrace.AlwaysSample()
		log.Infof("Using AlwaysSample sampler")
	case "always_off":
		sampler = sdktrace.NeverSample()
		log.Infof("Using NeverSample sampler")
	case "traceidratio", "parentbased_traceidratio":
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplingRatio))
		log.Infof("Using ParentBased TraceIDRatio sampler with ratio: %.2f", samplingRatio)
	default:
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplingRatio))
		log.Infof("Using default ParentBased TraceIDRatio sampler with ratio: %.2f", samplingRatio)
	}

	// Create tracer provider
	log.Infof("Creating tracer provider with batch timeout: 5s, max batch size: 512")
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	log.Infof("Successfully created tracer provider")

	// Set global tracer provider
	otel.SetTracerProvider(tracerProvider)
	log.Infof("Set global tracer provider")

	// Set global propagator (for cross-service trace context propagation)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	log.Infof("Set global text map propagator (TraceContext + Baggage)")

	log.Infof("✓ OpenTelemetry tracer initialized successfully: service=%s, endpoint=%s, sampler=%s(%.2f)",
		serviceName, endpoint, samplerType, samplingRatio)
	log.Infof("✓ Trace export is now active and ready to send spans to %s", endpoint)

	return nil
}

// CloseTracer closes the tracer and flushes all pending spans
func CloseTracer() error {
	if tracerProvider != nil {
		log.Info("Shutting down OpenTelemetry tracer...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := tracerProvider.Shutdown(ctx)
		if err != nil {
			log.Errorf("Failed to shutdown tracer provider: %v", err)
			return err
		}
		log.Info("OpenTelemetry tracer shutdown successfully")
		return nil
	}
	log.Warn("Tracer provider is nil, nothing to shutdown")
	return nil
}

// StartSpan creates a new span from context
// If there is already a span in context, the new span will be its child span
func StartSpan(ctx context.Context, operationName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer("")
	return tracer.Start(ctx, operationName, opts...)
}

// StartSpanFromContext creates a new span from context (compatible with legacy API)
// Note: Return value order is reversed from StartSpan, for compatibility with Jaeger SDK
func StartSpanFromContext(ctx context.Context, operationName string, opts ...trace.SpanStartOption) (trace.Span, context.Context) {
	tracer := otel.Tracer("")
	newCtx, span := tracer.Start(ctx, operationName, opts...)
	return span, newCtx
}

// GetSpan gets the currently active span from context
func GetSpan(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// ContextWithSpan sets span into context (compatible with legacy API)
// Note: OpenTelemetry usually doesn't require manual setting, as StartSpan already returns a new context
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// FinishSpan ends a span
func FinishSpan(span trace.Span) {
	if span != nil {
		span.End()
	}
}

// FinishSpanFromContext gets span from context and ends it
func FinishSpanFromContext(ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	span.End()
}

// AddEvent adds an event to span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// SetAttributes sets span attributes
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// SetAttribute sets a single span attribute
func SetAttribute(ctx context.Context, key string, value interface{}) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(convertToAttribute(key, value))
	}
}

// RecordError records an error to span
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err, opts...)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetStatus sets span status
func SetStatus(ctx context.Context, code codes.Code, description string) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetStatus(code, description)
	}
}

// GetTraceID gets the current trace ID
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// GetSpanID gets the current span ID
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasSpanID() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// SpanFromContext gets span from context (compatible with legacy API)
// Returns span and a boolean indicating whether the span is valid
func SpanFromContext(ctx context.Context) (trace.Span, bool) {
	span := trace.SpanFromContext(ctx)
	// Check if span is valid (recording or has valid span context)
	if span != nil && span.SpanContext().IsValid() {
		return span, true
	}
	return span, false
}

// GetTraceIDAndSpanID gets trace ID and span ID from span (compatible with legacy API)
// Returns traceID, spanID and a boolean indicating whether the trace is valid
func GetTraceIDAndSpanID(span trace.Span) (string, string, bool) {
	if span == nil {
		return "", "", false
	}
	spanCtx := span.SpanContext()
	if !spanCtx.IsValid() {
		return "", "", false
	}
	return spanCtx.TraceID().String(), spanCtx.SpanID().String(), true
}

// convertToAttribute converts interface{} to attribute.KeyValue
func convertToAttribute(key string, value interface{}) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	case bool:
		return attribute.Bool(key, v)
	default:
		return attribute.String(key, fmt.Sprintf("%v", v))
	}
}

// getEnvOrDefault gets environment variable, returns default value if not exists
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
