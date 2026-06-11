/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
)

// withOpenSearch enables OpenSearch and registers a stub client for the given
// cluster that returns the provided canned `_search` response body.
func withOpenSearch(t *testing.T, clusterId string, respBody string) {
	t.Helper()
	commonconfig.SetValue("opensearch.enable", "true")
	sc := commonsearch.NewTestSearchClient(
		func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
			return []byte(respBody), nil
		},
	)
	cleanup := commonsearch.RegisterClientForTest(clusterId, sc)
	t.Cleanup(func() {
		cleanup()
		commonconfig.SetValue("opensearch.enable", "false")
	})
}

const emptyHits = `{"hits":{"total":{"value":0},"hits":[]}}`

func newWorkloadForLog(name, cluster, workspace string) *v1.Workload {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{v1.ClusterIdLabel: cluster},
		},
		Spec: v1.WorkloadSpec{Workspace: workspace},
	}
	return wl
}

func TestGetWorkloadLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("opensearch disabled", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "wl-1")
		h.GetWorkloadLog(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("success", func(t *testing.T) {
		withOpenSearch(t, "c1", emptyHits)
		h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "wl-1")
		h.GetWorkloadLog(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}

func TestGetServiceLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withOpenSearch(t, "", emptyHits)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "my-svc")
	h.GetServiceLog(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetWorkloadEventHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withOpenSearch(t, "c1", emptyHits)
	h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	h.GetWorkloadEvent(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetCICDArcLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withOpenSearch(t, "c1", emptyHits)

	wl := newWorkloadForLog("wl-cicd", "c1", "ws-1")
	wl.Spec.GroupVersionKind = v1.GroupVersionKind{Kind: common.CICDScaleRunnerSetKind}
	h, user := newAdminHandlerWithObjects(wl)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-cicd")
	h.GetCICDArcLog(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetWorkloadLogContextHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Stub returns a hit matching the requested docId so addContextDoc succeeds.
	respWithDoc := `{"hits":{"total":{"value":1},"hits":[{"_id":"doc-1","_source":{"message":"hello"}}]}}`
	withOpenSearch(t, "c1", respWithDoc)
	h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	// parseContextQuery needs a `since` field and a docId path param.
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"since":"2025-01-01T00:00:00.000Z"}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	c.Params = gin.Params{{Key: "docId", Value: "doc-1"}}
	h.GetWorkloadLogContext(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetAndAuthWorkload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	wl, err := h.getAndAuthWorkload(c)
	assert.NoError(t, err)
	assert.Equal(t, "wl-1", wl.Name)

	// Empty name -> bad request.
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c2.Set(common.UserId, user.Name)
	_, err = h.getAndAuthWorkload(c2)
	assert.Error(t, err)
}
