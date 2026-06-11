/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// withDBEnabled toggles the global db.enable config for the duration of a test.
func withDBEnabled(t *testing.T) {
	t.Helper()
	commonconfig.SetValue("db.enable", "true")
	t.Cleanup(func() { commonconfig.SetValue("db.enable", "false") })
}

func TestCvtToListFaultSql(t *testing.T) {
	// With explicit filters.
	q := &view.ListFaultRequest{
		MonitorId: "1,2",
		ClusterId: "c1",
		NodeId:    "node-1",
		OnlyOpen:  true,
		SortBy:    "creation_time",
		Order:     dbclient.DESC,
	}
	sql, orderBy := cvtToListFaultSql(q)
	assert.NotNil(t, sql)
	assert.NotEmpty(t, orderBy)

	// Without monitor filter (uses the reserved-id exclusion branch).
	q2 := &view.ListFaultRequest{SortBy: "creation_time", Order: dbclient.DESC}
	sql2, _ := cvtToListFaultSql(q2)
	assert.NotNil(t, sql2)
}

func TestListFaultHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("db disabled returns error", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects()
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(common.UserId, user.Name)
		h.ListFault(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful list", func(t *testing.T) {
		withDBEnabled(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		h, user := newAdminHandlerWithObjects()
		mockDB := mock_client.NewMockInterface(ctrl)
		h.dbClient = mockDB
		mockDB.EXPECT().SelectFaults(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]*dbclient.Fault{{Uid: "f1", MonitorId: "1"}}, nil)
		mockDB.EXPECT().CountFaults(gomock.Any(), gomock.Any()).Return(1, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(common.UserId, user.Name)
		h.ListFault(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}

func TestStopFaultHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "nonexistent-uid")
	h.StopFault(c)
	// No matching fault in the (empty) cluster list -> succeeds with 200.
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestDeleteFaultHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("db disabled returns error", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects()
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		h.DeleteFault(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful delete", func(t *testing.T) {
		withDBEnabled(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		h, user := newAdminHandlerWithObjects()
		mockDB := mock_client.NewMockInterface(ctrl)
		h.dbClient = mockDB
		mockDB.EXPECT().DeleteFault(gomock.Any(), "uid-1").Return(nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "uid-1")
		h.DeleteFault(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}
