package trace

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// TestMetadataCarrier_GetSetKeys tests metadataCarrier operations
func TestMetadataCarrier_GetSetKeys(t *testing.T) {
	md := metadata.New(map[string]string{
		"key1": "value1",
		"key2": "value2",
	})
	carrier := &metadataCarrier{md: &md}
	
	// Test Get
	value := carrier.Get("key1")
	assert.Equal(t, "value1", value)
	
	value = carrier.Get("non-existent")
	assert.Empty(t, value)
	
	// Test Set
	carrier.Set("key3", "value3")
	assert.Equal(t, "value3", carrier.Get("key3"))
	
	// Test Keys
	keys := carrier.Keys()
	assert.Contains(t, keys, "key1")
	assert.Contains(t, keys, "key2")
	assert.Contains(t, keys, "key3")
	assert.Len(t, keys, 3)
}

// TestMetadataCarrier_EmptyMetadata tests metadataCarrier with empty metadata
func TestMetadataCarrier_EmptyMetadata(t *testing.T) {
	md := metadata.New(nil)
	carrier := &metadataCarrier{md: &md}
	
	// Get on empty metadata
	value := carrier.Get("any-key")
	assert.Empty(t, value)
	
	// Set on empty metadata
	carrier.Set("new-key", "new-value")
	assert.Equal(t, "new-value", carrier.Get("new-key"))
	
	// Keys on empty metadata
	keys := carrier.Keys()
	assert.Len(t, keys, 1)
	assert.Contains(t, keys, "new-key")
}

// TestMetadataCarrier_MultipleValues tests metadataCarrier with multiple values
func TestMetadataCarrier_MultipleValues(t *testing.T) {
	md := metadata.MD{
		"key1": []string{"value1", "value2", "value3"},
	}
	carrier := &metadataCarrier{md: &md}
	
	// Get should return first value
	value := carrier.Get("key1")
	assert.Equal(t, "value1", value)
	
	// Set should replace all values
	carrier.Set("key1", "new-value")
	values := (*carrier.md).Get("key1")
	assert.Len(t, values, 1)
	assert.Equal(t, "new-value", values[0])
}

// TestUnaryClientInterceptor tests UnaryClientInterceptor
func TestUnaryClientInterceptor(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := UnaryClientInterceptor()
	require.NotNil(t, interceptor)
	
	ctx := context.Background()
	method := "/test.Service/TestMethod"
	req := "test-request"
	var reply string
	
	// Mock invoker
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		// Verify metadata is injected
		md, ok := metadata.FromOutgoingContext(ctx)
		assert.True(t, ok)
		assert.NotNil(t, md)
		
		return nil
	}
	
	err := interceptor(ctx, method, req, &reply, nil, invoker)
	assert.NoError(t, err)
}

// TestUnaryClientInterceptor_WithError tests UnaryClientInterceptor with error
func TestUnaryClientInterceptor_WithError(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := UnaryClientInterceptor()
	
	ctx := context.Background()
	method := "/test.Service/ErrorMethod"
	
	expectedErr := errors.New("rpc error")
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return expectedErr
	}
	
	err := interceptor(ctx, method, nil, nil, nil, invoker)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// TestUnaryClientInterceptor_WithExistingMetadata tests with existing metadata
func TestUnaryClientInterceptor_WithExistingMetadata(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := UnaryClientInterceptor()
	
	// Create context with existing metadata
	md := metadata.New(map[string]string{"existing-key": "existing-value"})
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		assert.True(t, ok)
		// Should preserve existing metadata
		assert.Contains(t, md.Get("existing-key"), "existing-value")
		return nil
	}
	
	err := interceptor(ctx, "/test.Service/Method", nil, nil, nil, invoker)
	assert.NoError(t, err)
}

// TestUnaryServerInterceptor tests UnaryServerInterceptor
func TestUnaryServerInterceptor(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := UnaryServerInterceptor()
	require.NotNil(t, interceptor)
	
	ctx := context.Background()
	req := "test-request"
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/TestMethod",
	}
	
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		// Verify span is in context
		span, ok := SpanFromContext(ctx)
		assert.True(t, ok)
		assert.NotNil(t, span)
		
		return "test-response", nil
	}
	
	resp, err := interceptor(ctx, req, info, handler)
	assert.NoError(t, err)
	assert.Equal(t, "test-response", resp)
}

// TestUnaryServerInterceptor_WithError tests UnaryServerInterceptor with error
func TestUnaryServerInterceptor_WithError(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := UnaryServerInterceptor()
	
	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/ErrorMethod",
	}
	
	expectedErr := errors.New("handler error")
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, expectedErr
	}
	
	resp, err := interceptor(ctx, nil, info, handler)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Nil(t, resp)
}

// TestUnaryServerInterceptor_WithIncomingMetadata tests with incoming metadata
func TestUnaryServerInterceptor_WithIncomingMetadata(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := UnaryServerInterceptor()
	
	// Create context with incoming metadata
	md := metadata.New(map[string]string{"client-key": "client-value"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}
	
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		// Context should have span
		span, ok := SpanFromContext(ctx)
		assert.True(t, ok)
		assert.NotNil(t, span)
		
		return "response", nil
	}
	
	_, err := interceptor(ctx, nil, info, handler)
	assert.NoError(t, err)
}

// TestStreamClientInterceptor tests StreamClientInterceptor
func TestStreamClientInterceptor(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := StreamClientInterceptor()
	require.NotNil(t, interceptor)
	
	ctx := context.Background()
	desc := &grpc.StreamDesc{
		StreamName:    "TestStream",
		ClientStreams: true,
		ServerStreams: true,
	}
	method := "/test.Service/StreamMethod"
	
	mockClientStream := &mockClientStream{}
	
	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		// Verify metadata is injected
		md, ok := metadata.FromOutgoingContext(ctx)
		assert.True(t, ok)
		assert.NotNil(t, md)
		
		return mockClientStream, nil
	}
	
	stream, err := interceptor(ctx, desc, nil, method, streamer)
	assert.NoError(t, err)
	assert.NotNil(t, stream)
	
	// Verify it's a tracedClientStream
	tracedStream, ok := stream.(*tracedClientStream)
	assert.True(t, ok)
	assert.NotNil(t, tracedStream.span)
}

// TestStreamClientInterceptor_WithError tests StreamClientInterceptor with error
func TestStreamClientInterceptor_WithError(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := StreamClientInterceptor()
	
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	method := "/test.Service/ErrorStream"
	
	expectedErr := errors.New("stream error")
	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, expectedErr
	}
	
	stream, err := interceptor(ctx, desc, nil, method, streamer)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Nil(t, stream)
}

// TestStreamServerInterceptor tests StreamServerInterceptor
func TestStreamServerInterceptor(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := StreamServerInterceptor()
	require.NotNil(t, interceptor)
	
	ctx := context.Background()
	mockServerStream := &mockServerStream{ctx: ctx}
	info := &grpc.StreamServerInfo{
		FullMethod:     "/test.Service/StreamMethod",
		IsClientStream: true,
		IsServerStream: true,
	}
	
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		// Verify stream has traced context
		ctx := stream.Context()
		span, ok := SpanFromContext(ctx)
		assert.True(t, ok)
		assert.NotNil(t, span)
		
		return nil
	}
	
	err := interceptor(nil, mockServerStream, info, handler)
	assert.NoError(t, err)
}

// TestStreamServerInterceptor_WithError tests StreamServerInterceptor with error
func TestStreamServerInterceptor_WithError(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := StreamServerInterceptor()
	
	ctx := context.Background()
	mockServerStream := &mockServerStream{ctx: ctx}
	info := &grpc.StreamServerInfo{
		FullMethod: "/test.Service/ErrorStream",
	}
	
	expectedErr := errors.New("handler error")
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return expectedErr
	}
	
	err := interceptor(nil, mockServerStream, info, handler)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// TestTracedClientStream_CloseSend tests tracedClientStream.CloseSend
func TestTracedClientStream_CloseSend(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	_, span := StartSpan(context.Background(), "test")
	
	tests := []struct {
		name        string
		mockStream  *mockClientStream
		expectError bool
	}{
		{
			name:        "successful close",
			mockStream:  &mockClientStream{closeErr: nil},
			expectError: false,
		},
		{
			name:        "close with error",
			mockStream:  &mockClientStream{closeErr: errors.New("close error")},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, testSpan := StartSpan(context.Background(), "test")
			tracedStream := &tracedClientStream{
				ClientStream: tt.mockStream,
				span:         testSpan,
			}
			
			err := tracedStream.CloseSend()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
	
	span.End()
}

// TestTracedServerStream_Context tests tracedServerStream.Context
func TestTracedServerStream_Context(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	ctx, span := StartSpan(context.Background(), "test")
	defer span.End()
	
	mockStream := &mockServerStream{ctx: context.Background()}
	tracedStream := &tracedServerStream{
		ServerStream: mockStream,
		ctx:          ctx,
	}
	
	resultCtx := tracedStream.Context()
	assert.Equal(t, ctx, resultCtx)
	
	// Verify span is in context
	retrievedSpan, ok := SpanFromContext(resultCtx)
	assert.True(t, ok)
	assert.Equal(t, span, retrievedSpan)
}

// Mock implementations for testing

type mockClientStream struct {
	grpc.ClientStream
	closeErr error
}

func (m *mockClientStream) CloseSend() error {
	return m.closeErr
}

func (m *mockClientStream) SendMsg(msg interface{}) error {
	return nil
}

func (m *mockClientStream) RecvMsg(msg interface{}) error {
	return nil
}

func (m *mockClientStream) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

func (m *mockClientStream) Trailer() metadata.MD {
	return metadata.MD{}
}

func (m *mockClientStream) Context() context.Context {
	return context.Background()
}

type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func (m *mockServerStream) SendMsg(msg interface{}) error {
	return nil
}

func (m *mockServerStream) RecvMsg(msg interface{}) error {
	return nil
}

func (m *mockServerStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockServerStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockServerStream) SetTrailer(metadata.MD) {
}

// BenchmarkMetadataCarrier_Get benchmarks metadataCarrier Get operation
func BenchmarkMetadataCarrier_Get(b *testing.B) {
	md := metadata.New(map[string]string{
		"key1": "value1",
		"key2": "value2",
	})
	carrier := &metadataCarrier{md: &md}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = carrier.Get("key1")
	}
}

// BenchmarkMetadataCarrier_Set benchmarks metadataCarrier Set operation
func BenchmarkMetadataCarrier_Set(b *testing.B) {
	md := metadata.New(nil)
	carrier := &metadataCarrier{md: &md}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		carrier.Set("key", "value")
	}
}

// BenchmarkUnaryClientInterceptor benchmarks UnaryClientInterceptor
func BenchmarkUnaryClientInterceptor(b *testing.B) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := UnaryClientInterceptor()
	ctx := context.Background()
	
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return nil
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = interceptor(ctx, "/test.Service/Method", nil, nil, nil, invoker)
	}
}

// BenchmarkUnaryServerInterceptor benchmarks UnaryServerInterceptor
func BenchmarkUnaryServerInterceptor(b *testing.B) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())
	
	interceptor := UnaryServerInterceptor()
	ctx := context.Background()
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}
	
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = interceptor(ctx, nil, info, handler)
	}
}

