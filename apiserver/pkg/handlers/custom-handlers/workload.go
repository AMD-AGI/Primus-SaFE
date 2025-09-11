/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	DefaultLogTailLine int64 = 1000
)

func (h *Handler) CreateWorkload(c *gin.Context) {
	handle(c, h.createWorkload)
}

func (h *Handler) ListWorkload(c *gin.Context) {
	handle(c, h.listWorkload)
}

func (h *Handler) GetWorkload(c *gin.Context) {
	handle(c, h.getWorkload)
}

func (h *Handler) DeleteWorkload(c *gin.Context) {
	handle(c, h.deleteWorkload)
}

func (h *Handler) StopWorkload(c *gin.Context) {
	handle(c, h.stopWorkload)
}

func (h *Handler) PatchWorkload(c *gin.Context) {
	handle(c, h.patchWorkload)
}

func (h *Handler) GetWorkloadPodLog(c *gin.Context) {
	handle(c, h.getWorkloadPodLog)
}

func (h *Handler) createWorkload(c *gin.Context) (interface{}, error) {
	req := &types.CreateWorkloadRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		return nil, err
	}
	workload, err := generateWorkload(c, req, body)
	if err != nil {
		return nil, err
	}

	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)
	if err = h.authWorkloadAction(c, workload, v1.CreateVerb, requestUser, roles); err != nil {
		return nil, err
	}
	if err = h.authWorkloadPriority(c, workload, v1.CreateVerb, req.Priority, requestUser, roles); err != nil {
		return nil, err
	}

	if err = h.Create(c.Request.Context(), workload); err != nil {
		return nil, err
	}
	if err = h.patchPhase(c.Request.Context(), workload, v1.WorkloadPending, nil); err != nil {
		return nil, err
	}

	klog.Infof("create workload, name: %s, user: %s, priority: %d, timeout: %d",
		workload.Name, c.GetString(common.UserId), workload.Spec.Priority, workload.Spec.Timeout)
	return &types.CreateWorkloadResponse{WorkloadId: workload.Name}, nil
}
func (h *Handler) listWorkload(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)

	query, err := parseListWorkloadQuery(c)
	if err != nil {
		return nil, err
	}
	adminWorkload := generateAuthWorkload("", "", query.WorkspaceId, query.ClusterId)
	if err = h.authWorkloadAction(c, adminWorkload, v1.ListVerb, requestUser, roles); err != nil {
		return nil, err
	}
	if !commonconfig.IsDBEnable() {
		return h.listAdminWorkloads(c, query)
	}

	dbSql, orderBy, err := cvtToListWorkloadSql(query)
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	workloads, err := h.dbClient.SelectWorkloads(ctx, dbSql, orderBy, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}

	result := &types.ListWorkloadResponse{}
	if result.TotalCount, err = h.dbClient.CountWorkloads(ctx, dbSql); err != nil {
		return nil, err
	}
	for _, w := range workloads {
		workload := h.cvtDBWorkloadToResponseItem(ctx, w)
		result.Items = append(result.Items, workload)
	}
	return result, nil
}

func (h *Handler) getWorkload(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)

	name := c.GetString(types.Name)
	ctx := c.Request.Context()
	if commonconfig.IsDBEnable() {
		dbWorkload, err := h.dbClient.GetWorkload(ctx, name)
		if err != nil {
			return nil, err
		}
		adminWorkload := generateAuthWorkload(name, dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
		if err = h.authWorkloadAction(c, adminWorkload, v1.GetVerb, requestUser, roles); err != nil {
			return nil, err
		}
		return h.cvtDBWorkloadToGetResponse(ctx, dbWorkload), nil
	} else {
		adminWorkload, err := h.getAdminWorkload(ctx, name)
		if err != nil {
			return nil, err
		}
		if err = h.authWorkloadAction(c, adminWorkload, v1.GetVerb, requestUser, roles); err != nil {
			return nil, err
		}
		return h.cvtAdminWorkloadToGetResponse(ctx, adminWorkload), nil
	}
}

func (h *Handler) deleteWorkload(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)

	name := c.GetString(types.Name)
	adminWorkload, err := h.getAdminWorkload(c.Request.Context(), name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, requestUser, roles); err != nil {
			return nil, err
		}
		if err = h.deleteAdminWorkload(c.Request.Context(), adminWorkload); err != nil {
			return nil, err
		}
	}

	if commonconfig.IsDBEnable() {
		dbWorkload, err := h.dbClient.GetWorkload(c.Request.Context(), name)
		if err != nil {
			return nil, commonerrors.IgnoreFound(err)
		}
		adminWorkload = generateAuthWorkload(name, dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, requestUser, roles); err != nil {
			return nil, err
		}
		if err = h.dbClient.SetWorkloadDeleted(c.Request.Context(), name); err != nil {
			return nil, err
		}
	}
	klog.Infof("delete workload %s", name)
	return nil, nil
}

func (h *Handler) deleteAdminWorkload(ctx context.Context, adminWorkload *v1.Workload) error {
	cond := &metav1.Condition{
		Type:    string(v1.AdminStopped),
		Status:  metav1.ConditionTrue,
		Message: "the workload is deleted",
	}

	if err := h.patchPhase(ctx, adminWorkload, v1.WorkloadStopped, cond); err != nil {
		return err
	}
	if err := h.Delete(ctx, adminWorkload); err != nil {
		return err
	}
	return nil
}

func (h *Handler) stopWorkload(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)

	name := c.GetString(types.Name)
	adminWorkload, err := h.getAdminWorkload(c.Request.Context(), name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		if !commonconfig.IsDBEnable() {
			return nil, nil
		}
		dbWorkload, err := h.dbClient.GetWorkload(c.Request.Context(), name)
		if err != nil {
			return nil, commonerrors.IgnoreFound(err)
		}
		if dbutils.ParseNullString(dbWorkload.Phase) != string(v1.WorkloadStopped) {
			adminWorkload = generateAuthWorkload(name,
				dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
			if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, requestUser, roles); err != nil {
				return nil, err
			}
			if err = h.dbClient.SetWorkloadStopped(c.Request.Context(), name); err != nil {
				return nil, err
			}
		}
	} else {
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, requestUser, roles); err != nil {
			return nil, err
		}
		if err = h.deleteAdminWorkload(c.Request.Context(), adminWorkload); err != nil {
			return nil, err
		}
	}
	klog.Infof("stop workload %s", name)
	return nil, nil
}

func (h *Handler) patchWorkload(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)

	name := c.GetString(types.Name)
	adminWorkload, err := h.getAdminWorkload(c.Request.Context(), name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewInternalError("The workload can only be edited when it is running.")
		}
		return nil, err
	}

	req := &types.PatchWorkloadRequest{}
	if _, err = getBodyFromRequest(c.Request, req); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	if err = h.authWorkloadAction(c, adminWorkload, v1.UpdateVerb, requestUser, roles); err != nil {
		return nil, err
	}
	if req.Priority != nil {
		if err = h.authWorkloadPriority(c, adminWorkload, v1.UpdateVerb, *req.Priority, requestUser, roles); err != nil {
			return nil, err
		}
	}

	patch := client.MergeFrom(adminWorkload.DeepCopy())
	updateWorkload(adminWorkload, req)
	if err = h.Patch(c.Request.Context(), adminWorkload, patch); err != nil {
		return nil, err
	}

	klog.Infof("patch workload, name: %s, request: %s", name, string(jsonutils.MarshalSilently(*req)))
	return nil, nil
}

func (h *Handler) getWorkloadPodLog(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)

	workload, err := h.getAdminWorkload(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.authWorkloadAction(c, workload, v1.GetVerb, requestUser, roles); err != nil {
		return nil, err
	}

	k8sClients, err := apiutils.GetK8sClientFactory(h.clientManager, v1.GetClusterId(workload))
	if err != nil {
		return nil, err
	}
	podName := strings.TrimSpace(c.Param(types.PodId))
	podLogs, err := h.getPodLog(c, k8sClients.ClientSet(),
		workload.Spec.Workspace, podName, v1.GetMainContainer(workload))
	if err != nil {
		return nil, err
	}
	return &types.GetWorkloadPodLogResponse{
		WorkloadId: workload.Name,
		PodId:      podName,
		Namespace:  workload.Spec.Workspace,
		Logs:       strings.Split(string(podLogs), "\n"),
	}, nil
}

func (h *Handler) patchPhase(ctx context.Context, workload *v1.Workload,
	phase v1.WorkloadPhase, cond *metav1.Condition) error {
	patch := client.MergeFrom(workload.DeepCopy())
	if phase != "" {
		workload.Status.Phase = phase
		if phase == v1.WorkloadStopped && workload.Status.EndTime == nil {
			workload.Status.EndTime = &metav1.Time{Time: time.Now().UTC()}
		}
	}

	if cond != nil {
		cond.LastTransitionTime = metav1.NewTime(time.Now())
		cond.Reason = commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload))
		if cond2 := workload.GetLastCondition(); cond2 != nil && cond2.Type == cond.Type {
			meta.SetStatusCondition(&workload.Status.Conditions, *cond)
		} else {
			workload.Status.Conditions = append(workload.Status.Conditions, *cond)
		}
	}
	if err := h.Status().Patch(ctx, workload, patch); err != nil {
		return err
	}
	return nil
}

func (h *Handler) listAdminWorkloads(c *gin.Context, query *types.ListWorkloadRequest) (interface{}, error) {
	labelSelector := buildWorkloadLabelSelector(query)
	workloadList := &v1.WorkloadList{}
	if err := h.List(c.Request.Context(), workloadList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	if len(workloadList.Items) > 0 {
		sort.Sort(types.WorkloadSlice(workloadList.Items))
	}
	sinceTime, err := timeutil.CvtStrToRFC3339Milli(query.Since)
	if err != nil {
		return nil, err
	}
	untilTime, err := timeutil.CvtStrToRFC3339Milli(query.Until)
	if err != nil {
		return nil, err
	}

	result := &types.ListWorkloadResponse{}
	for _, w := range workloadList.Items {
		if query.Phase != "" {
			values := strings.Split(query.Kind, ",")
			if !slice.Contains(values, string(w.Status.Phase)) {
				continue
			}
		}
		if query.Description != "" {
			if !strings.Contains(v1.GetDescription(&w), query.Description) {
				continue
			}
		}
		if !sinceTime.IsZero() && w.CreationTimestamp.Time.Before(sinceTime) {
			continue
		}
		if !untilTime.IsZero() && w.CreationTimestamp.Time.After(untilTime) {
			continue
		}
		result.Items = append(result.Items, cvtWorkloadToResponseItem(&w))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) getAdminWorkload(ctx context.Context, name string) (*v1.Workload, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}
	workload := &v1.Workload{}
	if err := h.Get(ctx, client.ObjectKey{Name: name}, workload); err != nil {
		return nil, err
	}
	return workload.DeepCopy(), nil
}

func (h *Handler) getWorkloadInternal(ctx context.Context, workloadId string) (*v1.Workload, error) {
	if !commonconfig.IsDBEnable() {
		return h.getAdminWorkload(ctx, workloadId)
	}
	dbWorkload, err := h.dbClient.GetWorkload(ctx, workloadId)
	if err != nil {
		return nil, err
	}
	adminWorkload := generateAuthWorkload(workloadId, dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
	adminWorkload.CreationTimestamp = metav1.NewTime(dbutils.ParseNullTime(dbWorkload.CreateTime))
	endTime := dbutils.ParseNullTime(dbWorkload.EndTime)
	if !endTime.IsZero() {
		adminWorkload.Status.EndTime = &metav1.Time{Time: endTime}
	}
	return adminWorkload, nil
}

func (h *Handler) getRunningWorkloads(ctx context.Context, clusterName string, workspaceNames []string) ([]*v1.Workload, error) {
	filterFunc := func(w *v1.Workload) bool {
		if w.IsEnd() || !v1.IsWorkloadDispatched(w) {
			return true
		}
		return false
	}
	return commonworkload.GetWorkloadsOfWorkspace(ctx, h.Client, clusterName, workspaceNames, filterFunc)
}

func (h *Handler) authWorkloadAction(c *gin.Context,
	adminWorkload *v1.Workload, verb v1.RoleVerb, requestUser *v1.User, roles []*v1.Role) error {
	var workspaces []string
	if adminWorkload.Spec.Workspace != "" {
		workspaces = append(workspaces, adminWorkload.Spec.Workspace)
	}
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: v1.WorkloadKind,
		Resource:     adminWorkload,
		Verb:         verb,
		Workspaces:   workspaces,
		User:         requestUser,
		UserId:       c.GetString(common.UserId),
		Roles:        roles,
	}); err != nil {
		return err
	}
	return nil
}

func (h *Handler) authWorkloadPriority(c *gin.Context, adminWorkload *v1.Workload,
	verb v1.RoleVerb, priority int, requestUser *v1.User, roles []*v1.Role) error {
	priorityKind := fmt.Sprintf("workload/%s", commonworkload.GeneratePriority(priority))
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: priorityKind,
		Verb:         verb,
		Workspaces:   []string{adminWorkload.Spec.Workspace},
		User:         requestUser,
		Roles:        roles,
	}); err != nil {
		return err
	}
	return nil
}

func generateWorkload(c *gin.Context, req *types.CreateWorkloadRequest, body []byte) (*v1.Workload, error) {
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(req.DisplayName),
			Labels: map[string]string{
				v1.DisplayNameLabel: req.DisplayName,
				v1.UserIdLabel:      c.GetString(common.UserId),
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation: req.Description,
				v1.UserNameAnnotation:    c.GetString(common.UserName),
			},
		},
	}
	var err error
	if err = json.Unmarshal(body, &workload.Spec); err != nil {
		return nil, err
	}

	if len(workload.Spec.CustomerLabels) > 0 {
		customerLabels := make(map[string]string)
		for key, val := range workload.Spec.CustomerLabels {
			if len(val) == 0 {
				continue
			}
			if key != common.K8sHostNameLabel {
				key = common.CustomerLabelPrefix + key
			}
			customerLabels[key] = val
		}
		workload.Spec.CustomerLabels = customerLabels
	}
	return workload, nil
}

func parseListWorkloadQuery(c *gin.Context) (*types.ListWorkloadRequest, error) {
	query := &types.ListWorkloadRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.UserName != "" {
		if nameUnescape, err := url.QueryUnescape(query.UserName); err == nil {
			query.UserName = nameUnescape
		}
	}
	if query.Limit <= 0 {
		query.Limit = types.DefaultQueryLimit
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.Order == "" {
		query.Order = dbclient.DESC
	}
	if query.Description != "" {
		if desUnescape, err := url.QueryUnescape(query.Description); err == nil {
			query.Description = desUnescape
		}
	}
	return query, nil
}

func parseGetPodLogQuery(c *gin.Context, mainContainerName string) (*types.GetPodLogRequest, error) {
	query := &types.GetPodLogRequest{}
	var err error
	if err = c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.TailLines <= 0 {
		query.TailLines = DefaultLogTailLine
	}
	if query.Container == "" {
		query.Container = mainContainerName
	}
	return query, nil
}

func cvtToListWorkloadSql(query *types.ListWorkloadRequest) (sqrl.Sqlizer, []string, error) {
	dbTags := dbclient.GetWorkloadFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
	}
	if clusterId := strings.TrimSpace(query.ClusterId); clusterId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Cluster"): clusterId})
	}
	if workspaceId := strings.TrimSpace(query.WorkspaceId); workspaceId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Workspace"): workspaceId})
	}
	if query.Phase != "" {
		values := strings.Split(query.Phase, ",")
		var sqlList []sqrl.Sqlizer
		for _, val := range values {
			sqlList = append(sqlList, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Phase"): val})
		}
		dbSql = append(dbSql, sqrl.Or(sqlList))
	}
	if description := strings.TrimSpace(query.Description); description != "" {
		dbSql = append(dbSql,
			sqrl.Like{dbclient.GetFieldTag(dbTags, "Description"): fmt.Sprintf("%%%s%%", description)})
	}
	userNameField := dbclient.GetFieldTag(dbTags, "UserName")
	if userName := strings.TrimSpace(query.UserName); userName != "" {
		dbSql = append(dbSql, sqrl.Like{userNameField: fmt.Sprintf("%%%s%%", userName)})
	} else {
		userCondition := sqrl.Or{
			sqrl.NotEq{userNameField: common.UserSystem},        // username != 'system'
			sqrl.Expr(fmt.Sprintf("%s IS NULL", userNameField)), // username IS NULL
		}
		dbSql = append(dbSql, userCondition)
	}
	if userId := strings.TrimSpace(query.UserId); userId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "UserId"): userId})
	}
	if sinceTime := strings.TrimSpace(query.Since); sinceTime != "" {
		if t, err := timeutil.CvtStrToRFC3339Milli(sinceTime); err == nil {
			dbSql = append(dbSql, sqrl.GtOrEq{dbclient.GetFieldTag(dbTags, "CreateTime"): t})
		} else {
			klog.ErrorS(err, "failed to parse since time")
		}
	}
	if untilTime := strings.TrimSpace(query.Until); untilTime != "" {
		if t, err := timeutil.CvtStrToRFC3339Milli(untilTime); err == nil {
			dbSql = append(dbSql, sqrl.LtOrEq{dbclient.GetFieldTag(dbTags, "CreateTime"): t})
		} else {
			klog.ErrorS(err, "failed to parse until time")
		}
	}
	if kind := strings.TrimSpace(query.Kind); kind != "" {
		values := strings.Split(query.Kind, ",")
		var sqlList []sqrl.Sqlizer
		for _, val := range values {
			gvk := v1.GroupVersionKind{Kind: val, Version: v1.SchemeGroupVersion.Version}
			gvkStr := string(jsonutils.MarshalSilently(gvk))
			sqlList = append(sqlList, sqrl.Eq{dbclient.GetFieldTag(dbTags, "GVK"): gvkStr})
		}
		dbSql = append(dbSql, sqrl.Or(sqlList))
	}
	orderBy := buildListWorkloadOrderBy(query, dbTags)
	return dbSql, orderBy, nil
}

func buildListWorkloadOrderBy(query *types.ListWorkloadRequest, dbTags map[string]string) []string {
	var nullOrder string
	if query.Order == dbclient.DESC {
		nullOrder = "NULLS FIRST"
	} else {
		nullOrder = "NULLS LAST"
	}
	createTime := dbclient.GetFieldTag(dbTags, "CreateTime")

	var orderBy []string
	isSortByCreatedTime := false
	if query.SortBy != "" {
		sortBy := strings.TrimSpace(query.SortBy)
		sortBy = dbclient.GetFieldTag(dbTags, sortBy)
		if sortBy != "" {
			if stringutil.StrCaseEqual(query.SortBy, createTime) {
				isSortByCreatedTime = true
			}
			orderBy = append(orderBy, fmt.Sprintf("%s %s %s", sortBy, query.Order, nullOrder))
		}
	}
	if !isSortByCreatedTime {
		orderBy = append(orderBy, fmt.Sprintf("%s %s", createTime, dbclient.DESC))
	}
	return orderBy
}

func updateWorkload(adminWorkload *v1.Workload, req *types.PatchWorkloadRequest) {
	if req.Priority != nil {
		adminWorkload.Spec.Priority = *req.Priority
	}
	if req.Replica != nil {
		adminWorkload.Spec.Resource.Replica = *req.Replica
	}
	if req.CPU != nil {
		adminWorkload.Spec.Resource.CPU = *req.CPU
	}
	if req.GPU != nil {
		adminWorkload.Spec.Resource.GPU = *req.GPU
	}
	if req.Memory != nil {
		adminWorkload.Spec.Resource.Memory = *req.Memory
	}
	if req.EphemeralStorage != nil {
		adminWorkload.Spec.Resource.EphemeralStorage = *req.EphemeralStorage
	}
	if req.SharedMemory != nil {
		adminWorkload.Spec.Resource.SharedMemory = *req.SharedMemory
	}
	if req.Image != nil && *req.Image != "" {
		adminWorkload.Spec.Image = *req.Image
	}
	if req.EntryPoint != nil && *req.EntryPoint != "" {
		adminWorkload.Spec.EntryPoint = *req.EntryPoint
	}
	if req.Description != nil {
		v1.SetAnnotation(adminWorkload, v1.DescriptionAnnotation, *req.Description)
	}
	if req.Timeout != nil {
		adminWorkload.Spec.Timeout = pointer.Int(*req.Timeout)
	}
	if req.Env != nil {
		for key, val := range *req.Env {
			adminWorkload.Spec.Env[key] = val
		}
	}
}

func (h *Handler) cvtDBWorkloadToResponseItem(ctx context.Context,
	w *dbclient.Workload) types.WorkloadResponseItem {
	result := types.WorkloadResponseItem{
		WorkloadId:     w.WorkloadId,
		Workspace:      w.Workspace,
		Cluster:        w.Cluster,
		Phase:          dbutils.ParseNullString(w.Phase),
		CreationTime:   dbutils.ParseNullTimeToString(w.CreateTime),
		StartTime:      dbutils.ParseNullTimeToString(w.StartTime),
		EndTime:        dbutils.ParseNullTimeToString(w.EndTime),
		DeletionTime:   dbutils.ParseNullTimeToString(w.DeleteTime),
		SchedulerOrder: w.SchedulerOrder,
		DispatchCount:  w.DispatchCount,
		DisplayName:    w.DisplayName,
		Description:    dbutils.ParseNullString(w.Description),
		UserId:         dbutils.ParseNullString(w.UserId),
		UserName:       dbutils.ParseNullString(w.UserName),
		Priority:       w.Priority,
		IsTolerateAll:  w.IsTolerateAll,
		WorkloadUid:    dbutils.ParseNullString(w.WorkloadUId),
	}
	if result.EndTime == "" && result.DeletionTime != "" {
		result.EndTime = result.DeletionTime
	}
	json.Unmarshal([]byte(w.GVK), &result.GroupVersionKind)
	json.Unmarshal([]byte(w.Resource), &result.Resource)
	if w.Timeout > 0 {
		result.Timeout = pointer.Int(w.Timeout)
		if t := dbutils.ParseNullTime(w.StartTime); !t.IsZero() {
			result.SecondsUntilTimeout = t.Unix() + int64(3600*w.Timeout) - time.Now().Unix()
			if result.SecondsUntilTimeout < 0 {
				result.SecondsUntilTimeout = 0
			}
		}
	}
	if result.Phase == string(v1.WorkloadPending) {
		adminWorkload, err := h.getAdminWorkload(ctx, result.WorkloadId)
		if err == nil {
			result.Message = adminWorkload.Status.Message
		}
	}
	return result
}

func (h *Handler) cvtDBWorkloadToGetResponse(ctx context.Context, w *dbclient.Workload) *types.GetWorkloadResponse {
	result := &types.GetWorkloadResponse{
		WorkloadResponseItem: h.cvtDBWorkloadToResponseItem(ctx, w),
		Image:                w.Image,
		IsSupervised:         w.IsSupervised,
		MaxRetry:             w.MaxRetry,
	}
	if result.GroupVersionKind.Kind != common.AuthoringKind && w.EntryPoint != "" {
		if stringutil.IsBase64(w.EntryPoint) {
			result.EntryPoint = stringutil.Base64Decode(w.EntryPoint)
		}
	}
	if w.TTLSecond > 0 {
		result.TTLSecondsAfterFinished = pointer.Int(w.TTLSecond)
	}
	if str := dbutils.ParseNullString(w.Conditions); str != "" {
		json.Unmarshal([]byte(str), &result.Conditions)
	}
	if str := dbutils.ParseNullString(w.Pods); str != "" {
		json.Unmarshal([]byte(str), &result.Pods)
		for i, p := range result.Pods {
			result.Pods[i].SSHAddr = h.buildSSHAddress(ctx, &p.WorkloadPod, result.UserId, result.Workspace)
		}
	}
	if str := dbutils.ParseNullString(w.Nodes); str != "" {
		json.Unmarshal([]byte(str), &result.Nodes)
	}
	if str := dbutils.ParseNullString(w.CustomerLabels); str != "" {
		var customerLabels map[string]string
		json.Unmarshal([]byte(str), &customerLabels)
		if len(customerLabels) > 0 {
			result.CustomerLabels = make(map[string]string)
			for key, val := range customerLabels {
				if strings.HasPrefix(key, common.CustomerLabelPrefix) {
					key = key[len(common.CustomerLabelPrefix):]
				}
				result.CustomerLabels[key] = val
			}
		}
	}
	if str := dbutils.ParseNullString(w.Liveness); str != "" {
		json.Unmarshal([]byte(str), &result.Liveness)
	}
	if str := dbutils.ParseNullString(w.Readiness); str != "" {
		json.Unmarshal([]byte(str), &result.Readiness)
	}
	if str := dbutils.ParseNullString(w.Service); str != "" {
		json.Unmarshal([]byte(str), &result.Service)
	}
	if str := dbutils.ParseNullString(w.Env); str != "" {
		json.Unmarshal([]byte(str), &result.Env)
		result.Env = maps.RemoveValue(result.Env, "")
	}
	return result
}

func (h *Handler) cvtAdminWorkloadToGetResponse(ctx context.Context, w *v1.Workload) *types.GetWorkloadResponse {
	result := &types.GetWorkloadResponse{
		WorkloadResponseItem:    cvtWorkloadToResponseItem(w),
		Image:                   w.Spec.Image,
		IsSupervised:            w.Spec.IsSupervised,
		MaxRetry:                w.Spec.MaxRetry,
		Conditions:              w.Status.Conditions,
		Nodes:                   w.Status.Nodes,
		TTLSecondsAfterFinished: w.Spec.TTLSecondsAfterFinished,
		Service:                 w.Spec.Service,
		Liveness:                w.Spec.Liveness,
		Readiness:               w.Spec.Readiness,
		Env:                     w.Spec.Env,
	}

	result.Pods = make([]types.WorkloadPodWrapper, len(w.Status.Pods))
	for i, p := range w.Status.Pods {
		result.Pods[i].WorkloadPod = w.Status.Pods[i]
		result.Pods[i].SSHAddr = h.buildSSHAddress(ctx, &p, result.UserId, result.Workspace)
	}
	if len(w.Spec.CustomerLabels) > 0 {
		result.CustomerLabels = make(map[string]string)
		for key, val := range w.Spec.CustomerLabels {
			if strings.HasPrefix(key, common.CustomerLabelPrefix) {
				key = key[len(common.CustomerLabelPrefix):]
			}
			result.CustomerLabels[key] = val
		}
	}
	if !commonworkload.IsAuthoring(w) {
		result.EntryPoint = stringutil.Base64Decode(w.Spec.EntryPoint)
	}
	return result
}

func cvtWorkloadToResponseItem(w *v1.Workload) types.WorkloadResponseItem {
	result := types.WorkloadResponseItem{
		WorkloadId:       w.Name,
		Workspace:        w.Spec.Workspace,
		Resource:         w.Spec.Resource,
		DisplayName:      v1.GetDisplayName(w),
		Description:      v1.GetDescription(w),
		UserId:           v1.GetUserId(w),
		UserName:         v1.GetUserName(w),
		Cluster:          v1.GetClusterId(w),
		Phase:            string(w.Status.Phase),
		Priority:         w.Spec.Priority,
		CreationTime:     timeutil.FormatRFC3339(&w.CreationTimestamp.Time),
		SchedulerOrder:   w.Status.SchedulerOrder,
		DispatchCount:    v1.GetWorkloadDispatchCnt(w),
		IsTolerateAll:    w.Spec.IsTolerateAll,
		GroupVersionKind: w.Spec.GroupVersionKind,
		Timeout:          w.Spec.Timeout,
		WorkloadUid:      string(w.UID),
	}
	if !w.Status.StartTime.IsZero() {
		result.StartTime = timeutil.FormatRFC3339(&w.Status.StartTime.Time)
		if w.Spec.Timeout != nil {
			result.SecondsUntilTimeout = w.Status.StartTime.Unix() + int64(3600*w.GetTimeout()) - time.Now().Unix()
		}
	}
	if !w.Status.EndTime.IsZero() {
		result.EndTime = timeutil.FormatRFC3339(&w.Status.EndTime.Time)
	}
	if result.Phase == string(v1.WorkloadPending) {
		result.Message = w.Status.Message
	}
	return result
}

func (h *Handler) buildSSHAddress(ctx context.Context, pod *v1.WorkloadPod, userId, workspace string) string {
	if !commonconfig.IsSSHEnable() || pod.Phase != corev1.PodRunning {
		return ""
	}
	if userId == "" {
		userId = "none"
	}
	ep, err := h.clientSet.CoreV1().Endpoints(common.HigressNamespace).Get(ctx, common.HigressGateway, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to get higress gateway")
		return ""
	}

	gatewayIp := ""
	for _, sub := range ep.Subsets {
		isMatch := false
		for _, p := range sub.Ports {
			if p.Port == common.HigressSSHPort && p.Protocol == corev1.ProtocolTCP {
				isMatch = true
				break
			}
		}
		if !isMatch {
			continue
		}
		if len(sub.Addresses) > 0 {
			gatewayIp = sub.Addresses[0].IP
			break
		}
	}
	if gatewayIp != "" {
		return fmt.Sprintf("ssh %s.%s.%s@%s", userId, pod.PodId, workspace, gatewayIp)
	}

	localIp, _ := netutil.GetLocalIp()
	if localIp == "" {
		return ""
	}
	return fmt.Sprintf("ssh -p %d %s.%s.%s@%s",
		commonconfig.GetSSHServerPort(), userId, pod.PodId, workspace, localIp)
}

func buildWorkloadLabelSelector(query *types.ListWorkloadRequest) labels.Selector {
	var labelSelector = labels.NewSelector()
	if query.WorkspaceId != "" {
		req, _ := labels.NewRequirement(v1.WorkspaceIdLabel, selection.Equals, []string{query.WorkspaceId})
		labelSelector = labelSelector.Add(*req)
	}
	if query.ClusterId != "" {
		req, _ := labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{query.ClusterId})
		labelSelector = labelSelector.Add(*req)
	}
	if query.UserName != "" {
		nameMd5 := stringutil.MD5(query.UserName)
		req, _ := labels.NewRequirement(v1.UserNameMd5Label, selection.Equals, []string{nameMd5})
		labelSelector = labelSelector.Add(*req)
	} else {
		nameMd5 := stringutil.MD5(common.UserSystem)
		req, _ := labels.NewRequirement(v1.UserNameMd5Label, selection.NotEquals, []string{nameMd5})
		labelSelector = labelSelector.Add(*req)
	}
	if query.Kind != "" {
		values := strings.Split(query.Kind, ",")
		req, _ := labels.NewRequirement(v1.WorkloadKindLabel, selection.In, values)
		labelSelector = labelSelector.Add(*req)
	}
	if query.UserId != "" {
		req, _ := labels.NewRequirement(v1.UserIdLabel, selection.Equals, []string{query.UserId})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector
}
func generateAuthWorkload(name, userId, workspace, clusterId string) *v1.Workload {
	return &v1.Workload{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.WorkloadKind,
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.UserIdLabel:    userId,
				v1.ClusterIdLabel: clusterId,
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
		},
	}
}
