// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMeta tests the Meta struct
func TestMeta(t *testing.T) {
	meta := Meta{
		Code:    CodeSuccess,
		Message: "OK",
	}

	assert.Equal(t, 2000, meta.Code)
	assert.Equal(t, "OK", meta.Message)
}

// TestMeta_JSONMarshal tests JSON marshaling of Meta
func TestMeta_JSONMarshal(t *testing.T) {
	meta := Meta{
		Code:    CodeSuccess,
		Message: "Test message",
	}

	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var decoded Meta
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, meta.Code, decoded.Code)
	assert.Equal(t, meta.Message, decoded.Message)
}

// TestTrace tests the Trace struct
func TestTrace(t *testing.T) {
	trace := Trace{
		TraceId: "trace-123",
		SpanId:  "span-456",
	}

	assert.Equal(t, "trace-123", trace.TraceId)
	assert.Equal(t, "span-456", trace.SpanId)
}

// TestTrace_JSONMarshal tests JSON marshaling of Trace
func TestTrace_JSONMarshal(t *testing.T) {
	trace := Trace{
		TraceId: "test-trace-id",
		SpanId:  "test-span-id",
	}

	data, err := json.Marshal(trace)
	require.NoError(t, err)

	var decoded Trace
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, trace.TraceId, decoded.TraceId)
	assert.Equal(t, trace.SpanId, decoded.SpanId)
}

// TestResponse tests the Response struct
func TestResponse(t *testing.T) {
	resp := Response{
		Meta: Meta{Code: 2000, Message: "OK"},
		Data: map[string]string{"key": "value"},
		Tracing: &Trace{
			TraceId: "trace-123",
			SpanId:  "span-456",
		},
	}

	assert.Equal(t, 2000, resp.Meta.Code)
	assert.Equal(t, "OK", resp.Meta.Message)
	assert.NotNil(t, resp.Data)
	assert.NotNil(t, resp.Tracing)
}

// TestResponse_JSONMarshal tests JSON marshaling of Response
func TestResponse_JSONMarshal(t *testing.T) {
	testData := map[string]interface{}{
		"name": "test",
		"age":  30,
	}

	resp := Response{
		Meta:    Meta{Code: 2000, Message: "Success"},
		Data:    testData,
		Tracing: &Trace{TraceId: "t1", SpanId: "s1"},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded Response
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resp.Meta.Code, decoded.Meta.Code)
	assert.Equal(t, resp.Meta.Message, decoded.Meta.Message)
	assert.NotNil(t, decoded.Data)
	assert.NotNil(t, decoded.Tracing)
}

// TestListData tests the ListData struct
func TestListData(t *testing.T) {
	rows := []map[string]string{
		{"id": "1", "name": "item1"},
		{"id": "2", "name": "item2"},
	}

	listData := ListData{
		Rows:       rows,
		TotalCount: 100,
	}

	assert.Equal(t, rows, listData.Rows)
	assert.Equal(t, 100, listData.TotalCount)
}

// TestListData_JSONMarshal tests JSON marshaling of ListData
func TestListData_JSONMarshal(t *testing.T) {
	rows := []string{"item1", "item2", "item3"}
	listData := ListData{
		Rows:       rows,
		TotalCount: 3,
	}

	data, err := json.Marshal(listData)
	require.NoError(t, err)

	var decoded ListData
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, listData.TotalCount, decoded.TotalCount)
	assert.NotNil(t, decoded.Rows)
}

// TestSuccessResp tests the SuccessResp function
func TestSuccessResp(t *testing.T) {
	ctx := context.Background()
	data := map[string]string{"status": "ok"}

	resp := SuccessResp(ctx, data)

	assert.Equal(t, CodeSuccess, resp.Meta.Code)
	assert.Equal(t, "OK", resp.Meta.Message)
	assert.Equal(t, data, resp.Data)
}

// TestSuccessResp_WithNilData tests SuccessResp with nil data
func TestSuccessResp_WithNilData(t *testing.T) {
	ctx := context.Background()

	resp := SuccessResp(ctx, nil)

	assert.Equal(t, CodeSuccess, resp.Meta.Code)
	assert.Equal(t, "OK", resp.Meta.Message)
	assert.Nil(t, resp.Data)
}

// TestErrorResp tests the ErrorResp function
func TestErrorResp(t *testing.T) {
	ctx := context.Background()
	code := 4001
	errMsg := "Invalid parameter"
	data := map[string]string{"field": "username"}

	resp := ErrorResp(ctx, code, errMsg, data)

	assert.Equal(t, code, resp.Meta.Code)
	assert.Equal(t, errMsg, resp.Meta.Message)
	assert.Equal(t, data, resp.Data)
}

// TestErrorResp_WithNilData tests ErrorResp with nil data
func TestErrorResp_WithNilData(t *testing.T) {
	ctx := context.Background()

	resp := ErrorResp(ctx, 5000, "Internal error", nil)

	assert.Equal(t, 5000, resp.Meta.Code)
	assert.Equal(t, "Internal error", resp.Meta.Message)
	assert.Nil(t, resp.Data)
}

// TestError tests the Error struct
func TestError(t *testing.T) {
	err := Error{
		Code:    4001,
		Message: "Validation failed",
	}

	assert.Equal(t, 4001, err.Code)
	assert.Equal(t, "Validation failed", err.Message)
	assert.Nil(t, err.OriginError)
}

// TestError_Error tests the Error method
func TestError_Error(t *testing.T) {
	err := Error{
		Code:    5000,
		Message: "Database error",
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "Code 5000")
	assert.Contains(t, errStr, "Database error")
}

// TestError_WithOriginError tests Error with origin error
func TestError_WithOriginError(t *testing.T) {
	originErr := assert.AnError
	err := Error{
		Code:        6001,
		Message:     "Client error",
		OriginError: originErr,
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "Code 6001")
	assert.Contains(t, errStr, "Client error")
	assert.NotNil(t, err.OriginError)
}

// TestParseResponse_Success tests successful response parsing
func TestParseResponse_Success(t *testing.T) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	respData := Response{
		Meta: Meta{Code: CodeSuccess, Message: "OK"},
		Data: map[string]interface{}{
			"name": "John",
			"age":  30,
		},
	}

	jsonData, err := json.Marshal(respData)
	require.NoError(t, err)

	reader := bytes.NewReader(jsonData)
	var targetData TestData
	meta, trace, err := ParseResponse(reader, &targetData)

	require.NoError(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, CodeSuccess, meta.Code)
	assert.Equal(t, "John", targetData.Name)
	assert.Equal(t, 30, targetData.Age)
	assert.Nil(t, trace) // No tracing in this test
}

// TestParseResponse_ErrorCode tests response parsing with error code
func TestParseResponse_ErrorCode(t *testing.T) {
	respData := Response{
		Meta: Meta{Code: 4001, Message: "Invalid parameter"},
		Data: nil,
	}

	jsonData, err := json.Marshal(respData)
	require.NoError(t, err)

	reader := bytes.NewReader(jsonData)
	var targetData map[string]interface{}
	meta, _, err := ParseResponse(reader, &targetData)

	require.Error(t, err)
	assert.NotNil(t, meta)
	assert.Equal(t, 4001, meta.Code)
	assert.Contains(t, err.Error(), "Invalid parameter")
}

// TestParseResponse_InvalidJSON tests response parsing with invalid JSON
func TestParseResponse_InvalidJSON(t *testing.T) {
	reader := bytes.NewReader([]byte("invalid json"))
	var targetData map[string]interface{}
	meta, trace, err := ParseResponse(reader, &targetData)

	require.Error(t, err)
	assert.Nil(t, meta)
	assert.Nil(t, trace)
}

// TestParseResponse_NoData tests response parsing with zero code
func TestParseResponse_NoData(t *testing.T) {
	respData := Response{
		Meta: Meta{Code: 0, Message: ""},
		Data: nil,
	}

	jsonData, err := json.Marshal(respData)
	require.NoError(t, err)

	reader := bytes.NewReader(jsonData)
	var targetData map[string]interface{}
	_, _, err = ParseResponse(reader, &targetData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data")
}

// TestParseResponse_WithTracing tests response parsing with tracing
func TestParseResponse_WithTracing(t *testing.T) {
	respData := Response{
		Meta: Meta{Code: CodeSuccess, Message: "OK"},
		Data: map[string]interface{}{"result": "ok"},
		Tracing: &Trace{
			TraceId: "test-trace",
			SpanId:  "test-span",
		},
	}

	jsonData, err := json.Marshal(respData)
	require.NoError(t, err)

	reader := bytes.NewReader(jsonData)
	var targetData map[string]interface{}
	meta, trace, err := ParseResponse(reader, &targetData)

	require.NoError(t, err)
	assert.NotNil(t, meta)
	assert.NotNil(t, trace)
	assert.Equal(t, "test-trace", trace.TraceId)
	assert.Equal(t, "test-span", trace.SpanId)
}

// TestNewListData tests the NewListData function
func TestNewListData(t *testing.T) {
	rows := []string{"item1", "item2", "item3"}
	totalCount := 100

	listData := NewListData(rows, totalCount)

	assert.Equal(t, rows, listData.Rows)
	assert.Equal(t, totalCount, listData.TotalCount)
}

// TestNewListData_Empty tests NewListData with empty rows
func TestNewListData_Empty(t *testing.T) {
	rows := []string{}
	totalCount := 0

	listData := NewListData(rows, totalCount)

	assert.Equal(t, rows, listData.Rows)
	assert.Equal(t, 0, listData.TotalCount)
}

// TestNewListData_Nil tests NewListData with nil rows
func TestNewListData_Nil(t *testing.T) {
	listData := NewListData(nil, 0)

	assert.Nil(t, listData.Rows)
	assert.Equal(t, 0, listData.TotalCount)
}

// TestCodeSuccess tests the CodeSuccess constant
func TestCodeSuccess(t *testing.T) {
	assert.Equal(t, 2000, CodeSuccess)
}

// BenchmarkSuccessResp benchmarks SuccessResp function
func BenchmarkSuccessResp(b *testing.B) {
	ctx := context.Background()
	data := map[string]string{"key": "value"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SuccessResp(ctx, data)
	}
}

// BenchmarkErrorResp benchmarks ErrorResp function
func BenchmarkErrorResp(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ErrorResp(ctx, 4001, "Error message", nil)
	}
}

// BenchmarkParseResponse benchmarks ParseResponse function
func BenchmarkParseResponse(b *testing.B) {
	respData := Response{
		Meta: Meta{Code: CodeSuccess, Message: "OK"},
		Data: map[string]interface{}{"name": "test", "value": 123},
	}

	jsonData, _ := json.Marshal(respData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(jsonData)
		var targetData map[string]interface{}
		_, _, _ = ParseResponse(reader, &targetData)
	}
}

// TestParseResponse_EmptyReader tests parsing with empty reader
func TestParseResponse_EmptyReader(t *testing.T) {
	reader := bytes.NewReader([]byte{})
	var targetData map[string]interface{}
	meta, trace, err := ParseResponse(reader, &targetData)

	require.Error(t, err)
	assert.Nil(t, meta)
	assert.Nil(t, trace)
}

// TestParseResponse_NilTarget tests parsing into nil target
func TestParseResponse_NilTarget(t *testing.T) {
	respData := Response{
		Meta: Meta{Code: CodeSuccess, Message: "OK"},
		Data: map[string]interface{}{"key": "value"},
	}

	jsonData, err := json.Marshal(respData)
	require.NoError(t, err)

	reader := bytes.NewReader(jsonData)
	_, _, err = ParseResponse(reader, nil)

	require.Error(t, err)
}

