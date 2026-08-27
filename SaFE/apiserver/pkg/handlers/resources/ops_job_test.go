/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	testifyassert "github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// TestCvtToOpsJobResponseItem tests conversion from database OpsJob to response item
func TestCvtToOpsJobResponseItem(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		job      *dbclient.OpsJob
		validate func(*testing.T, view.OpsJobResponseItem)
	}{
		{
			name: "complete ops job",
			job: &dbclient.OpsJob{
				JobId:        "preflight-job-123",
				Cluster:      "test-cluster",
				Workspace:    sql.NullString{String: "test-workspace", Valid: true},
				UserId:       sql.NullString{String: "user-123", Valid: true},
				UserName:     sql.NullString{String: "testuser", Valid: true},
				Type:         string(v1.OpsJobPreflightType),
				Phase:        sql.NullString{String: string(v1.OpsJobRunning), Valid: true},
				CreationTime: pq.NullTime{Time: now, Valid: true},
				StartTime:    pq.NullTime{Time: now.Add(1 * time.Minute), Valid: true},
				EndTime:      pq.NullTime{Time: now.Add(10 * time.Minute), Valid: true},
				Timeout:      600,
			},
			validate: func(t *testing.T, result view.OpsJobResponseItem) {
				assert.Equal(t, "preflight-job-123", result.JobId)
				assert.Equal(t, "test-cluster", result.ClusterId)
				assert.Equal(t, "test-workspace", result.WorkspaceId)
				assert.Equal(t, "user-123", result.UserId)
				assert.Equal(t, "testuser", result.UserName)
				assert.Equal(t, v1.OpsJobPreflightType, result.Type)
				assert.Equal(t, v1.OpsJobRunning, result.Phase)
				assert.Equal(t, 600, result.TimeoutSecond)
				assert.NotEmpty(t, result.CreationTime)
			},
		},
		{
			name: "minimal ops job with null fields",
			job: &dbclient.OpsJob{
				JobId:     "addon-job-456",
				Cluster:   "prod-cluster",
				Type:      string(v1.OpsJobAddonType),
				Workspace: sql.NullString{Valid: false},
				UserId:    sql.NullString{Valid: false},
				UserName:  sql.NullString{Valid: false},
				Phase:     sql.NullString{Valid: false},
				Timeout:   0,
			},
			validate: func(t *testing.T, result view.OpsJobResponseItem) {
				assert.Equal(t, "addon-job-456", result.JobId)
				assert.Equal(t, "prod-cluster", result.ClusterId)
				assert.Equal(t, v1.OpsJobAddonType, result.Type)
				assert.Empty(t, result.WorkspaceId)
				assert.Empty(t, result.UserId)
				assert.Empty(t, result.UserName)
				// Empty phase should default to Pending
				assert.Equal(t, v1.OpsJobPending, result.Phase)
			},
		},
		{
			name: "dumplog job",
			job: &dbclient.OpsJob{
				JobId:        "dumplog-job-789",
				Cluster:      "debug-cluster",
				Type:         string(v1.OpsJobDumpLogType),
				Phase:        sql.NullString{String: string(v1.OpsJobSucceeded), Valid: true},
				CreationTime: pq.NullTime{Time: now, Valid: true},
				EndTime:      pq.NullTime{Time: now.Add(5 * time.Minute), Valid: true},
				Timeout:      300,
			},
			validate: func(t *testing.T, result view.OpsJobResponseItem) {
				assert.Equal(t, "dumplog-job-789", result.JobId)
				assert.Equal(t, v1.OpsJobDumpLogType, result.Type)
				assert.Equal(t, v1.OpsJobSucceeded, result.Phase)
				assert.Equal(t, 300, result.TimeoutSecond)
			},
		},
		{
			name: "failed ops job",
			job: &dbclient.OpsJob{
				JobId:        "failed-job-001",
				Cluster:      "test-cluster",
				Type:         string(v1.OpsJobPreflightType),
				Phase:        sql.NullString{String: string(v1.OpsJobFailed), Valid: true},
				CreationTime: pq.NullTime{Time: now, Valid: true},
				StartTime:    pq.NullTime{Time: now.Add(1 * time.Minute), Valid: true},
				EndTime:      pq.NullTime{Time: now.Add(2 * time.Minute), Valid: true},
				Timeout:      120,
			},
			validate: func(t *testing.T, result view.OpsJobResponseItem) {
				assert.Equal(t, v1.OpsJobFailed, result.Phase)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToOpsJobResponseItem(tt.job)
			tt.validate(t, result)
		})
	}
}

// TestBaseOpsJobRequestValidation tests BaseOpsJobRequest structure
func TestBaseOpsJobRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		request  view.BaseOpsJobRequest
		validate func(*testing.T, view.BaseOpsJobRequest)
	}{
		{
			name: "complete request",
			request: view.BaseOpsJobRequest{
				Name: "test-preflight",
				Type: v1.OpsJobPreflightType,
				Inputs: []v1.Parameter{
					{Name: "node", Value: "node-1"},
					{Name: "cluster", Value: "test-cluster"},
				},
				TimeoutSecond:           600,
				TTLSecondsAfterFinished: 3600,
			},
			validate: func(t *testing.T, req view.BaseOpsJobRequest) {
				assert.Equal(t, "test-preflight", req.Name)
				assert.Equal(t, v1.OpsJobPreflightType, req.Type)
				assert.Len(t, req.Inputs, 2)
				assert.Equal(t, 600, req.TimeoutSecond)
				assert.Equal(t, 3600, req.TTLSecondsAfterFinished)
			},
		},
		{
			name: "minimal request",
			request: view.BaseOpsJobRequest{
				Name: "simple-job",
				Type: v1.OpsJobAddonType,
				Inputs: []v1.Parameter{
					{Name: "addon", Value: "prometheus"},
				},
			},
			validate: func(t *testing.T, req view.BaseOpsJobRequest) {
				assert.Equal(t, "simple-job", req.Name)
				assert.Equal(t, v1.OpsJobAddonType, req.Type)
				assert.Len(t, req.Inputs, 1)
				assert.Equal(t, 0, req.TimeoutSecond)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.request)
		})
	}
}

// TestCreateAddonRequestValidation tests CreateAddonRequest structure
func TestCreateAddonRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		request  view.CreateAddonRequest
		validate func(*testing.T, view.CreateAddonRequest)
	}{
		{
			name: "addon request with batch settings",
			request: view.CreateAddonRequest{
				BaseOpsJobRequest: view.BaseOpsJobRequest{
					Name:              "addon-upgrade",
					Type:              v1.OpsJobAddonType,
					SecurityOperation: true,
					Inputs: []v1.Parameter{
						{Name: "addon", Value: "driver"},
					},
				},
				BatchCount:     5,
				AvailableRatio: floatPtr(0.95),
			},
			validate: func(t *testing.T, req view.CreateAddonRequest) {
				assert.Equal(t, 5, req.BatchCount)
				assert.NotNil(t, req.AvailableRatio)
				assert.Equal(t, 0.95, *req.AvailableRatio)
			},
		},
		{
			name: "addon request with defaults",
			request: view.CreateAddonRequest{
				BaseOpsJobRequest: view.BaseOpsJobRequest{
					Name: "addon-install",
					Type: v1.OpsJobAddonType,
					Inputs: []v1.Parameter{
						{Name: "addon", Value: "monitoring"},
					},
				},
			},
			validate: func(t *testing.T, req view.CreateAddonRequest) {
				assert.Equal(t, 0, req.BatchCount)
				assert.Nil(t, req.AvailableRatio)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.request)
		})
	}
}

// TestCreatePreflightRequestValidation tests CreatePreflightRequest structure
func TestCreatePreflightRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		request  view.CreatePreflightRequest
		validate func(*testing.T, view.CreatePreflightRequest)
	}{
		{
			name: "preflight with resource requirements",
			request: view.CreatePreflightRequest{
				BaseOpsJobRequest: view.BaseOpsJobRequest{
					Name: "network-check",
					Type: v1.OpsJobPreflightType,
					Inputs: []v1.Parameter{
						{Name: "node", Value: "node-1"},
					},
				},
				Resource: &v1.WorkloadResource{
					CPU:     "1",
					Memory:  "2Gi",
					Replica: 1,
				},
				Image:      strPtr("preflight-checker:v1.0"),
				EntryPoint: strPtr("L2Jpbi9iYXNo"), // base64 encoded
				Env: map[string]string{
					"CHECK_TYPE": "network",
					"TIMEOUT":    "300",
				},
				Hostpath: []string{"/var/log", "/etc"},
			},
			validate: func(t *testing.T, req view.CreatePreflightRequest) {
				assert.NotNil(t, req.Resource)
				assert.Equal(t, "1", req.Resource.CPU)
				assert.NotNil(t, req.Image)
				assert.Equal(t, "preflight-checker:v1.0", *req.Image)
				assert.NotNil(t, req.EntryPoint)
				assert.Len(t, req.Env, 2)
				assert.Len(t, req.Hostpath, 2)
			},
		},
		{
			name: "minimal preflight request",
			request: view.CreatePreflightRequest{
				BaseOpsJobRequest: view.BaseOpsJobRequest{
					Name: "simple-check",
					Type: v1.OpsJobPreflightType,
					Inputs: []v1.Parameter{
						{Name: "check", Value: "disk"},
					},
				},
			},
			validate: func(t *testing.T, req view.CreatePreflightRequest) {
				assert.Nil(t, req.Resource)
				assert.Nil(t, req.Image)
				assert.Nil(t, req.EntryPoint)
				assert.Nil(t, req.Env)
				assert.Nil(t, req.Hostpath)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.request)
		})
	}
}

// Helper functions
func floatPtr(f float64) *float64 {
	return &f
}

func strPtr(s string) *string {
	return &s
}

// --- merged from ops_job_dumplog_test.go ---

func TestGenerateDumpLogJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withS3(t)

	wl := newWorkloadForLog("wl-1", "c1", "ws-1")
	h, user := newAdminHandlerWithObjects(wl)

	body := `{"name":"dumplog","type":"dumplog","inputs":[{"name":"workload","value":"wl-1"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generateDumpLogJob(c, []byte(body))
	testifyassert.NoError(t, err)
	assert.Equal(t, "wl-1", job.Name)
	assert.Equal(t, "ws-1", v1.GetWorkspaceId(job))

	// Missing workload parameter -> bad request.
	body2 := `{"name":"dumplog","type":"dumplog","inputs":[{"name":"foo","value":"bar"}]}`
	c2, _ := newOpsJobCtx(user.Name, body2)
	_, err = h.generateDumpLogJob(c2, []byte(body2))
	testifyassert.Error(t, err)
}

func TestGenerateDownloadJob(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Handler with workspace CR (ctrl client) + a general secret (clientSet).
	user := genMockUser()
	role := genMockRole()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	sch := runtime.NewScheme()
	_ = v1.AddToScheme(sch)
	ctrlClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(user, role, ws).Build()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sec-1",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.SecretTypeLabel: string(v1.SecretGeneral)},
		},
	}
	h := &Handler{
		Client:           ctrlClient,
		clientSet:        k8sfake.NewSimpleClientset(secret),
		accessController: authority.NewAccessController(ctrlClient),
	}

	body := `{"name":"download","type":"download","inputs":[{"name":"secret","value":"sec-1"},{"name":"workspace","value":"ws-1"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generateDownloadJob(c, []byte(body))
	testifyassert.NoError(t, err)
	assert.Equal(t, "ws-1", v1.GetWorkspaceId(job))
	assert.Equal(t, "c1", v1.GetClusterId(job))

	// Missing secret param -> bad request.
	body2 := `{"name":"download","type":"download","inputs":[{"name":"workspace","value":"ws-1"}]}`
	c2, _ := newOpsJobCtx(user.Name, body2)
	_, err = h.generateDownloadJob(c2, []byte(body2))
	testifyassert.Error(t, err)
}

// --- merged from ops_job_eval_test.go ---

func TestGenerateEvaluationJobValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	cases := []struct {
		name string
		body string
	}{
		{
			name: "missing serviceId",
			body: `{"name":"eval","type":"evaluation","inputs":[{"name":"eval.service.type","value":"remote_api"}]}`,
		},
		{
			name: "missing serviceType",
			body: `{"name":"eval","type":"evaluation","inputs":[{"name":"eval.service.id","value":"svc-1"}]}`,
		},
		{
			name: "missing benchmarks",
			body: `{"name":"eval","type":"evaluation","inputs":[{"name":"eval.service.id","value":"svc-1"},{"name":"eval.service.type","value":"remote_api"}]}`,
		},
		{
			name: "invalid benchmarks json",
			body: `{"name":"eval","type":"evaluation","inputs":[{"name":"eval.service.id","value":"svc-1"},{"name":"eval.service.type","value":"remote_api"},{"name":"eval.benchmarks","value":"not-json"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newOpsJobCtx(user.Name, tc.body)
			_, err := h.generateEvaluationJob(c, []byte(tc.body))
			testifyassert.Error(t, err)
		})
	}
}

func TestGenerateEvaluationJobDatasetNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user := newAdminHandlerWithObjects()
	mockDB := mock_client.NewMockInterface(ctrl)
	h.dbClient = mockDB
	mockDB.EXPECT().GetDataset(gomock.Any(), "ds-1").Return(nil, assert.AnError)

	body := `{"name":"eval","type":"evaluation","inputs":[` +
		`{"name":"eval.service.id","value":"svc-1"},` +
		`{"name":"eval.service.type","value":"remote_api"},` +
		`{"name":"eval.benchmarks","value":"[{\"datasetId\":\"ds-1\"}]"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	_, err := h.generateEvaluationJob(c, []byte(body))
	testifyassert.Error(t, err)
}

// --- merged from ops_job_generate_test.go ---

func newOpsJobCtx(user string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user)
	return c, rsp
}

func TestGenerateExportImageJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1"},
		Spec:       v1.WorkloadSpec{Workspace: "ws-1", Images: []string{"repo/img:tag"}},
	}
	h, user := newAdminHandlerWithObjects(wl)

	body := `{"name":"export","type":"exportImage","inputs":[{"name":"workload","value":"wl-1"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generateExportImageJob(c, []byte(body))
	testifyassert.NoError(t, err)
	assert.Equal(t, "wl-1", job.Labels[v1.WorkloadIdLabel])

	// Missing workload id in inputs -> bad request.
	body2 := `{"name":"export","type":"exportImage","inputs":[]}`
	c2, _ := newOpsJobCtx(user.Name, body2)
	_, err = h.generateExportImageJob(c2, []byte(body2))
	testifyassert.Error(t, err)
}

func TestGeneratePrewarmImageJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	}
	h, user := newAdminHandlerWithObjects(ws)

	body := `{"name":"prewarm","type":"prewarm","inputs":[{"name":"image","value":"repo/img:tag"},{"name":"workspace","value":"ws-1"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generatePrewarmImageJob(c, []byte(body))
	testifyassert.NoError(t, err)
	assert.Equal(t, "ws-1", job.Labels[v1.WorkspaceIdLabel])
	assert.Equal(t, "c1", job.Labels[v1.ClusterIdLabel])

	// Missing image -> bad request.
	body2 := `{"name":"prewarm","type":"prewarm","inputs":[{"name":"workspace","value":"ws-1"}]}`
	c2, _ := newOpsJobCtx(user.Name, body2)
	_, err = h.generatePrewarmImageJob(c2, []byte(body2))
	testifyassert.Error(t, err)
}

func TestCreateOpsJobRebootHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	body, _ := json.Marshal(view.BaseOpsJobRequest{
		Name:   "reboot-job",
		Type:   v1.OpsJobRebootType,
		Inputs: []v1.Parameter{{Name: "node", Value: "node-1"}},
	})
	c, rsp := newOpsJobCtx(user.Name, string(body))
	h.CreateOpsJob(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Unsupported type -> error.
	body2, _ := json.Marshal(view.BaseOpsJobRequest{
		Name:   "weird-job",
		Type:   v1.OpsJobType("weird"),
		Inputs: []v1.Parameter{{Name: "node", Value: "node-1"}},
	})
	c2, rsp2 := newOpsJobCtx(user.Name, string(body2))
	h.CreateOpsJob(c2)
	testifyassert.NotEqual(t, http.StatusOK, rsp2.Code)
}

func TestDeleteOpsJobHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsJob := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "job-del"}}
	h, user := newAdminHandlerWithObjects(opsJob)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "job-del")
	h.DeleteOpsJob(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

// --- merged from ops_job_handlers_test.go ---

func TestListOpsJobDBDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListOpsJob(c)
	testifyassert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestGetOpsJobDBDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "job-1")
	h.GetOpsJob(c)
	testifyassert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestStopOpsJobNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	// Empty name -> bad request.
	rsp0 := httptest.NewRecorder()
	c0, _ := gin.CreateTestContext(rsp0)
	c0.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c0.Set(common.UserId, user.Name)
	h.StopOpsJob(c0)
	testifyassert.NotEqual(t, http.StatusOK, rsp0.Code)

	// Name set but job not in cluster -> not found.
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "missing-job")
	h.StopOpsJob(c)
	testifyassert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestGenDefaultOpsJob(t *testing.T) {
	user := genMockUser()
	req := &view.BaseOpsJobRequest{
		Name:              "my-job",
		Type:              v1.OpsJobRebootType,
		SecurityOperation: true,
	}
	job := genDefaultOpsJob(req, user)
	assert.Equal(t, "my-job", job.Labels[v1.DisplayNameLabel])
	assert.Equal(t, user.Name, job.Labels[v1.UserIdLabel])
	assert.Equal(t, v1.OpsJobRebootType, job.Spec.Type)
}

func TestGenerateRebootJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)

	body, _ := json.Marshal(view.BaseOpsJobRequest{Name: "reboot-job", Type: v1.OpsJobRebootType})
	job, err := h.generateRebootJob(c, body)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, job)
	assert.Equal(t, v1.OpsJobRebootType, job.Spec.Type)
}

func TestDeleteAdminOpsJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)

	// Empty id -> bad request error.
	_, err := h.deleteAdminOpsJob(c, "")
	testifyassert.Error(t, err)

	// Missing job -> not found is ignored, returns (false, nil).
	found, err := h.deleteAdminOpsJob(c, "missing")
	testifyassert.NoError(t, err)
	testifyassert.False(t, found)
}

// --- merged from ops_job_helpers_test.go ---

func TestParseListOpsJobQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Defaults.
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h := &Handler{}
	q, err := h.parseListOpsJobQuery(c)
	testifyassert.NoError(t, err)
	assert.Equal(t, view.DefaultQueryLimit, q.Limit)
	assert.Equal(t, dbclient.DESC, q.Order)
	testifyassert.False(t, q.SinceTime.IsZero())

	// Invalid until time.
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?until=not-a-time", nil)
	_, err = h.parseListOpsJobQuery(c2)
	testifyassert.Error(t, err)
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
	testifyassert.NotNil(t, sql)
	testifyassert.NotEmpty(t, orderBy)
}

func TestCvtToGetOpsJobSql(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	// Empty job id -> error.
	_, err := h.cvtToGetOpsJobSql(c)
	testifyassert.Error(t, err)

	c.Set(common.Name, "job-1")
	sql, err := h.cvtToGetOpsJobSql(c)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, sql)
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
		testifyassert.NoError(t, h.authGetOpsJob(c, "ws-1", opsType))
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
	testifyassert.NoError(t, err)
	assert.Equal(t, "job", req.Name)

	// Missing inputs.
	body2 := `{"name":"job","type":"reboot"}`
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body2))
	c2.Request.Header.Set("Content-Type", "application/json")
	_, _, err = parseCreateOpsJobRequest(c2)
	testifyassert.Error(t, err)
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
	testifyassert.NotEmpty(t, resp.Inputs)
}

func TestDeserializeParams(t *testing.T) {
	// Short input -> nil.
	testifyassert.Nil(t, deserializeParams(""))
	testifyassert.Nil(t, deserializeParams("[]"))

	params := deserializeParams("[node:n1,workload:wl1]")
	testifyassert.Len(t, params, 2)
	assert.Equal(t, "node", params[0].Name)
	assert.Equal(t, "n1", params[0].Value)
}

func TestGetParametersExcept(t *testing.T) {
	inputs := []v1.Parameter{{Name: "node", Value: "n1"}, {Name: "workload", Value: "wl1"}}
	result := getParametersExcept(inputs, "node")
	testifyassert.Len(t, result, 1)
	assert.Equal(t, "workload", result[0].Name)
}

func TestHasParameters(t *testing.T) {
	inputs := []v1.Parameter{{Name: "node", Value: "n1"}}
	testifyassert.True(t, hasParameters(inputs, "node"))
	testifyassert.True(t, hasParameters(inputs, "missing", "node"))
	testifyassert.False(t, hasParameters(inputs, "workload"))
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

// --- merged from ops_job_nodes_test.go ---

func TestGenerateOpsJobNodesInput(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-1",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"},
		},
	}
	h, _ := newAdminHandlerWithObjects(node)

	// Node param branch -> resolves workspace from the node.
	job := &v1.OpsJob{
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "node-1"}}},
	}
	isSpecified, err := h.generateOpsJobNodesInput(context.Background(), job)
	testifyassert.NoError(t, err)
	testifyassert.True(t, isSpecified)
	assert.Equal(t, "ws-1", v1.GetWorkspaceId(job))

	// No node scope -> bad request.
	emptyJob := &v1.OpsJob{}
	_, err = h.generateOpsJobNodesInput(context.Background(), emptyJob)
	testifyassert.Error(t, err)

	// Node not found -> error.
	badJob := &v1.OpsJob{
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "missing"}}},
	}
	_, err = h.generateOpsJobNodesInput(context.Background(), badJob)
	testifyassert.Error(t, err)
}

// The workspace branch takes the nodes the workspace still holds, not the ones whose label
// still says so. The label is a mirror of Node.Spec.Workspace and lags it by a data-plane
// round trip, and what gets built here is the target list of a job that then runs on those
// machines -- a released node would put this workspace's job onto one another workspace is
// already using.
func TestGenerateOpsJobNodesInputSkipsANodeTheWorkspaceNoLongerHolds(t *testing.T) {
	held := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "node-held", Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"}},
		Spec: v1.NodeSpec{Workspace: pointer.String("ws-1")}}
	// Released to another workspace; only the label has yet to catch up.
	moved := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "node-moved", Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"}},
		Spec: v1.NodeSpec{Workspace: pointer.String("ws-2")}}
	h, _ := newAdminHandlerWithObjects(held, moved)

	job := &v1.OpsJob{
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterWorkspace, Value: "ws-1"}}},
	}
	_, err := h.generateOpsJobNodesInput(context.Background(), job)
	testifyassert.NoError(t, err)
	var targets []string
	for _, p := range job.GetParameters(v1.ParameterNode) {
		targets = append(targets, p.Value)
	}
	assert.Equal(t, []string{"node-held"}, targets)
}

func TestGenerateAddonJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-1",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"},
		},
	}
	h, user := newAdminHandlerWithObjects(node)

	body := `{"name":"addon-job","type":"addon","inputs":[{"name":"node","value":"node-1"}],"batchCount":2}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generateAddonJob(c, []byte(body))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, job)
	assert.Equal(t, "ws-1", v1.GetWorkspaceId(job))
}

func TestGeneratePreflightJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-1",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"},
		},
	}
	h, user := newAdminHandlerWithObjects(node)

	body := `{"name":"preflight-job","type":"preflight","inputs":[{"name":"node","value":"node-1"}],"image":"repo/img:tag"}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generatePreflightJob(c, []byte(body))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, job)
}

func TestGenerateAddonJobNodeNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	body := `{"name":"addon-job","type":"addon","inputs":[{"name":"node","value":"missing"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	_, err := h.generateAddonJob(c, []byte(body))
	testifyassert.Error(t, err)
}
