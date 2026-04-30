// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ginCtxFromReq(req *http.Request) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

func TestBindGinRequest_Query(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type R struct {
		Name string `query:"name"`
	}
	req := httptest.NewRequest(http.MethodGet, "/?name=value", nil)
	c, _ := ginCtxFromReq(req)
	var got R
	require.NoError(t, BindGinRequest(c, &got))
	assert.Equal(t, "value", got.Name)
}

func TestBindGinRequest_Param(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type R struct {
		ID string `param:"id"`
	}
	req := httptest.NewRequest(http.MethodGet, "/x/abc", nil)
	c, _ := ginCtxFromReq(req)
	c.Params = []gin.Param{{Key: "id", Value: "abc"}}
	var got R
	require.NoError(t, BindGinRequest(c, &got))
	assert.Equal(t, "abc", got.ID)
}

func TestBindGinRequest_Header(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type R struct {
		Token string `header:"X-Token"`
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Token", "secret")
	c, _ := ginCtxFromReq(req)
	var got R
	require.NoError(t, BindGinRequest(c, &got))
	assert.Equal(t, "secret", got.Token)
}

func TestBindGinRequest_JSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type R struct {
		Foo string `json:"foo"`
	}
	body := bytes.NewBufferString(`{"foo":"bar"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")
	c, _ := ginCtxFromReq(req)
	var got R
	require.NoError(t, BindGinRequest(c, &got))
	assert.Equal(t, "bar", got.Foo)
}

func TestBindGinRequest_PtrError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type R struct{ X int }
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c, _ := ginCtxFromReq(req)

	var n int
	err := BindGinRequest(c, &n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pointer to struct")

	var nilPtr *R
	err = BindGinRequest(c, nilPtr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-nil pointer")
}

func TestBindGinRequest_AllTypesViaQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type R struct {
		I   int      `query:"i"`
		U   uint     `query:"u"`
		F   float64  `query:"f"`
		B   bool     `query:"b"`
		S   string   `query:"s"`
		Sl  []string `query:"sl"`
		Pi  *int     `query:"pi"`
	}
	raw := "/?i=-3&u=42&f=1.5&b=true&s=hi&sl=a,b,%20c&pi=7"
	req := httptest.NewRequest(http.MethodGet, raw, nil)
	c, _ := ginCtxFromReq(req)
	var got R
	require.NoError(t, BindGinRequest(c, &got))
	assert.Equal(t, -3, got.I)
	assert.Equal(t, uint(42), got.U)
	assert.InEpsilon(t, 1.5, got.F, 1e-9)
	assert.True(t, got.B)
	assert.Equal(t, "hi", got.S)
	assert.Equal(t, []string{"a", "b", "c"}, got.Sl)
	require.NotNil(t, got.Pi)
	assert.Equal(t, 7, *got.Pi)
}

func TestBindGinRequest_ParseError_Int(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type R struct {
		Count int `query:"count"`
	}
	req := httptest.NewRequest(http.MethodGet, "/?count=not-a-number", nil)
	c, _ := ginCtxFromReq(req)
	var got R
	err := BindGinRequest(c, &got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set")
}

func TestBindGinRequest_EmbeddedStruct(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type Inner struct {
		A string `query:"a"`
		B int    `query:"b"`
	}
	type Outer struct {
		Inner
	}
	req := httptest.NewRequest(http.MethodGet, "/?a=embed&b=99", nil)
	c, _ := ginCtxFromReq(req)
	var got Outer
	require.NoError(t, BindGinRequest(c, &got))
	assert.Equal(t, "embed", got.A)
	assert.Equal(t, 99, got.B)
}

func TestBindMCPRequest(t *testing.T) {
	type R struct {
		X int    `json:"x"`
		Y string `json:"y"`
	}
	raw := json.RawMessage(`{"x":3,"y":"z"}`)
	var got R
	require.NoError(t, BindMCPRequest(raw, &got))
	assert.Equal(t, 3, got.X)
	assert.Equal(t, "z", got.Y)
}

func TestBindMCPRequest_Empty(t *testing.T) {
	type R struct {
		X int `json:"x"`
	}
	var got R
	require.NoError(t, BindMCPRequest(nil, &got))
	assert.Zero(t, got.X)
	require.NoError(t, BindMCPRequest(json.RawMessage{}, &got))
	assert.Zero(t, got.X)
}

func TestParseMCPTag(t *testing.T) {
	cases := []struct {
		tag  string
		name string
		desc string
		req  bool
	}{
		{"name", "name", "", false},
		{"name,description=hello,required", "name", "hello", true},
		{"name,required", "name", "", true},
		{"name,description=multi word", "name", "multi word", false},
	}
	for _, tc := range cases {
		opts := ParseMCPTag(tc.tag)
		assert.Equal(t, tc.name, opts.Name, "tag=%q", tc.tag)
		assert.Equal(t, tc.desc, opts.Description, "tag=%q", tc.tag)
		assert.Equal(t, tc.req, opts.Required, "tag=%q", tc.tag)
	}
}

func TestParseTagName(t *testing.T) {
	assert.Equal(t, "name", parseTagName("name,omitempty"))
	assert.Equal(t, "name", parseTagName("name"))
}

func TestSetFieldValue_Errors(t *testing.T) {
	vBool := reflect.ValueOf(new(bool)).Elem()
	err := setFieldValue(vBool, "notbool")
	require.Error(t, err)

	vFloat := reflect.ValueOf(new(float64)).Elem()
	err = setFieldValue(vFloat, "xyz")
	require.Error(t, err)

	vIntSlice := reflect.ValueOf(new([]int)).Elem()
	err = setFieldValue(vIntSlice, "1,2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported slice")

	vMap := reflect.ValueOf(new(map[string]int)).Elem()
	err = setFieldValue(vMap, "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported field type")
}
