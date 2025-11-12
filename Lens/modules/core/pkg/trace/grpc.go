package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor for client-side Unary calls
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		span, ctx := StartSpanFromContext(ctx, "gRPC.Client."+method,
			trace.WithSpanKind(trace.SpanKindClient),
		)
		defer FinishSpan(span)

		span.SetAttributes(
			semconv.RPCMethod(method),
			semconv.RPCSystemGRPC,
			attribute.String("component", "grpc-client"),
		)

		// Inject span context to gRPC metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		propagator := otel.GetTextMapPropagator()
		carrier := &metadataCarrier{md: &md}
		propagator.Inject(ctx, carrier)
		ctx = metadata.NewOutgoingContext(ctx, md)

		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		return err
	}
}

// UnaryServerInterceptor for server-side Unary calls
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			propagator := otel.GetTextMapPropagator()
			carrier := &metadataCarrier{md: &md}
			ctx = propagator.Extract(ctx, carrier)
		}

		tracer := otel.Tracer("")
		ctx, span := tracer.Start(ctx, "gRPC.Server."+info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		span.SetAttributes(
			semconv.RPCMethod(info.FullMethod),
			semconv.RPCSystemGRPC,
			attribute.String("component", "grpc-server"),
		)

		resp, err := handler(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		return resp, err
	}
}

// StreamClientInterceptor for client-side Stream calls
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		span, ctx := StartSpanFromContext(ctx, "gRPC.ClientStream."+method,
			trace.WithSpanKind(trace.SpanKindClient),
		)

		span.SetAttributes(
			semconv.RPCMethod(method),
			semconv.RPCSystemGRPC,
			attribute.Bool("rpc.is_stream", true),
			attribute.String("component", "grpc-client-stream"),
		)

		// Inject span context to gRPC metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		propagator := otel.GetTextMapPropagator()
		carrier := &metadataCarrier{md: &md}
		propagator.Inject(ctx, carrier)
		ctx = metadata.NewOutgoingContext(ctx, md)

		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return nil, err
		}

		return &tracedClientStream{ClientStream: clientStream, span: span}, nil
	}
}

// StreamServerInterceptor for server-side Stream calls
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			propagator := otel.GetTextMapPropagator()
			carrier := &metadataCarrier{md: &md}
			ctx = propagator.Extract(ctx, carrier)
		}

		tracer := otel.Tracer("")
		ctx, span := tracer.Start(ctx, "gRPC.ServerStream."+info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		span.SetAttributes(
			semconv.RPCMethod(info.FullMethod),
			semconv.RPCSystemGRPC,
			attribute.Bool("rpc.is_stream", true),
			attribute.String("component", "grpc-server-stream"),
		)

		wrappedStream := &tracedServerStream{ServerStream: ss, ctx: ctx}

		err := handler(srv, wrappedStream)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		return err
	}
}

// tracedServerStream wraps grpc.ServerStream with tracing context
type tracedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *tracedServerStream) Context() context.Context {
	return s.ctx
}

// tracedClientStream wraps grpc.ClientStream with tracing span
type tracedClientStream struct {
	grpc.ClientStream
	span trace.Span
}

func (s *tracedClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	} else {
		s.span.SetStatus(codes.Ok, "")
	}
	s.span.End()
	return err
}

// metadataCarrier implements propagation.TextMapCarrier for gRPC metadata
type metadataCarrier struct {
	md *metadata.MD
}

func (m *metadataCarrier) Get(key string) string {
	values := (*m.md).Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (m *metadataCarrier) Set(key, val string) {
	(*m.md).Set(key, val)
}

func (m *metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(*m.md))
	for k := range *m.md {
		keys = append(keys, k)
	}
	return keys
}
