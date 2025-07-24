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
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	workload, err := generateWorkload(req, body)
	if err != nil {
		klog.ErrorS(err, "failed to generate workload")
		return nil, err
	}
	ctx := c.Request.Context()
	if err = h.Create(ctx, workload); err != nil {
		klog.ErrorS(err, "failed to create workload")
		return nil, err
	}
	if err = h.patchPhase(ctx, workload, v1.WorkloadPending, nil); err != nil {
		klog.ErrorS(err, "failed to patch workload phase")
		return nil, err
	}

	klog.Infof("create workload, name: %s, user: %s, priority: %d, timeout: %d",
		workload.Name, req.UserName, workload.Spec.Priority, workload.Spec.Timeout)
	return &types.CreateWorkloadResponse{WorkloadId: workload.Name}, nil
}

func (h *Handler) listWorkload(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return h.listAdminWorkloads(c)
	}

	query, err := parseListWorkloadQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
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

	result := &types.GetWorkloadResponse{}
	if result.TotalCount, err = h.dbClient.CountWorkloads(ctx, dbSql); err != nil {
		return nil, err
	}

	for _, w := range workloads {
		workload := h.cvtDBWorkloadToResponse(ctx, w, false)
		result.Items = append(result.Items, workload)
	}
	return result, nil
}

func (h *Handler) getWorkload(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("workloadId is empty")
	}
	ctx := c.Request.Context()
	if commonconfig.IsDBEnable() {
		workload, err := h.dbClient.GetWorkload(ctx, name)
		if err != nil {
			return nil, err
		}
		return h.cvtDBWorkloadToResponse(ctx, workload, true), nil
	} else {
		adminWorkload, err := h.getAdminWorkload(ctx, name)
		if err != nil {
			return nil, err
		}
		return h.cvtAdminWorkloadToResponse(ctx, adminWorkload, true), nil
	}
}

func (h *Handler) deleteWorkload(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("workloadId is empty")
	}
	ctx := c.Request.Context()
	adminWorkload, err := h.getAdminWorkload(ctx, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		if err = h.deleteAdminWorkload(c, adminWorkload); err != nil {
			return nil, err
		}
	}
	if commonconfig.IsDBEnable() {
		if err = h.dbClient.SetWorkloadDeleted(ctx, name); err != nil {
			return nil, err
		}
	}
	klog.Infof("delete workload %s", name)
	return nil, nil
}

func (h *Handler) deleteAdminWorkload(c *gin.Context, adminWorkload *v1.Workload) error {
	ctx := c.Request.Context()
	cond := &metav1.Condition{
		Type:    string(v1.AdminStopped),
		Status:  metav1.ConditionTrue,
		Message: "the workload is deleted",
	}

	if err := h.patchPhase(ctx, adminWorkload, v1.WorkloadStopped, cond); err != nil {
		klog.ErrorS(err, "failed to patch workload phase")
		return err
	}
	if err := h.Delete(ctx, adminWorkload); err != nil {
		klog.ErrorS(err, "failed to delete workload")
		return err
	}
	return nil
}

func (h *Handler) stopWorkload(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("workloadId is empty")
	}
	ctx := c.Request.Context()
	adminWorkload, err := h.getAdminWorkload(ctx, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		if commonconfig.IsDBEnable() {
			if err = h.dbClient.SetWorkloadStopped(ctx, name); err != nil {
				return nil, err
			}
		}
	} else {
		if err = h.deleteAdminWorkload(c, adminWorkload); err != nil {
			return nil, err
		}
	}
	klog.Infof("stop workload %s", name)
	return nil, nil
}

func (h *Handler) patchWorkload(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("workloadId is empty")
	}

	ctx := c.Request.Context()
	adminWorkload, err := h.getAdminWorkload(ctx, name)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}
	req := &types.PatchWorkloadRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	if adminWorkload != nil {
		patch := client.MergeFrom(adminWorkload.DeepCopy())
		updateWorkload(adminWorkload, req)
		if err = h.Patch(ctx, adminWorkload, patch); err != nil {
			klog.ErrorS(err, "failed to patch workload")
			return nil, err
		}
	} else if commonconfig.IsDBEnable() {
		if req.Description == nil || *req.Description == "" {
			return nil, fmt.Errorf("The terminated workload can only modify the description")
		}
		if err = h.dbClient.SetWorkloadDescription(ctx, name, *req.Description); err != nil {
			return nil, err
		}
	}
	klog.Infof("patch workload, name: %s, request: %s", name, string(jsonutils.MarshalSilently(*req)))
	return nil, nil
}

func (h *Handler) getWorkloadPodLog(c *gin.Context) (interface{}, error) {
	workload, err := h.getAdminWorkload(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
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
		klog.ErrorS(err, "failed to patch workload status", "name", workload.Name)
		return err
	}
	return nil
}
func (h *Handler) listAdminWorkloads(c *gin.Context) (interface{}, error) {
	query, err := parseListWorkloadQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}

	ctx := c.Request.Context()
	labelSelector := buildWorkloadLabelSelector(query)
	workloadList := &v1.WorkloadList{}
	if err = h.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
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
	result := &types.GetWorkloadResponse{}
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
		result.Items = append(result.Items, h.cvtAdminWorkloadToResponse(ctx, &w, false))
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
		klog.ErrorS(err, "failed to get admin workload", "name", name)
		return nil, err
	}
	return workload.DeepCopy(), nil
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

func generateWorkload(req *types.CreateWorkloadRequest, body []byte) (*v1.Workload, error) {
	if req.UserName == v1.SystemUser {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("The %s user is a reserved system account.", v1.SystemUser))
	}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(req.DisplayName),
			Labels: map[string]string{
				v1.DisplayNameLabel: req.DisplayName,
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation: req.Description,
				v1.UserNameAnnotation:    req.UserName,
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

func parseListWorkloadQuery(c *gin.Context) (*types.GetWorkloadRequest, error) {
	query := &types.GetWorkloadRequest{}
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

func cvtToListWorkloadSql(query *types.GetWorkloadRequest) (sqrl.Sqlizer, []string, error) {
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
	if userName := strings.TrimSpace(query.UserName); userName != "" {
		dbSql = append(dbSql, sqrl.Like{
			dbclient.GetFieldTag(dbTags, "UserName"): fmt.Sprintf("%%%s%%", userName)})
	}
	if sinceTime := strings.TrimSpace(query.Since); sinceTime != "" {
		if t, err := timeutil.CvtStrToRFC3339Milli(sinceTime); err == nil {
			dbSql = append(dbSql, sqrl.GtOrEq{dbclient.GetFieldTag(dbTags, "CreateTime"): t})
		}
	}
	if untilTime := strings.TrimSpace(query.Until); untilTime != "" {
		if t, err := timeutil.CvtStrToRFC3339Milli(untilTime); err == nil {
			dbSql = append(dbSql, sqrl.LtOrEq{dbclient.GetFieldTag(dbTags, "CreateTime"): t})
		}
	}
	if kind := strings.TrimSpace(query.Kind); kind != "" {
		values := strings.Split(query.Kind, ",")
		var sqlList []sqrl.Sqlizer
		for _, val := range values {
			gvk := v1.GroupVersionKind{Kind: val, Version: common.DefaultVersion}
			gvkStr := string(jsonutils.MarshalSilently(gvk))
			sqlList = append(sqlList, sqrl.Eq{dbclient.GetFieldTag(dbTags, "GVK"): gvkStr})
		}
		dbSql = append(dbSql, sqrl.Or(sqlList))
	}
	orderBy := buildListWorkloadOrderBy(query, dbTags)
	return dbSql, orderBy, nil
}

func buildListWorkloadOrderBy(query *types.GetWorkloadRequest, dbTags map[string]string) []string {
	var nullOrder string
	if query.Order == dbclient.DESC {
		nullOrder = "NULLS FIRST"
	} else {
		nullOrder = "NULLS LAST"
	}
	createTime := dbclient.GetFieldTag(dbTags, "CreateTime")

	var orderBy []string
	hasOrderByCreatedTime := false
	if query.SortBy != "" {
		sortBy := strings.TrimSpace(query.SortBy)
		sortBy = dbclient.GetFieldTag(dbTags, sortBy)
		if sortBy != "" {
			if stringutil.StrCaseEqual(query.SortBy, createTime) {
				hasOrderByCreatedTime = true
			}
			orderBy = append(orderBy, fmt.Sprintf("%s %s %s", sortBy, query.Order, nullOrder))
		}
	}
	if !hasOrderByCreatedTime {
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

func (h *Handler) cvtDBWorkloadToResponse(ctx context.Context,
	w *dbclient.Workload, isNeedDetail bool) types.GetWorkloadResponseItem {
	result := types.GetWorkloadResponseItem{
		WorkloadId:     w.WorkloadId,
		Cluster:        w.Cluster,
		Phase:          dbutils.ParseNullString(w.Phase),
		CreationTime:   dbutils.ParseNullTimeToString(w.CreateTime),
		StartTime:      dbutils.ParseNullTimeToString(w.StartTime),
		EndTime:        dbutils.ParseNullTimeToString(w.EndTime),
		DeletionTime:   dbutils.ParseNullTimeToString(w.DeleteTime),
		SchedulerOrder: w.SchedulerOrder,
		DispatchCount:  w.DispatchCount,
		CreateWorkloadRequest: types.CreateWorkloadRequest{
			DisplayName: w.DisplayName,
			Description: dbutils.ParseNullString(w.Description),
			UserName:    dbutils.ParseNullString(w.UserName),
			WorkloadSpec: v1.WorkloadSpec{
				Priority:      w.Priority,
				Workspace:     w.Workspace,
				IsTolerateAll: w.IsTolerateAll,
			},
		},
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
	if isNeedDetail {
		h.buildWorkloadDetail(ctx, w, &result)
	}
	return result
}

func (h *Handler) buildWorkloadDetail(ctx context.Context, w *dbclient.Workload, result *types.GetWorkloadResponseItem) {
	result.Image = w.Image
	result.IsSupervised = w.IsSupervised
	result.MaxRetry = w.MaxRetry
	if w.TTLSecond > 0 {
		result.TTLSecondsAfterFinished = pointer.Int(w.TTLSecond)
	}
	if str := dbutils.ParseNullString(w.Conditions); str != "" {
		json.Unmarshal([]byte(str), &result.Conditions)
	}
	if str := dbutils.ParseNullString(w.Pods); str != "" {
		json.Unmarshal([]byte(str), &result.Pods)
		for i, p := range result.Pods {
			result.Pods[i].SSHAddr = h.buildSSHAddress(ctx, &p.WorkloadPod, result.UserName, result.Workspace)
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
	if result.GroupVersionKind.Kind != common.AuthoringKind && w.EntryPoint != "" {
		if stringutil.IsBase64(w.EntryPoint) {
			result.EntryPoint = stringutil.Base64Decode(w.EntryPoint)
		}
	}
}

func (h *Handler) cvtAdminWorkloadToResponse(ctx context.Context, w *v1.Workload, isNeedDetail bool) types.GetWorkloadResponseItem {
	result := types.GetWorkloadResponseItem{
		WorkloadId:     w.Name,
		Cluster:        v1.GetClusterId(w),
		Phase:          string(w.Status.Phase),
		CreationTime:   timeutil.FormatRFC3339(&w.CreationTimestamp.Time),
		SchedulerOrder: w.Status.SchedulerOrder,
		DispatchCount:  v1.GetWorkloadDispatchCnt(w),
		CreateWorkloadRequest: types.CreateWorkloadRequest{
			DisplayName: v1.GetDisplayName(w),
			Description: v1.GetDescription(w),
			UserName:    v1.GetUserName(w),
			WorkloadSpec: v1.WorkloadSpec{
				Priority:      w.Spec.Priority,
				Workspace:     w.Spec.Workspace,
				Timeout:       w.Spec.Timeout,
				IsTolerateAll: w.Spec.IsTolerateAll,
			},
		},
	}
	if !w.Status.StartTime.IsZero() {
		result.StartTime = timeutil.FormatRFC3339(&w.Status.StartTime.Time)
	}
	if !w.Status.EndTime.IsZero() {
		result.EndTime = timeutil.FormatRFC3339(&w.Status.EndTime.Time)
	}
	if result.Phase == string(v1.WorkloadPending) {
		result.Message = w.Status.Message
	}
	if isNeedDetail {
		result.WorkloadSpec = w.Spec
		result.EntryPoint = stringutil.Base64Decode(result.EntryPoint)
		result.Conditions = w.Status.Conditions
		result.Pods = make([]types.WorkloadPodWrapper, len(w.Status.Pods))
		for i, p := range w.Status.Pods {
			result.Pods[i].WorkloadPod = w.Status.Pods[i]
			result.Pods[i].SSHAddr = h.buildSSHAddress(ctx, &p, result.UserName, result.Workspace)
		}
		result.Nodes = w.Status.Nodes
		if len(w.Spec.CustomerLabels) > 0 {
			result.CustomerLabels = make(map[string]string)
			for key, val := range w.Spec.CustomerLabels {
				if strings.HasPrefix(key, common.CustomerLabelPrefix) {
					key = key[len(common.CustomerLabelPrefix):]
				}
				result.CustomerLabels[key] = val
			}
		}
	}
	if commonworkload.IsAuthoring(w) {
		result.EntryPoint = ""
	}
	return result
}

func (h *Handler) buildSSHAddress(ctx context.Context, pod *v1.WorkloadPod, userName, workspace string) string {
	if !commonconfig.IsSSHEnable() || pod.Phase != corev1.PodRunning {
		return ""
	}
	if userName == "" {
		userName = "none"
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
		return fmt.Sprintf("ssh %s.%s.%s@%s", userName, pod.PodId, workspace, gatewayIp)
	}

	localIp, _ := netutil.GetLocalIp()
	if localIp == "" {
		return ""
	}
	return fmt.Sprintf("ssh -p %d %s.%s.%s@%s",
		commonconfig.GetSSHServerPort(), userName, pod.PodId, workspace, localIp)
}

func buildWorkloadLabelSelector(query *types.GetWorkloadRequest) labels.Selector {
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
	}
	if query.Kind != "" {
		values := strings.Split(query.Kind, ",")
		req, _ := labels.NewRequirement(v1.WorkloadKindLabel, selection.In, values)
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector
}
