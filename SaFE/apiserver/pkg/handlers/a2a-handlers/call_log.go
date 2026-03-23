/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2ahandlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	sqrl "github.com/Masterminds/squirrel"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/a2a-handlers/view"
)

func toCallLogView(log *dbclient.A2ACallLog) view.CallLogView {
	v := view.CallLogView{
		Id:                log.Id,
		TraceId:           log.TraceId,
		CallerServiceName: log.CallerServiceName,
		CallerUserId:      log.CallerUserId,
		TargetServiceName: log.TargetServiceName,
		SkillId:           log.SkillId,
		Status:            log.Status,
		LatencyMs:         log.LatencyMs,
		RequestSizeBytes:  log.RequestSizeBytes,
		ResponseSizeBytes: log.ResponseSizeBytes,
	}
	if log.ErrorMessage.Valid {
		v.ErrorMessage = log.ErrorMessage.String
	}
	if log.CreatedAt.Valid {
		t := log.CreatedAt.Time
		v.CreatedAt = &t
	}
	return v
}

// ListCallLogs handles GET /api/v1/a2a/call-logs
func (h *Handler) ListCallLogs(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	caller := c.Query("caller")
	target := c.Query("target")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit <= 0 {
		limit = 50
	}

	conditions := sqrl.And{}
	if caller != "" {
		conditions = append(conditions, sqrl.Eq{"caller_service_name": caller})
	}
	if target != "" {
		conditions = append(conditions, sqrl.Eq{"target_service_name": target})
	}

	var query sqrl.Sqlizer
	if len(conditions) > 0 {
		query = conditions
	} else {
		query = sqrl.Expr("1=1")
	}

	logs, err := h.dbClient.SelectA2ACallLogs(c.Request.Context(), query, []string{"created_at DESC"}, limit, offset)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError(err.Error()))
		return
	}
	total, _ := h.dbClient.CountA2ACallLogs(c.Request.Context(), query)

	// Build userId -> userName map
	userNameMap := make(map[string]string)
	for _, log := range logs {
		if log.CallerUserId != "" {
			userNameMap[log.CallerUserId] = ""
		}
	}
	for uid := range userNameMap {
		keys, _ := h.dbClient.SelectApiKeys(c.Request.Context(), sqrl.Eq{"user_id": uid}, nil, 1, 0)
		if len(keys) > 0 {
			userNameMap[uid] = keys[0].UserName
		}
	}

	views := make([]view.CallLogView, 0, len(logs))
	for _, log := range logs {
		v := toCallLogView(log)
		v.CallerUserName = userNameMap[log.CallerUserId]
		views = append(views, v)
	}
	c.JSON(http.StatusOK, gin.H{"data": views, "total": total})
}
