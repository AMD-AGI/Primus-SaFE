/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockdb "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func newWorkloadDBHandler(t *testing.T, ctrl *gomock.Controller) (*Handler, *v1.User, *mockdb.MockInterface) {
	t.Helper()
	commonconfig.SetValue("db.enable", "true")
	t.Cleanup(func() { commonconfig.SetValue("db.enable", "") })

	user := genMockUser()
	role := genMockRole()
	sch := runtime.NewScheme()
	_ = v1.AddToScheme(sch)
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().WithObjects(user, role).WithScheme(sch).Build()
	mockDB := mockdb.NewMockInterface(ctrl)
	h := &Handler{
		Client:           fakeCtrlClient,
		clientSet:        k8sfake.NewSimpleClientset(),
		dbClient:         mockDB,
		accessController: authority.NewAccessController(fakeCtrlClient),
	}
	return h, user, mockDB
}

func TestListWorkloadWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user, mockDB := newWorkloadDBHandler(t, ctrl)
	mockDB.EXPECT().SelectWorkloadsForList(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Workload{{WorkloadId: "wl-1", Workspace: "ws-1", Cluster: "c1"}}, nil).AnyTimes()
	mockDB.EXPECT().CountWorkloads(gomock.Any(), gomock.Any()).Return(1, nil).AnyTimes()
	mockDB.EXPECT().GetWorkloadStatisticsByWorkloadIDs(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?workspaceId=ws-1", nil)
	c.Set(common.UserId, user.Name)
	h.ListWorkload(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetWorkloadWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user, mockDB := newWorkloadDBHandler(t, ctrl)
	mockDB.EXPECT().GetWorkload(gomock.Any(), "wl-1").Return(&dbclient.Workload{
		WorkloadId:  "wl-1",
		Workspace:   "ws-1",
		Cluster:     "c1",
		DisplayName: "WL One",
		UserId:      sql.NullString{String: user.Name, Valid: true},
		GVK:         `{"group":"kubeflow.org","version":"v1","kind":"PyTorchJob"}`,
	}, nil).AnyTimes()
	mockDB.EXPECT().ListWorkloadPods(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockDB.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	h.GetWorkload(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetWorkloadDispatchNodesWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user, mockDB := newWorkloadDBHandler(t, ctrl)
	mockDB.EXPECT().GetWorkload(gomock.Any(), "wl-1").Return(&dbclient.Workload{
		WorkloadId: "wl-1",
		Workspace:  "ws-1",
		Cluster:    "c1",
		UserId:     sql.NullString{String: user.Name, Valid: true},
	}, nil)
	mockDB.EXPECT().GetWorkloadDispatchNode(gomock.Any(), "wl-1", 1).Return(&dbclient.WorkloadDispatchNode{
		WorkloadId:    "wl-1",
		DispatchIndex: 1,
		Nodes:         sql.NullString{String: `["n1","n2","n3"]`, Valid: true},
		Ranks:         sql.NullString{String: `["0","1","2"]`, Valid: true},
	}, nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?dispatchIndex=1&offset=1&limit=1", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	h.GetWorkloadDispatchNodes(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var body map[string]interface{}
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &body))
	assert.Equal(t, float64(3), body["totalCount"])
	assert.Equal(t, []interface{}{"n2"}, body["nodes"])
	assert.Equal(t, []interface{}{"1"}, body["ranks"])
}

func TestGetWorkloadDispatchNodesLegacyFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user, mockDB := newWorkloadDBHandler(t, ctrl)
	mockDB.EXPECT().GetWorkload(gomock.Any(), "wl-legacy").Return(&dbclient.Workload{
		WorkloadId: "wl-legacy",
		Workspace:  "ws-1",
		Cluster:    "c1",
		UserId:     sql.NullString{String: user.Name, Valid: true},
		Nodes:      sql.NullString{String: `[["n0"],["n1","n2","n3"]]`, Valid: true},
		Ranks:      sql.NullString{String: `[["0"],["0","1","2"]]`, Valid: true},
	}, nil)
	mockDB.EXPECT().GetWorkloadDispatchNode(gomock.Any(), "wl-legacy", 1).Return(nil, sql.ErrNoRows)
	mockDB.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), "wl-legacy").Return(nil, nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?dispatchIndex=1&offset=0&limit=2", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-legacy")
	h.GetWorkloadDispatchNodes(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var body map[string]interface{}
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &body))
	assert.Equal(t, float64(3), body["totalCount"])
	assert.Equal(t, []interface{}{"n1", "n2"}, body["nodes"])
	assert.Equal(t, []interface{}{"0", "1"}, body["ranks"])
}

func TestGetWorkloadDispatchNodesLegacyFallbackOnListError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user, mockDB := newWorkloadDBHandler(t, ctrl)
	mockDB.EXPECT().GetWorkload(gomock.Any(), "wl-legacy-list-error").Return(&dbclient.Workload{
		WorkloadId: "wl-legacy-list-error",
		Workspace:  "ws-1",
		Cluster:    "c1",
		UserId:     sql.NullString{String: user.Name, Valid: true},
		Nodes:      sql.NullString{String: `[["n0"],["n1","n2","n3"]]`, Valid: true},
		Ranks:      sql.NullString{String: `[["0"],["0","1","2"]]`, Valid: true},
	}, nil)
	mockDB.EXPECT().GetWorkloadDispatchNode(gomock.Any(), "wl-legacy-list-error", 1).Return(nil, sql.ErrNoRows)
	mockDB.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), "wl-legacy-list-error").Return(nil, sql.ErrConnDone)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?dispatchIndex=1&offset=1&limit=1", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-legacy-list-error")
	h.GetWorkloadDispatchNodes(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var body map[string]interface{}
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &body))
	assert.Equal(t, float64(3), body["totalCount"])
	assert.Equal(t, []interface{}{"n2"}, body["nodes"])
	assert.Equal(t, []interface{}{"1"}, body["ranks"])
}

func TestGetWorkloadDispatchNodesNoLegacyFallbackWhenOffloadExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user, mockDB := newWorkloadDBHandler(t, ctrl)
	mockDB.EXPECT().GetWorkload(gomock.Any(), "wl-mixed").Return(&dbclient.Workload{
		WorkloadId: "wl-mixed",
		Workspace:  "ws-1",
		Cluster:    "c1",
		UserId:     sql.NullString{String: user.Name, Valid: true},
		Nodes:      sql.NullString{String: `[["stale-n0"],["stale-n1"]]`, Valid: true},
		Ranks:      sql.NullString{String: `[["0"],["1"]]`, Valid: true},
	}, nil)
	mockDB.EXPECT().GetWorkloadDispatchNode(gomock.Any(), "wl-mixed", 1).Return(nil, sql.ErrNoRows)
	mockDB.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), "wl-mixed").Return([]*dbclient.WorkloadDispatchNode{
		{WorkloadId: "wl-mixed", DispatchIndex: 0, Nodes: sql.NullString{String: `["fresh-n0"]`, Valid: true}},
	}, nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?dispatchIndex=1&offset=0&limit=100", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-mixed")
	h.GetWorkloadDispatchNodes(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var body map[string]interface{}
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &body))
	assert.Equal(t, float64(0), body["totalCount"])
	assert.Equal(t, []interface{}{}, body["nodes"])
}
