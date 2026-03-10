package unified

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unexportedBase simulates the diagBaseRequest pattern
type unexportedBase struct {
	UID     string `param:"uid"`
	Cluster string `query:"cluster"`
}

type RequestWithUnexportedEmbed struct {
	unexportedBase
}

// ExportedBase simulates the fix
type ExportedBase struct {
	UID     string `param:"uid"`
	Cluster string `query:"cluster"`
}

type RequestWithExportedEmbed struct {
	ExportedBase
}

func TestBindGinRequest_UnexportedEmbeddedStruct_PathParam(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test/my-uid-123?cluster=prod", nil)
	c.Params = gin.Params{
		{Key: "uid", Value: "my-uid-123"},
	}

	var req RequestWithUnexportedEmbed
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, "my-uid-123", req.UID, "UID should be bound from path param")
	assert.Equal(t, "prod", req.Cluster, "Cluster should be bound from query param")
}

func TestBindGinRequest_ExportedEmbeddedStruct_PathParam(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test/my-uid-456?cluster=staging", nil)
	c.Params = gin.Params{
		{Key: "uid", Value: "my-uid-456"},
	}

	var req RequestWithExportedEmbed
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, "my-uid-456", req.UID, "UID should be bound from path param")
	assert.Equal(t, "staging", req.Cluster, "Cluster should be bound from query param")
}
