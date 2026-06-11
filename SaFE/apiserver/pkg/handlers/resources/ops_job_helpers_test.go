/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestParseListOpsJobQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Defaults.
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h := &Handler{}
	q, err := h.parseListOpsJobQuery(c)
	assert.NoError(t, err)
	assert.Equal(t, view.DefaultQueryLimit, q.Limit)
	assert.Equal(t, dbclient.DESC, q.Order)
	assert.False(t, q.SinceTime.IsZero())

	// Invalid until time.
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?until=not-a-time", nil)
	_, err = h.parseListOpsJobQuery(c2)
	assert.Error(t, err)
}

func TestCvtToListOpsJobSql(t *testing.T) {
	q := &view.ListOpsJobRequest{
		ListOpsJobInput: view.ListOpsJobInput{
			ClusterId:   "c1",
			WorkspaceId: "ws-1",
			Phase:       v1.OpsJobRunning,
			Type:        v1.OpsJobRebootType,
			UserName:    "alice",
			JobName:     "job",
			SortBy:      "creation_time",
			Order:       dbclient.DESC,
		},
		UserId: "u1",
	}
	sql, orderBy := cvtToListOpsJobSql(q)
	assert.NotNil(t, sql)
	assert.NotEmpty(t, orderBy)
}

func TestCvtToGetOpsJobSql(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	// Empty job id -> error.
	_, err := h.cvtToGetOpsJobSql(c)
	assert.Error(t, err)

	c.Set(common.Name, "job-1")
	sql, err := h.cvtToGetOpsJobSql(c)
	assert.NoError(t, err)
	assert.NotNil(t, sql)
}

func TestAuthGetOpsJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)

	// Admin passes for each resource-kind branch.
	for _, opsType := range []string{
		string(v1.OpsJobPreflightType), string(v1.OpsJobDownloadType),
		string(v1.OpsJobDumpLogType), string(v1.OpsJobAddonType), "other",
	} {
		assert.NoError(t, h.authGetOpsJob(c, "ws-1", opsType))
	}
}

func TestParseCreateOpsJobRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Valid.
	body := `{"name":"job","type":"reboot","inputs":[{"name":"node","value":"n1"}]}`
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	req, _, err := parseCreateOpsJobRequest(c)
	assert.NoError(t, err)
	assert.Equal(t, "job", req.Name)

	// Missing inputs.
	body2 := `{"name":"job","type":"reboot"}`
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body2))
	c2.Request.Header.Set("Content-Type", "application/json")
	_, _, err = parseCreateOpsJobRequest(c2)
	assert.Error(t, err)
}

func TestCvtToGetOpsJobResponse(t *testing.T) {
	job := &dbclient.OpsJob{
		JobId:  "job-1",
		Type:   string(v1.OpsJobRebootType),
		Inputs: []byte("[node:n1,workload:wl1]"),
		Env:    sql.NullString{String: `{"K":"V"}`, Valid: true},
	}
	resp := cvtToGetOpsJobResponse(job)
	assert.Equal(t, "job-1", resp.JobId)
	assert.NotEmpty(t, resp.Inputs)
}

func TestDeserializeParams(t *testing.T) {
	// Short input -> nil.
	assert.Nil(t, deserializeParams(""))
	assert.Nil(t, deserializeParams("[]"))

	params := deserializeParams("[node:n1,workload:wl1]")
	assert.Len(t, params, 2)
	assert.Equal(t, "node", params[0].Name)
	assert.Equal(t, "n1", params[0].Value)
}

func TestGetParametersExcept(t *testing.T) {
	inputs := []v1.Parameter{{Name: "node", Value: "n1"}, {Name: "workload", Value: "wl1"}}
	result := getParametersExcept(inputs, "node")
	assert.Len(t, result, 1)
	assert.Equal(t, "workload", result[0].Name)
}

func TestHasParameters(t *testing.T) {
	inputs := []v1.Parameter{{Name: "node", Value: "n1"}}
	assert.True(t, hasParameters(inputs, "node"))
	assert.True(t, hasParameters(inputs, "missing", "node"))
	assert.False(t, hasParameters(inputs, "workload"))
}

func TestGetParamValue(t *testing.T) {
	inputs := []v1.Parameter{{Name: "node", Value: "n1"}}
	assert.Equal(t, "n1", getParamValue(inputs, "node"))
	assert.Equal(t, "", getParamValue(inputs, "missing"))
}

func TestParseServedModelNameFromCmd(t *testing.T) {
	assert.Equal(t, "my-model", parseServedModelNameFromCmd("vllm serve --served-model-name my-model --port 8000"))
	assert.Equal(t, "m2", parseServedModelNameFromCmd("cmd --served-model-name=m2"))
	assert.Equal(t, "", parseServedModelNameFromCmd("vllm serve --port 8000"))
}

func TestExtractServedModelName(t *testing.T) {
	ep := base64.StdEncoding.EncodeToString([]byte("vllm serve --served-model-name my-model"))
	assert.Equal(t, "my-model", extractServedModelName(ep, sql.NullString{}))

	// From entryPoints array.
	ep2 := base64.StdEncoding.EncodeToString([]byte("cmd --served-model-name arr-model"))
	arr := `["` + ep2 + `"]`
	assert.Equal(t, "arr-model", extractServedModelName("", sql.NullString{String: arr, Valid: true}))

	// None.
	assert.Equal(t, "", extractServedModelName("", sql.NullString{}))
}

func TestExtractModelNameFromEnv(t *testing.T) {
	assert.Equal(t, "", extractModelNameFromEnv(sql.NullString{}))
	assert.Equal(t, "", extractModelNameFromEnv(sql.NullString{String: "not-json", Valid: true}))
	assert.Equal(t, "Qwen/Q", extractModelNameFromEnv(sql.NullString{String: `{"PRIMUS_SOURCE_MODEL":"Qwen/Q"}`, Valid: true}))
	assert.Equal(t, "m2", extractModelNameFromEnv(sql.NullString{String: `{"MODEL_NAME":"m2"}`, Valid: true}))
}
