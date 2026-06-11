/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestGetWorkloadForAuthDBDisabled(t *testing.T) {
	// DB disabled -> falls back to admin (controller-runtime) workload lookup.
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl-1"}}
	h, _ := newAdminHandlerWithObjects(wl)
	got, err := h.getWorkloadForAuth(context.Background(), "wl-1")
	assert.NoError(t, err)
	assert.Equal(t, "wl-1", got.Name)
}

func TestGetAdminFault(t *testing.T) {
	h, _ := newAdminHandlerWithObjects(&v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "fault-1"}})

	_, err := h.getAdminFault(context.Background(), "")
	assert.Error(t, err)

	f, err := h.getAdminFault(context.Background(), "fault-1")
	assert.NoError(t, err)
	assert.Equal(t, "fault-1", f.Name)

	_, err = h.getAdminFault(context.Background(), "missing")
	assert.Error(t, err)
}

func TestInitCustomRouters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	InitCustomRouters(engine, &Handler{})
	assert.NotEmpty(t, engine.Routes())
}

func TestListNodeRebootLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	h, user := newAdminHandlerWithObjects(node)
	mockDB := mock_client.NewMockInterface(ctrl)
	h.dbClient = mockDB
	mockDB.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.OpsJob{{JobId: "job-1"}}, nil)
	mockDB.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(1, nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "node-1")
	h.ListNodeRebootLog(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
