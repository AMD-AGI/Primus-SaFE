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

// InitTracer 初始化 OpenTelemetry tracer
// 使用环境变量配置，兼容 OpenTelemetry 标准环境变量
func InitTracer(serviceName string) error {
	ctx := context.Background()

	// 读取 OTLP endpoint
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// 回退到旧的 Jaeger 环境变量（兼容性）
		jaegerHost := os.Getenv("JAEGER_AGENT_HOST")
		if jaegerHost == "" {
			jaegerHost = "localhost"
		}
		// OTLP 使用 gRPC 4317 端口，而不是 Jaeger Agent 的 6831
		endpoint = fmt.Sprintf("%s:4317", jaegerHost)
	}

	// 读取采样配置
	samplingRatio := 1.0 // 默认 100% 采样
	if ratioStr := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); ratioStr != "" {
		if ratio, err := strconv.ParseFloat(ratioStr, 64); err == nil {
			samplingRatio = ratio
		}
	} else if paramStr := os.Getenv("JAEGER_SAMPLER_PARAM"); paramStr != "" {
		// 兼容旧的 Jaeger 环境变量
		if ratio, err := strconv.ParseFloat(paramStr, 64); err == nil {
			samplingRatio = ratio
		}
	}

	// 读取采样器类型
	samplerType := os.Getenv("OTEL_TRACES_SAMPLER")
	if samplerType == "" {
		samplerType = "traceidratio" // 默认使用 trace ID ratio 采样
	}

	// 创建 OTLP gRPC exporter
	log.Infof("Connecting to OTLP endpoint: %s", endpoint)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to create gRPC connection to %s: %w", endpoint, err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// 创建 resource（服务标识信息）
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			// 服务信息
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
			// 环境信息
			attribute.String("environment", getEnvOrDefault("ENVIRONMENT", "production")),
			attribute.String("cluster.name", getEnvOrDefault("DEFAULT_CLUSTER_NAME", "default")),
			// 部署信息
			attribute.String("k8s.namespace.name", getEnvOrDefault("POD_NAMESPACE", "default")),
			attribute.String("k8s.pod.name", getEnvOrDefault("POD_NAME", "unknown")),
			attribute.String("k8s.node.name", getEnvOrDefault("NODE_NAME", "unknown")),
		),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// 选择采样器
	var sampler sdktrace.Sampler
	switch samplerType {
	case "always_on":
		sampler = sdktrace.AlwaysSample()
	case "always_off":
		sampler = sdktrace.NeverSample()
	case "traceidratio", "parentbased_traceidratio":
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplingRatio))
	default:
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplingRatio))
	}

	// 创建 tracer provider
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// 设置全局 tracer provider
	otel.SetTracerProvider(tracerProvider)

	// 设置全局 propagator（用于跨服务传播 trace context）
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Infof("OpenTelemetry tracer initialized: service=%s, endpoint=%s, sampler=%s(%.2f)",
		serviceName, endpoint, samplerType, samplingRatio)

	return nil
}

// CloseTracer 关闭 tracer 并刷新所有挂起的 spans
func CloseTracer() error {
	if tracerProvider != nil {
		log.Info("Shutting down OpenTelemetry tracer...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return tracerProvider.Shutdown(ctx)
	}
	return nil
}

// StartSpan 从 context 创建一个新的 span
// 如果 context 中已有 span，新 span 将作为其子 span
func StartSpan(ctx context.Context, operationName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer("")
	return tracer.Start(ctx, operationName, opts...)
}

// StartSpanFromContext 从 context 创建一个新的 span（兼容旧 API）
// 注意：返回值顺序与 StartSpan 相反，用于兼容 Jaeger SDK
func StartSpanFromContext(ctx context.Context, operationName string, opts ...trace.SpanStartOption) (trace.Span, context.Context) {
	tracer := otel.Tracer("")
	newCtx, span := tracer.Start(ctx, operationName, opts...)
	return span, newCtx
}

// GetSpan 从 context 获取当前活跃的 span
func GetSpan(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// ContextWithSpan 将 span 设置到 context 中（兼容旧 API）
// 注意：OpenTelemetry 通常不需要手动设置，因为 StartSpan 已经返回了新的 context
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// FinishSpan 结束一个 span
func FinishSpan(span trace.Span) {
	if span != nil {
		span.End()
	}
}

// FinishSpanFromContext 从 context 中获取 span 并结束它
func FinishSpanFromContext(ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	span.End()
}

// AddEvent 向 span 添加一个事件
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// SetAttributes 设置 span 属性
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// SetAttribute 设置单个 span 属性
func SetAttribute(ctx context.Context, key string, value interface{}) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(convertToAttribute(key, value))
	}
}

// RecordError 记录错误到 span
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err, opts...)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetStatus 设置 span 状态
func SetStatus(ctx context.Context, code codes.Code, description string) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetStatus(code, description)
	}
}

// GetTraceID 获取当前 trace ID
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// GetSpanID 获取当前 span ID
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasSpanID() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// SpanFromContext 从 context 获取 span（兼容旧 API）
// 返回 span 和一个布尔值表示是否为有效的 span
func SpanFromContext(ctx context.Context) (trace.Span, bool) {
	span := trace.SpanFromContext(ctx)
	// 检查 span 是否有效（正在记录或有有效的 span context）
	if span != nil && span.SpanContext().IsValid() {
		return span, true
	}
	return span, false
}

// GetTraceIDAndSpanID 从 span 获取 trace ID 和 span ID（兼容旧 API）
// 返回 traceID, spanID 和一个布尔值表示是否为有效的 trace
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

// convertToAttribute 将 interface{} 转换为 attribute.KeyValue
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

// getEnvOrDefault 获取环境变量，如果不存在则返回默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
