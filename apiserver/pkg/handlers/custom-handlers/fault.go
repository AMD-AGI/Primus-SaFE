/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"fmt"
	"strings"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

func (h *Handler) ListFault(c *gin.Context) {
	handle(c, h.listFault)
}

func (h *Handler) DeleteFault(c *gin.Context) {
	handle(c, h.deleteFault)
}

func (h *Handler) StopFault(c *gin.Context) {
	handle(c, h.stopFault)
}

func (h *Handler) listFault(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: v1.FaultKind,
		Verb:         v1.ListVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
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
	result := &types.ListFaultResponse{
		TotalCount: count,
	}
	for _, f := range faults {
		result.Items = append(result.Items, cvtToFaultResponseItem(f))
	}
	return result, nil
}

func (h *Handler) deleteFault(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	_, err := h.stopFault(c)
	if err != nil {
		return nil, err
	}

	id := c.GetString(types.Name)
	err = h.dbClient.DeleteFault(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	klog.Infof("delete fault from db %s", id)
	return nil, nil
}

func (h *Handler) stopFault(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	if err := h.auth.Authorize(authority.Input{
		Context:      ctx,
		ResourceKind: v1.FaultKind,
		Verb:         v1.DeleteVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	id := c.GetString(types.Name)
	faultList := &v1.FaultList{}
	err := h.List(ctx, faultList)
	if err != nil {
		return nil, err
	}
	for _, item := range faultList.Items {
		if string(item.UID) != id {
			continue
		}
		if err = h.Delete(ctx, &item); err != nil {
			return nil, err
		}
		break
	}
	klog.Infof("delete admin fault %s", id)
	return nil, nil
}

func (h *Handler) getAdminFault(ctx context.Context, name string) (*v1.Fault, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the faultId is empty")
	}
	fault := &v1.Fault{}
	if err := h.Get(ctx, client.ObjectKey{Name: name}, fault); err != nil {
		return nil, err
	}
	return fault.DeepCopy(), nil
}

func parseListFaultQuery(c *gin.Context) (*types.ListFaultRequest, error) {
	query := &types.ListFaultRequest{}
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

func cvtToListFaultSql(query *types.ListFaultRequest) sqrl.Sqlizer {
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
	}

	if cluster := strings.TrimSpace(query.Cluster); cluster != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Cluster"): cluster})
	}
	if nodeId := strings.TrimSpace(query.NodeId); nodeId != "" {
		dbSql = append(dbSql, sqrl.Like{
			dbclient.GetFieldTag(dbTags, "Node"): fmt.Sprintf("%%%s%%", nodeId)})
	}
	if query.OnlyOpen {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "DeleteTime"): nil})
	}
	return dbSql
}

func cvtToFaultResponseItem(f *dbclient.Fault) types.FaultResponseItem {
	return types.FaultResponseItem{
		ID:           f.Uid,
		NodeId:       dbutils.ParseNullString(f.Node),
		MonitorId:    f.MonitorId,
		Message:      dbutils.ParseNullString(f.Message),
		Action:       dbutils.ParseNullString(f.Action),
		Phase:        dbutils.ParseNullString(f.Phase),
		Cluster:      dbutils.ParseNullString(f.Cluster),
		CreationTime: dbutils.ParseNullTimeToString(f.CreateTime),
		DeletionTime: dbutils.ParseNullTimeToString(f.DeleteTime),
	}
}
