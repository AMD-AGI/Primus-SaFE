// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package trace

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

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
	"k8s.io/klog/v2"
)

var (
	tracerProvider *sdktrace.TracerProvider
	traceOptions   TraceOptions
)

// InitTracer initializes OpenTelemetry tracer with default options (error_only mode)
// Uses environment variables for configuration, compatible with OpenTelemetry standard environment variables
func InitTracer(serviceName string) error {
	return InitTracerWithOptions(serviceName, DefaultTraceOptions())
}

// InitTracerWithOptions initializes OpenTelemetry tracer with custom options
// Uses environment variables for configuration, compatible with OpenTelemetry standard environment variables
//
// Environment variables:
//   - OTEL_TRACING_MODE: "error_only" (default) - only exports traces when an error occurs
//   - OTEL_TRACING_MODE: "all" - exports all traces
func InitTracerWithOptions(serviceName string, opts TraceOptions) error {
	traceOptions = opts
	klog.Infof("Starting OpenTelemetry tracer initialization for service: %s, mode: error_only", serviceName)
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
		klog.Infof("OTEL_EXPORTER_OTLP_ENDPOINT not set, using fallback endpoint: %s", endpoint)
	} else {
		klog.Infof("Using OTEL_EXPORTER_OTLP_ENDPOINT: %s", endpoint)
	}

	// Determine tracing mode from environment variable
	mode := os.Getenv("OTEL_TRACING_MODE")
	if mode == "" {
		mode = "error_only"
	}

	// Determine sampling ratio for "all" mode (default 1.0 = 100%)
	samplingRatio := 1.0
	if ratioStr := os.Getenv("OTEL_SAMPLING_RATIO"); ratioStr != "" {
		if ratio, err := strconv.ParseFloat(ratioStr, 64); err == nil && ratio >= 0 && ratio <= 1 {
			samplingRatio = ratio
		}
	}
	klog.Infof("Trace mode: %s, sampling ratio: %.2f", mode, samplingRatio)

	// Create OTLP gRPC exporter
	klog.Infof("Creating OTLP gRPC exporter, connecting to endpoint: %s", endpoint)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		klog.Errorf("Failed to create gRPC connection to %s: %v", endpoint, err)
		return fmt.Errorf("failed to create gRPC connection to %s: %w", endpoint, err)
	}
	klog.Infof("Successfully established gRPC connection to %s", endpoint)

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		klog.Errorf("Failed to create OTLP trace exporter: %v", err)
		return fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}
	klog.Infof("Successfully created OTLP trace exporter")

	// Create resource (service identification information)
	klog.Infof("Creating resource with service name: %s", serviceName)
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
		klog.Errorf("Failed to create resource: %v", err)
		return fmt.Errorf("failed to create resource: %w", err)
	}
	klog.Infof("Successfully created resource")

	// Create tracer provider based on mode
	var providerOpts []sdktrace.TracerProviderOption
	providerOpts = append(providerOpts, sdktrace.WithResource(res))

	if mode == "all" {
		// all mode: use BatchSpanProcessor to export traces with configurable sampling ratio
		var sampler sdktrace.Sampler
		if samplingRatio >= 1.0 {
			sampler = sdktrace.AlwaysSample()
		} else if samplingRatio <= 0 {
			sampler = sdktrace.NeverSample()
		} else {
			sampler = sdktrace.TraceIDRatioBased(samplingRatio)
		}
		providerOpts = append(providerOpts,
			sdktrace.WithSampler(sampler),
			sdktrace.WithBatcher(exporter),
		)
		klog.Infof("Using BatchSpanProcessor (all mode, sampling ratio: %.2f)", samplingRatio)
	} else {
		// error_only mode (default): use ErrorOnlySpanProcessor with configurable sampling ratio
		providerOpts = append(providerOpts,
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithSpanProcessor(NewErrorOnlySpanProcessor(exporter, samplingRatio)),
		)
		klog.Infof("Using ErrorOnlySpanProcessor (error_only mode, sampling ratio: %.2f)", samplingRatio)
	}

	klog.Infof("Creating tracer provider")
	tracerProvider = sdktrace.NewTracerProvider(providerOpts...)
	klog.Infof("Successfully created tracer provider")

	// Set global tracer provider
	otel.SetTracerProvider(tracerProvider)
	klog.Infof("Set global tracer provider")

	// Set global propagator (for cross-service trace context propagation)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	klog.Infof("Set global text map propagator (TraceContext + Baggage)")

	klog.Infof("✓ OpenTelemetry tracer initialized successfully: service=%s, endpoint=%s, mode=%s",
		serviceName, endpoint, mode)
	klog.Infof("✓ Trace export is now active and ready to send spans to %s", endpoint)

	return nil
}

// GetTraceOptions returns the current trace options
func GetTraceOptions() TraceOptions {
	return traceOptions
}

// TraceOptionsFromConfig creates TraceOptions from configuration values
func TraceOptionsFromConfig(mode string, samplingRatio, errorSamplingRatio float64) TraceOptions {
	opts := DefaultTraceOptions()

	if mode == "all" {
		opts.Mode = TraceModeAlways
	} else if mode == "error_only" {
		opts.Mode = TraceModeErrorOnly
	}

	if samplingRatio >= 0 && samplingRatio <= 1 {
		opts.SamplingRatio = samplingRatio
	}

	if errorSamplingRatio >= 0 && errorSamplingRatio <= 1 {
		opts.ErrorSamplingRatio = errorSamplingRatio
	}

	return opts
}

// CloseTracer closes the tracer and flushes all pending spans
func CloseTracer() error {
	if tracerProvider != nil {
		klog.Info("Shutting down OpenTelemetry tracer...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := tracerProvider.Shutdown(ctx)
		if err != nil {
			klog.Errorf("Failed to shutdown tracer provider: %v", err)
			return err
		}
		klog.Info("OpenTelemetry tracer shutdown successfully")
		return nil
	}
	klog.Warning("Tracer provider is nil, nothing to shutdown")
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
