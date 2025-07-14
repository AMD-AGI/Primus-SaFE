/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"fmt"
	"strings"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

func (h *Handler) ListFault(c *gin.Context) {
	handle(c, h.listFault)
}

func (h *Handler) listFault(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	query, err := parseListFaultQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}

	ctx := c.Request.Context()
	dbSql := cvtToListFaultSql(query)
	faults, err := h.dbClient.SelectFaults(ctx, dbSql, query.SortBy, query.Order, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	count, err := h.dbClient.CountFaults(ctx, dbSql)
	if err != nil {
		return nil, err
	}
	result := &types.GetFaultResponse{
		TotalCount: count,
	}
	for _, f := range faults {
		result.Items = append(result.Items, cvtToFaultResponse(f))
	}
	return result, nil
}

func parseListFaultQuery(c *gin.Context) (*types.GetFaultRequest, error) {
	query := &types.GetFaultRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.Limit <= 0 {
		query.Limit = types.DefaultQueryLimit
	}
	if query.Order == "" {
		query.Order = dbclient.DESC
	}
	if query.SortBy == "" {
		dbTags := dbclient.GetFaultFieldTags()
		createTime := dbclient.GetFieldTag(dbTags, "CreateTime")
		query.SortBy = createTime
	}
	return query, nil
}

func cvtToListFaultSql(query *types.GetFaultRequest) sqrl.Sqlizer {
	dbTags := dbclient.GetFaultFieldTags()
	monitorId := dbclient.GetFieldTag(dbTags, "MonitorId")
	dbSql := sqrl.And{}
	if query.MonitorId != "" {
		values := strings.Split(query.MonitorId, ",")
		var sqlList []sqrl.Sqlizer
		for _, val := range values {
			sqlList = append(sqlList, sqrl.Eq{monitorId: val})
		}
		dbSql = append(dbSql, sqrl.Or(sqlList))
	} else {
		// 5xx IDs are reserved for internal use and are generally not exposed to external users."
		dbSql = append(dbSql, sqrl.NotLike{monitorId: "5%"})
	}

	if cluster := strings.TrimSpace(query.Cluster); cluster != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Cluster"): cluster})
	}
	if nodeId := strings.TrimSpace(query.NodeId); nodeId != "" {
		dbSql = append(dbSql, sqrl.Like{
			dbclient.GetFieldTag(dbTags, "Node"): fmt.Sprintf("%%%s%%", nodeId)})
	}
	return dbSql
}

func cvtToFaultResponse(f *dbclient.Fault) types.GetFaultResponseItem {
	return types.GetFaultResponseItem{
		ID:          f.UUid,
		FaultId:     f.FaultId,
		MonitorId:   f.MonitorId,
		Message:     dbutils.ParseNullString(f.Message),
		NodeId:      dbutils.ParseNullString(f.Node),
		Action:      dbutils.ParseNullString(f.Action),
		Phase:       dbutils.ParseNullString(f.Phase),
		Cluster:     dbutils.ParseNullString(f.Cluster),
		CreatedTime: dbutils.ParseNullTimeToString(f.CreateTime),
		DeleteTime:  dbutils.ParseNullTimeToString(f.DeleteTime),
	}
}
