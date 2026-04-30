// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToMCPTool_Success(t *testing.T) {
	type Req struct {
		N int `json:"n"`
	}
	type Resp struct {
		Double int `json:"double"`
	}
	def := &EndpointDef[Req, Resp]{
		Name:        "calc",
		Description: "doubles",
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			return &Resp{Double: req.N * 2}, nil
		},
	}
	tool := ToMCPTool(def)
	require.NotNil(t, tool)
	out, err := tool.Handler(context.Background(), json.RawMessage(`{"n":21}`))
	require.NoError(t, err)
	resp, ok := out.(*Resp)
	require.True(t, ok)
	assert.Equal(t, 42, resp.Double)
}

func TestToMCPTool_BindError(t *testing.T) {
	type Req struct{ X int `json:"x"` }
	type Resp struct{}

	var calls atomic.Int32
	def := &EndpointDef[Req, Resp]{
		Name: "t",
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			calls.Add(1)
			return &Resp{}, nil
		},
	}
	tool := ToMCPTool(def)
	_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))
	require.Error(t, err)
	assert.Zero(t, calls.Load())
}

func TestToMCPTool_HandlerError(t *testing.T) {
	type Req struct{}
	type Resp struct{}
	want := errors.New("boom")
	def := &EndpointDef[Req, Resp]{
		Name: "t",
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			return nil, want
		},
	}
	tool := ToMCPTool(def)
	_, err := tool.Handler(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

func TestToMCPTool_NameDefault(t *testing.T) {
	type Req struct{}
	type Resp struct{}
	def := &EndpointDef[Req, Resp]{
		Name: "default-name",
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			return &Resp{}, nil
		},
	}
	tool := ToMCPTool(def)
	assert.Equal(t, "default-name", tool.Name)
}

func TestToMCPTool_NameOverride(t *testing.T) {
	type Req struct{}
	type Resp struct{}
	def := &EndpointDef[Req, Resp]{
		Name:        "default-name",
		MCPToolName: "override",
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			return &Resp{}, nil
		},
	}
	tool := ToMCPTool(def)
	assert.Equal(t, "override", tool.Name)
}

func TestToMCPToolFromRaw_NilSchema(t *testing.T) {
	tool := ToMCPToolFromRaw("raw", "d", nil, func(ctx context.Context, params map[string]any) (any, error) {
		return "ok", nil
	})
	exp := map[string]any{"type": "object", "properties": map[string]any{}}
	assert.Equal(t, exp, tool.InputSchema)
}

func TestToMCPToolFromRaw_CustomSchema(t *testing.T) {
	custom := map[string]any{"type": "object", "title": "T"}
	tool := ToMCPToolFromRaw("raw", "d", custom, func(ctx context.Context, params map[string]any) (any, error) {
		return nil, nil
	})
	assert.Equal(t, custom, tool.InputSchema)
}

func TestToMCPToolFromRaw_NilParams(t *testing.T) {
	var got map[string]any
	tool := ToMCPToolFromRaw("raw", "d", map[string]any{}, func(ctx context.Context, params map[string]any) (any, error) {
		got = params
		return nil, nil
	})
	_, err := tool.Handler(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, got)

	_, err = tool.Handler(context.Background(), json.RawMessage{})
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestToMCPToolFromRaw_InvalidJSON(t *testing.T) {
	tool := ToMCPToolFromRaw("raw", "d", map[string]any{}, func(ctx context.Context, params map[string]any) (any, error) {
		t.Fatal("handler should not run")
		return nil, nil
	})
	_, err := tool.Handler(context.Background(), json.RawMessage(`{`))
	require.Error(t, err)
}

func TestGetMCPTool_HTTPOnly(t *testing.T) {
	type Req struct{}
	type Resp struct{}
	def := &EndpointDef[Req, Resp]{
		Name:     "h",
		HTTPOnly: true,
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			return &Resp{}, nil
		},
	}
	assert.Nil(t, def.GetMCPTool())
}

func TestGetMCPTool_RawMCPHandlerPriority(t *testing.T) {
	type Req struct{}
	type Resp struct{}
	def := &EndpointDef[Req, Resp]{
		Name:        "n",
		MCPToolName: "from-raw",
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			t.Fatal("typed handler should not be used")
			return nil, nil
		},
		RawMCPHandler: func(ctx context.Context, params map[string]any) (any, error) {
			return "raw-out", nil
		},
	}
	tool := def.GetMCPTool()
	require.NotNil(t, tool)
	assert.Equal(t, "from-raw", tool.Name)
	out, err := tool.Handler(context.Background(), json.RawMessage(`{"a":1}`))
	require.NoError(t, err)
	assert.Equal(t, "raw-out", out)
}

func TestGetMCPTool_HandlerOnly(t *testing.T) {
	type Req struct{}
	type Resp struct{ OK bool `json:"ok"` }
	def := &EndpointDef[Req, Resp]{
		Name: "only-handler",
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			return &Resp{OK: true}, nil
		},
	}
	tool := def.GetMCPTool()
	require.NotNil(t, tool)
	assert.Equal(t, "only-handler", tool.Name)
}

func TestGetMCPTool_NoHandler(t *testing.T) {
	type Req struct{}
	type Resp struct{}
	def := &EndpointDef[Req, Resp]{Name: "empty"}
	assert.Nil(t, def.GetMCPTool())
}

func TestToGinHandler_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type Req struct {
		Q string `query:"q"`
	}
	type Resp struct {
		Echo string `json:"echo"`
	}
	h := ToGinHandler(func(ctx context.Context, req *Req) (*Resp, error) {
		return &Resp{Echo: req.Q}, nil
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x?q=hi", nil)
	h(c)
	assert.Equal(t, http.StatusOK, w.Code)
	var body Resp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "hi", body.Echo)
}

func TestToGinHandler_BindError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type Req struct {
		N int `query:"n"`
	}
	type Resp struct{}
	h := ToGinHandler(func(ctx context.Context, req *Req) (*Resp, error) {
		return &Resp{}, nil
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x?n=bad", nil)
	h(c)
	assert.Greater(t, len(c.Errors), 0)
}

func TestToGinHandler_HandlerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type Req struct{}
	type Resp struct{}
	want := errors.New("http-err")
	h := ToGinHandler(func(ctx context.Context, req *Req) (*Resp, error) {
		return nil, want
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	h(c)
	assert.Greater(t, len(c.Errors), 0)
	assert.ErrorIs(t, c.Errors.Last().Err, want)
}

func TestGetGinHandler_MCPOnly(t *testing.T) {
	type Req struct{}
	type Resp struct{}
	def := &EndpointDef[Req, Resp]{
		MCPOnly: true,
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			return &Resp{}, nil
		},
	}
	assert.Nil(t, def.GetGinHandler())
}

func TestGetGinHandler_RawHTTPHandlerPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type Req struct{}
	type Resp struct{}
	var rawHits atomic.Int32
	def := &EndpointDef[Req, Resp]{
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			t.Fatal("Handler must not run when RawHTTP is set")
			return nil, nil
		},
		RawHTTPHandler: func(c *gin.Context) {
			rawHits.Add(1)
			c.Status(http.StatusTeapot)
		},
	}
	g := def.GetGinHandler()
	require.NotNil(t, g)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	g(c)
	assert.Equal(t, 1, int(rawHits.Load()))
	assert.Equal(t, http.StatusTeapot, c.Writer.Status(), "httptest recorder Code=%d (Gin may defer WriteHeader)", w.Code)
}

func TestGetGinHandler_HandlerOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type Req struct{}
	type Resp struct{ OK bool `json:"ok"` }
	def := &EndpointDef[Req, Resp]{
		Handler: func(ctx context.Context, req *Req) (*Resp, error) {
			return &Resp{OK: true}, nil
		},
	}
	g := def.GetGinHandler()
	require.NotNil(t, g)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	g(c)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetGinHandler_NoHandler(t *testing.T) {
	type Req struct{}
	type Resp struct{}
	def := &EndpointDef[Req, Resp]{Name: "x"}
	assert.Nil(t, def.GetGinHandler())
}
