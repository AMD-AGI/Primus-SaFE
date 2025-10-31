package trace

import (
	"bytes"
	"context"
	commonContext "github.com/AMD-AGI/primus-lens/core/pkg/context"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-lib/metrics"
	"net/http"
)

const (
	TraceKey = "_trace"
)

// InitTracer inits a jaeger tracer
func InitTracer(serviceName string) error {
	cfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans: false,
		},
	}

	// Initialize tracer with a logger and a metrics factory
	_, err := cfg.InitGlobalTracer(
		serviceName,
		jaegercfg.Logger(jaegerlog.NullLogger),
		jaegercfg.Metrics(metrics.NullFactory),
		jaegercfg.Reporter(jaeger.NewNullReporter()),
	)
	if err != nil {
		return err
	}
	return nil
}

// SpanFromContext gets span from context
func SpanFromContext(ctx context.Context) (opentracing.Span, bool) {
	valueI, has := commonContext.GetValue(ctx, TraceKey)
	if !has {
		return nil, false
	}
	value, transd := valueI.(opentracing.Span)
	if !transd {
		return nil, false
	}
	return value, true
}

// ContextWithSpan sets span into context
func ContextWithSpan(ctx context.Context, span opentracing.Span) context.Context {
	return commonContext.WithObject(ctx, TraceKey, span)
}

// StartSpan creates a new span as child of existed span context
func StartSpan(parentSpanContext opentracing.SpanContext, operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	tracer := opentracing.GlobalTracer()
	opts = append(opts, opentracing.ChildOf(parentSpanContext))
	return tracer.StartSpan(operationName, opts...)
}

// StartSpanFromContext creates a new span as child of existed span in context, and set the new span into context
func StartSpanFromContext(ctx context.Context, operationName string, opts ...opentracing.StartSpanOption) (opentracing.Span, context.Context) {
	tracer := opentracing.GlobalTracer()
	if parentSpan, ok := SpanFromContext(ctx); ok {
		opts = append(opts, opentracing.ChildOf(parentSpan.Context()))
	}
	span := tracer.StartSpan(operationName, opts...)
	return span, ContextWithSpan(ctx, span)
}

// FinishSpan finishes a span
func FinishSpan(span opentracing.Span) {
	span.Finish()
}

// FinishSpanFromContext finishes a span from context
func FinishSpanFromContext(ctx context.Context) {
	if span, ok := SpanFromContext(ctx); ok {
		span.Finish()
	}
}

// GetTraceIDAndSpanID returns trace id and span id from a span
func GetTraceIDAndSpanID(span opentracing.Span) (string, string, bool) {
	if spanContext, ok := span.Context().(jaeger.SpanContext); ok {
		return spanContext.TraceID().String(), spanContext.SpanID().String(), true
	}
	return "", "", false
}

// CreateSpanContextByTraceIDAndSpanID returns a span context fomr a trace id and a span id
func CreateSpanContextByTraceIDAndSpanID(traceIDStr, spanIDStr string) (opentracing.SpanContext, error) {
	traceID, err := jaeger.TraceIDFromString(traceIDStr)
	if err != nil {
		return jaeger.SpanContext{}, err
	}
	spanID, err := jaeger.SpanIDFromString(spanIDStr)
	if err != nil {
		return jaeger.SpanContext{}, err
	}
	return jaeger.NewSpanContext(traceID, spanID, jaeger.SpanID(0), true, nil), nil
}

// InjectHeader injects span from context into header
func InjectHeader(ctx context.Context, header http.Header) error {
	if span, ok := SpanFromContext(ctx); ok {
		tracer := opentracing.GlobalTracer()
		err := tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(header))
		if err != nil {
			return err
		}
	}
	return nil
}

// ExtractHeader extracts span from header into ctx
func ExtractHeader(ctx context.Context, header http.Header, operation string) (context.Context, error) {
	tracer := opentracing.GlobalTracer()
	spanCtx, err := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(header))
	if err != nil {
		return ctx, err
	}
	span := tracer.StartSpan(operation, opentracing.ChildOf(spanCtx))
	ctx = ContextWithSpan(ctx, span)
	return ctx, nil
}

// InjectMessage injects span from context into message
func InjectMessage(ctx context.Context, msg *[]byte) error {
	if span, ok := SpanFromContext(ctx); ok {
		tracer := opentracing.GlobalTracer()
		payload := bytes.Buffer{}
		err := tracer.Inject(span.Context(), opentracing.Binary, &payload)
		if err != nil {
			return err
		}
		*msg = append(payload.Bytes(), *msg...)
	}
	return nil
}

// ExtractMessage extracts span from message into ctx
func ExtractMessage(ctx context.Context, msg *[]byte, subject string) (context.Context, error) {
	tracer := opentracing.GlobalTracer()
	payload := bytes.NewBuffer(*msg)
	spanCtx, err := tracer.Extract(opentracing.Binary, payload)
	if err != nil {
		return ctx, err
	}
	span := tracer.StartSpan(subject, ext.SpanKindConsumer, opentracing.FollowsFrom(spanCtx))
	ext.MessageBusDestination.Set(span, subject)
	ctx = ContextWithSpan(ctx, span)
	*msg = payload.Bytes()
	return ctx, nil
}
