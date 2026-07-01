/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"database/sql"
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

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-1")
	h.GetWorkload(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
