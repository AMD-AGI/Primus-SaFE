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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestConvertToAuditLogItem(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		record   *dbclient.AuditLog
		validate func(*testing.T, view.AuditLogItem)
	}{
		{
			name: "complete record",
			record: &dbclient.AuditLog{
				Id:             1,
				UserId:         "user-123",
				UserName:       sql.NullString{String: "Test User", Valid: true},
				UserType:       sql.NullString{String: "default", Valid: true},
				ClientIP:       sql.NullString{String: "192.168.1.1", Valid: true},
				HttpMethod:     "POST",
				RequestPath:    "/api/v1/workloads",
				ResourceType:   sql.NullString{String: "workloads", Valid: true},
				ResourceName:   sql.NullString{String: "my-workload", Valid: true},
				RequestBody:    sql.NullString{String: `{"name":"test"}`, Valid: true},
				ResponseStatus: 200,
				ResponseBody:   sql.NullString{String: `{"id":"123"}`, Valid: true},
				LatencyMs:      sql.NullInt64{Int64: 150, Valid: true},
				TraceId:        sql.NullString{String: "trace-abc-123", Valid: true},
				CreateTime:     pq.NullTime{Time: now, Valid: true},
			},
			validate: func(t *testing.T, result view.AuditLogItem) {
				assert.Equal(t, int64(1), result.Id)
				assert.Equal(t, "user-123", result.UserId)
				assert.Equal(t, "Test User", result.UserName)
				assert.Equal(t, "default", result.UserType)
				assert.Equal(t, "192.168.1.1", result.ClientIP)
				assert.Equal(t, "POST", result.HttpMethod)
				assert.Equal(t, "/api/v1/workloads", result.RequestPath)
				assert.Equal(t, "workloads", result.ResourceType)
				assert.Equal(t, "my-workload", result.ResourceName)
				assert.Equal(t, `{"name":"test"}`, result.RequestBody)
				assert.Equal(t, 200, result.ResponseStatus)
				assert.Equal(t, int64(150), result.LatencyMs)
				assert.Equal(t, "trace-abc-123", result.TraceId)
				assert.NotEmpty(t, result.CreateTime)
				assert.Equal(t, "create workload my-workload", result.Action)
			},
		},
		{
			name: "minimal record",
			record: &dbclient.AuditLog{
				Id:             2,
				UserId:         "user-456",
				HttpMethod:     "DELETE",
				RequestPath:    "/api/v1/nodes/node-1",
				ResponseStatus: 204,
			},
			validate: func(t *testing.T, result view.AuditLogItem) {
				assert.Equal(t, int64(2), result.Id)
				assert.Equal(t, "user-456", result.UserId)
				assert.Empty(t, result.UserName)
				assert.Empty(t, result.UserType)
				assert.Equal(t, "DELETE", result.HttpMethod)
				assert.Equal(t, 204, result.ResponseStatus)
				assert.Equal(t, "delete", result.Action) // no resource type
			},
		},
		{
			name: "error response",
			record: &dbclient.AuditLog{
				Id:             3,
				UserId:         "user-789",
				HttpMethod:     "POST",
				RequestPath:    "/api/v1/workloads",
				ResponseStatus: 400,
				ResourceType:   sql.NullString{String: "workloads", Valid: true},
				LatencyMs:      sql.NullInt64{Int64: 50, Valid: true},
			},
			validate: func(t *testing.T, result view.AuditLogItem) {
				assert.Equal(t, 400, result.ResponseStatus)
				assert.Equal(t, "create workload", result.Action)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToAuditLogItem(tt.record)
			tt.validate(t, result)
		})
	}
}

func TestListAuditLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()

	t.Run("missing user id returns unauthorized", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		_, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs", nil)

		h.ListAuditLog(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful list with records", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountAuditLogs(gomock.Any(), gomock.Any()).Return(2, nil)
		mockDB.EXPECT().SelectAuditLogs(gomock.Any(), gomock.Any(), gomock.Any(), view.DefaultQueryLimit, 0).Return([]*dbclient.AuditLog{
			{
				Id:             1,
				UserId:         mockUser.Name,
				HttpMethod:     "POST",
				RequestPath:    "/api/v1/workloads",
				ResponseStatus: 200,
				CreateTime:     pq.NullTime{Time: now, Valid: true},
			},
			{
				Id:             2,
				UserId:         mockUser.Name,
				HttpMethod:     "DELETE",
				RequestPath:    "/api/v1/nodes/node-1",
				ResponseStatus: 204,
				CreateTime:     pq.NullTime{Time: now.Add(-time.Hour), Valid: true},
			},
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListAuditLog(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var response view.ListAuditLogResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, 2, response.TotalCount)
		assert.Equal(t, 2, len(response.Items))
	})

	t.Run("successful list with empty results", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountAuditLogs(gomock.Any(), gomock.Any()).Return(0, nil)
		mockDB.EXPECT().SelectAuditLogs(gomock.Any(), gomock.Any(), gomock.Any(), view.DefaultQueryLimit, 0).Return([]*dbclient.AuditLog{}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListAuditLog(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var response view.ListAuditLogResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, 0, response.TotalCount)
		assert.Equal(t, 0, len(response.Items))
	})

	t.Run("list with pagination parameters", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountAuditLogs(gomock.Any(), gomock.Any()).Return(10, nil)
		mockDB.EXPECT().SelectAuditLogs(gomock.Any(), gomock.Any(), gomock.Any(), 5, 5).Return([]*dbclient.AuditLog{
			{
				Id:             6,
				UserId:         mockUser.Name,
				HttpMethod:     "PATCH",
				RequestPath:    "/api/v1/workloads/test",
				ResponseStatus: 200,
				CreateTime:     pq.NullTime{Time: now, Valid: true},
			},
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs?limit=5&offset=5", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListAuditLog(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var response view.ListAuditLogResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, 10, response.TotalCount)
		assert.Equal(t, 1, len(response.Items))
	})

	t.Run("nil db client returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         nil,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListAuditLog(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("database count error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountAuditLogs(gomock.Any(), gomock.Any()).Return(0, assert.AnError)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListAuditLog(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("database select error returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountAuditLogs(gomock.Any(), gomock.Any()).Return(5, nil)
		mockDB.EXPECT().SelectAuditLogs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, assert.AnError)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListAuditLog(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("list with httpMethod filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		mockDB.EXPECT().CountAuditLogs(gomock.Any(), gomock.Any()).Return(1, nil)
		mockDB.EXPECT().SelectAuditLogs(gomock.Any(), gomock.Any(), gomock.Any(), view.DefaultQueryLimit, 0).Return([]*dbclient.AuditLog{
			{
				Id:             1,
				UserId:         mockUser.Name,
				HttpMethod:     "DELETE",
				RequestPath:    "/api/v1/nodes/node-1",
				ResponseStatus: 204,
				CreateTime:     pq.NullTime{Time: now, Valid: true},
			},
		}, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs?httpMethod=DELETE", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListAuditLog(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var response view.ListAuditLogResponse
		err := json.Unmarshal(rsp.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, 1, response.TotalCount)
		assert.Equal(t, "DELETE", response.Items[0].HttpMethod)
	})

	t.Run("list with invalid order returns error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockUser, fakeClient := createMockUser()
		mockDB := mock_client.NewMockInterface(ctrl)

		h := Handler{
			Client:           fakeClient,
			accessController: authority.NewAccessController(fakeClient),
			dbClient:         mockDB,
		}

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs?order=invalid", nil)
		c.Set(common.UserId, mockUser.Name)

		h.ListAuditLog(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

func TestParseListAuditLogQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("default values when no params", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs", nil)

		query, err := parseListAuditLogQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, view.DefaultQueryLimit, query.Limit)
		assert.Equal(t, dbclient.DESC, query.Order)
		assert.Equal(t, dbclient.CreatedTime, query.SortBy)
	})

	t.Run("custom pagination params", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs?limit=50&offset=10", nil)

		query, err := parseListAuditLogQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, 50, query.Limit)
		assert.Equal(t, 10, query.Offset)
	})

	t.Run("custom sort params", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs?sortBy=UserId&order=asc", nil)

		query, err := parseListAuditLogQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, "userid", query.SortBy)
		assert.Equal(t, "asc", query.Order)
	})

	t.Run("zero limit uses default", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs?limit=0", nil)

		query, err := parseListAuditLogQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, view.DefaultQueryLimit, query.Limit)
	})

	t.Run("filter params", func(t *testing.T) {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auditlogs?userId=user-123&resourceType=workloads&httpMethod=POST", nil)

		query, err := parseListAuditLogQuery(c)
		assert.NoError(t, err)
		assert.Equal(t, "user-123", query.UserId)
		assert.Equal(t, "workloads", query.ResourceType)
		assert.Equal(t, "POST", query.HttpMethod)
	})
}

func TestBuildListAuditLogOrderBy(t *testing.T) {
	dbTags := dbclient.GetAuditLogFieldTags()

	t.Run("default sort by create time", func(t *testing.T) {
		req := &view.ListAuditLogRequest{
			Order: dbclient.DESC,
		}
		orderBy := buildListAuditLogOrderBy(req, dbTags)
		assert.Len(t, orderBy, 1)
		assert.Contains(t, orderBy[0], "create_time")
	})

	t.Run("sort by user_id with secondary sort", func(t *testing.T) {
		req := &view.ListAuditLogRequest{
			SortBy: "userid",
			Order:  dbclient.ASC,
		}
		orderBy := buildListAuditLogOrderBy(req, dbTags)
		assert.Len(t, orderBy, 2)
		assert.Contains(t, orderBy[0], "user_id")
		assert.Contains(t, orderBy[1], "create_time")
	})

	t.Run("sort by createtime without duplicate", func(t *testing.T) {
		req := &view.ListAuditLogRequest{
			SortBy: "createtime",
			Order:  dbclient.ASC,
		}
		orderBy := buildListAuditLogOrderBy(req, dbTags)
		assert.Len(t, orderBy, 1)
		assert.Contains(t, orderBy[0], "create_time")
	})

	t.Run("invalid sort field", func(t *testing.T) {
		req := &view.ListAuditLogRequest{
			SortBy: "invalidfield",
			Order:  dbclient.DESC,
		}
		orderBy := buildListAuditLogOrderBy(req, dbTags)
		assert.Len(t, orderBy, 1)
		assert.Contains(t, orderBy[0], "create_time")
	})
}
