/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// ListFault handles the listing of fault records.
// It authorizes the request, parses query parameters, and retrieves fault records from the database.
// Returns a list of faults with pagination and filtering support.
func (h *Handler) ListFault(c *gin.Context) {
	handle(c, h.listFault)
}

// DeleteFault handles the deletion of a fault record.
// It first stops the fault (removes it from the k8s cluster) and then deletes the record from the database.
// Returns nil on successful deletion or an error if any step fails.
func (h *Handler) DeleteFault(c *gin.Context) {
	handle(c, h.deleteFault)
}

// StopFault handles removing a fault resource from the k8s cluster.
// It performs authorization checks and deletes the fault resource from the cluster.
func (h *Handler) StopFault(c *gin.Context) {
	handle(c, h.stopFault)
}

// listFault implements the logic for listing fault records from the database.
// It checks if database functionality is enabled, authorizes the request,
// parses query parameters, executes database queries, and formats the response.
func (h *Handler) listFault(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	if err := h.accessController.Authorize(authority.AccessInput{
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
	dbSql, orderBy := cvtToListFaultSql(query)
	faults, err := h.dbClient.SelectFaults(ctx, dbSql, orderBy, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	count, err := h.dbClient.CountFaults(ctx, dbSql)
	if err != nil {
		return nil, err
	}
	result := &view.ListFaultResponse{
		TotalCount: count,
	}
	for _, f := range faults {
		result.Items = append(result.Items, cvtToFaultResponseItem(f))
	}
	return result, nil
}

// deleteFault handles the deletion of a fault record.
// It first stops the fault (removes it from the k8s cluster) and then deletes the record from the database.
// Returns nil on successful deletion or an error if any step fails.
func (h *Handler) deleteFault(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	_, err := h.stopFault(c)
	if err != nil {
		return nil, err
	}

	id := c.GetString(common.Name)
	err = h.dbClient.DeleteFault(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	klog.Infof("delete fault from db %s", id)
	return nil, nil
}

// stopFault removes a fault resource from the k8s cluster.
// It performs authorization checks, lists all fault resources, finds the one matching the given ID,
// and deletes it from the cluster. Returns nil on successful deletion
func (h *Handler) stopFault(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:      ctx,
		ResourceKind: v1.FaultKind,
		Verb:         v1.DeleteVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	id := c.GetString(common.Name)
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

// getAdminFault retrieves a fault resource by ID from the k8s cluster.
// Returns the fault object or an error if the fault doesn't exist or the ID is empty.
func (h *Handler) getAdminFault(ctx context.Context, faultId string) (*v1.Fault, error) {
	if faultId == "" {
		return nil, commonerrors.NewBadRequest("the faultId is empty")
	}
	fault := &v1.Fault{}
	if err := h.Get(ctx, client.ObjectKey{Name: faultId}, fault); err != nil {
		return nil, err
	}
	return fault.DeepCopy(), nil
}

// parseListFaultQuery parses and validates the query parameters for listing faults.
// Sets default values for pagination and sorting if not provided in the request.
func parseListFaultQuery(c *gin.Context) (*view.ListFaultRequest, error) {
	query := &view.ListFaultRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.Limit <= 0 {
		query.Limit = view.DefaultQueryLimit
	}
	if query.Order == "" {
		query.Order = dbclient.DESC
	}
	if query.SortBy == "" {
		dbTags := dbclient.GetFaultFieldTags()
		creationTime := dbclient.GetFieldTag(dbTags, "CreationTime")
		query.SortBy = creationTime
	}
	return query, nil
}

// cvtToListFaultSql converts the fault list query parameters into an SQL query.
// Builds WHERE conditions based on filter parameters like monitor ID, cluster ID, node ID, and open status.
func cvtToListFaultSql(query *view.ListFaultRequest) (sqrl.Sqlizer, []string) {
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

	if clusterId := strings.TrimSpace(query.ClusterId); clusterId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Cluster"): clusterId})
	}
	if nodeId := strings.TrimSpace(query.NodeId); nodeId != "" {
		dbSql = append(dbSql, sqrl.Like{
			dbclient.GetFieldTag(dbTags, "Node"): fmt.Sprintf("%%%s%%", nodeId)})
	}
	if query.OnlyOpen {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "DeletionTime"): nil})
	}
	orderBy := buildOrderBy(query.SortBy, query.Order, dbTags)
	return dbSql, orderBy
}

// cvtToFaultResponseItem converts a database fault record to a response item format.
// Maps database fields to the appropriate response structure.
func cvtToFaultResponseItem(f *dbclient.Fault) view.FaultResponseItem {
	return view.FaultResponseItem{
		ID:           f.Uid,
		NodeId:       dbutils.ParseNullString(f.Node),
		MonitorId:    f.MonitorId,
		Message:      dbutils.ParseNullString(f.Message),
		Action:       dbutils.ParseNullString(f.Action),
		Phase:        dbutils.ParseNullString(f.Phase),
		ClusterId:    dbutils.ParseNullString(f.Cluster),
		CreationTime: dbutils.ParseNullTimeToString(f.CreationTime),
		DeletionTime: dbutils.ParseNullTimeToString(f.DeletionTime),
	}
}
