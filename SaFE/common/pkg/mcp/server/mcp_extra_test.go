/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProtocolHelpers(t *testing.T) {
	id := json.RawMessage(`1`)
	resp := NewResponse(id, map[string]string{"a": "b"})
	assert.NotNil(t, resp.Result)
	assert.Nil(t, resp.Error)

	errResp := NewErrorResponse(id, ErrorCodeInternalError, "boom", "detail")
	assert.NotNil(t, errResp.Error)
	assert.Equal(t, ErrorCodeInternalError, errResp.Error.Code)

	tc := NewTextContent("hello")
	assert.Equal(t, "hello", tc.Text)

	jc, err := NewJSONContent(map[string]int{"x": 1})
	assert.NoError(t, err)
	assert.Contains(t, jc.Text, "x")

	// unmarshalable value -> error
	_, err = NewJSONContent(make(chan int))
	assert.Error(t, err)
}

func TestParseRequestAndFlags(t *testing.T) {
	req, err := ParseRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	assert.NoError(t, err)
	assert.Equal(t, "ping", req.Method)
	assert.False(t, req.IsNotification())
	assert.Equal(t, "1", req.GetIDString())

	// notification (no id)
	notif, err := ParseRequest([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	assert.NoError(t, err)
	assert.True(t, notif.IsNotification())

	_, err = ParseRequest([]byte(`not-json`))
	assert.Error(t, err)
}

func TestServerAccessorsAndListHandlers(t *testing.T) {
	s := New()
	assert.False(t, s.IsInitialized())
	assert.Equal(t, 0, s.ToolCount())
	assert.Empty(t, s.GetToolNames())
	assert.Nil(t, s.GetClientInfo())

	s.SetInstructions("be helpful")

	ctx := context.Background()
	// initialized notification -> nil response
	assert.Nil(t, s.HandleRequest(ctx, &JSONRPCRequest{Method: MethodInitialized}))

	// resources/list and prompts/list
	r := s.HandleRequest(ctx, &JSONRPCRequest{Method: MethodResourcesList, ID: json.RawMessage(`1`)})
	assert.Nil(t, r.Error)
	p := s.HandleRequest(ctx, &JSONRPCRequest{Method: MethodPromptsList, ID: json.RawMessage(`2`)})
	assert.Nil(t, p.Error)

	// after initialize, accessors reflect state
	initReq := &JSONRPCRequest{
		Method: MethodInitialize,
		ID:     json.RawMessage(`3`),
		Params: json.RawMessage(`{"protocolVersion":"1.0","clientInfo":{"name":"c","version":"1"}}`),
	}
	assert.Nil(t, s.HandleRequest(ctx, initReq).Error)
	assert.True(t, s.IsInitialized())
	assert.NotNil(t, s.GetClientInfo())
}
