package trace

import (
	"context"

	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor 用于客户端 Unary 调用
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		span, ctx := StartSpanFromContext(ctx, "gRPC.Client."+method)
		defer FinishSpan(span)

		span.SetTag("rpc.method", method)
		span.SetTag("rpc.system", "grpc")
		ext.SpanKindRPCClient.Set(span)
		ext.Component.Set(span, "grpc-client")

		// Inject span context to gRPC metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		tracer := opentracing.GlobalTracer()
		carrier := &metadataTextMap{md}
		err := tracer.Inject(span.Context(), opentracing.TextMap, carrier)
		if err != nil {
			log.Warnf("Failed to inject span to gRPC metadata: %v", err)
		}
		ctx = metadata.NewOutgoingContext(ctx, md)

		err = invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			span.SetTag("error", true)
			span.LogKV("error.message", err.Error())
			ext.Error.Set(span, true)
		}
		return err
	}
}

// UnaryServerInterceptor 用于服务端 Unary 调用
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		var span opentracing.Span
		tracer := opentracing.GlobalTracer()

		if ok {
			carrier := &metadataTextMap{md}
			spanCtx, err := tracer.Extract(opentracing.TextMap, carrier)
			if err == nil {
				span = tracer.StartSpan("gRPC.Server."+info.FullMethod, opentracing.ChildOf(spanCtx))
			} else {
				span = tracer.StartSpan("gRPC.Server." + info.FullMethod)
			}
		} else {
			span = tracer.StartSpan("gRPC.Server." + info.FullMethod)
		}
		defer span.Finish()

		span.SetTag("rpc.method", info.FullMethod)
		span.SetTag("rpc.system", "grpc")
		ext.SpanKindRPCServer.Set(span)
		ext.Component.Set(span, "grpc-server")

		ctx = ContextWithSpan(ctx, span)

		resp, err := handler(ctx, req)
		if err != nil {
			span.SetTag("error", true)
			span.LogKV("error.message", err.Error())
			ext.Error.Set(span, true)
		}
		return resp, err
	}
}

// StreamClientInterceptor 用于客户端 Stream 调用
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		span, ctx := StartSpanFromContext(ctx, "gRPC.ClientStream."+method)

		span.SetTag("rpc.method", method)
		span.SetTag("rpc.system", "grpc")
		span.SetTag("rpc.is_stream", true)
		ext.SpanKindRPCClient.Set(span)
		ext.Component.Set(span, "grpc-client-stream")

		// Inject span context to gRPC metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		tracer := opentracing.GlobalTracer()
		carrier := &metadataTextMap{md}
		err := tracer.Inject(span.Context(), opentracing.TextMap, carrier)
		if err != nil {
			log.Warnf("Failed to inject span to gRPC metadata: %v", err)
		}
		ctx = metadata.NewOutgoingContext(ctx, md)

		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.SetTag("error", true)
			span.LogKV("error.message", err.Error())
			ext.Error.Set(span, true)
			span.Finish()
			return nil, err
		}

		return &tracedClientStream{ClientStream: clientStream, span: span}, nil
	}
}

// StreamServerInterceptor 用于服务端 Stream 调用
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		var span opentracing.Span
		tracer := opentracing.GlobalTracer()

		if ok {
			carrier := &metadataTextMap{md}
			spanCtx, err := tracer.Extract(opentracing.TextMap, carrier)
			if err == nil {
				span = tracer.StartSpan("gRPC.ServerStream."+info.FullMethod, opentracing.ChildOf(spanCtx))
			} else {
				span = tracer.StartSpan("gRPC.ServerStream." + info.FullMethod)
			}
		} else {
			span = tracer.StartSpan("gRPC.ServerStream." + info.FullMethod)
		}
		defer span.Finish()

		span.SetTag("rpc.method", info.FullMethod)
		span.SetTag("rpc.system", "grpc")
		span.SetTag("rpc.is_stream", true)
		ext.SpanKindRPCServer.Set(span)
		ext.Component.Set(span, "grpc-server-stream")

		ctx = ContextWithSpan(ctx, span)
		wrappedStream := &tracedServerStream{ServerStream: ss, ctx: ctx}

		err := handler(srv, wrappedStream)
		if err != nil {
			span.SetTag("error", true)
			span.LogKV("error.message", err.Error())
			ext.Error.Set(span, true)
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
	span opentracing.Span
}

func (s *tracedClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.span.SetTag("error", true)
		s.span.LogKV("error.message", err.Error())
		ext.Error.Set(s.span, true)
	}
	s.span.Finish()
	return err
}

// metadataTextMap implements opentracing.TextMapReader and opentracing.TextMapWriter
// for gRPC metadata
type metadataTextMap struct {
	metadata.MD
}

func (m *metadataTextMap) Set(key, val string) {
	m.MD[key] = append(m.MD[key], val)
}

func (m *metadataTextMap) ForeachKey(handler func(key, val string) error) error {
	for k, vals := range m.MD {
		for _, v := range vals {
			if err := handler(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
