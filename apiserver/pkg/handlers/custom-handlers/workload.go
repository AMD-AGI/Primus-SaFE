/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
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
	if err = h.Create(c.Request.Context(), workload); err != nil {
		klog.ErrorS(err, "failed to create workload")
		return nil, err
	}
	if err = h.patchPhase(c.Request.Context(), workload, v1.WorkloadPending, nil); err != nil {
		return nil, err
	}

	klog.Infof("create workload, name: %s, user: %s, priority: %d, timeout: %d",
		workload.Name, req.UserName, workload.Spec.Priority, workload.Spec.Timeout)
	return &types.CreateWorkloadResponse{WorkloadId: workload.Name}, nil
}

func (h *Handler) listWorkload(c *gin.Context) (interface{}, error) {
	query, err := parseListWorkloadQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	labelSelector := buildWorkloadLabelSelector(query)
	workloadList := &v1.WorkloadList{}
	if err = h.List(c.Request.Context(), workloadList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	if len(workloadList.Items) > 0 {
		sort.Sort(types.WorkloadSlice(workloadList.Items))
	}

	result := &types.GetWorkloadResponse{}
	for _, w := range workloadList.Items {
		if query.Phase != "" && !stringutil.StrCaseEqual(query.Phase, string(w.Status.Phase)) {
			continue
		}
		result.Items = append(result.Items, cvtToWorkloadResponse(&w, false))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) getWorkload(c *gin.Context) (interface{}, error) {
	workload, err := h.getAdminWorkload(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	return cvtToWorkloadResponse(workload, true), nil
}

func (h *Handler) deleteWorkload(c *gin.Context) (interface{}, error) {
	workload, err := h.getAdminWorkload(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	cond := &metav1.Condition{
		Type:    string(v1.AdminStopped),
		Status:  metav1.ConditionTrue,
		Message: "the workload is deleted",
	}
	if err = h.patchPhase(c.Request.Context(), workload, v1.WorkloadStopped, cond); err != nil {
		return nil, err
	}
	if err = h.Delete(c.Request.Context(), workload); err != nil {
		return nil, err
	}
	klog.Infof("delete workload, workload.id: %s", workload.Name)
	return nil, nil
}

func (h *Handler) patchWorkload(c *gin.Context) (interface{}, error) {
	workload, err := h.getAdminWorkload(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	req := &types.PatchWorkloadRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	patch := client.MergeFrom(workload)
	updateWorkload(workload, req)
	if err = h.Patch(c.Request.Context(), workload, patch); err != nil {
		klog.ErrorS(err, "failed to patch workload")
		return nil, err
	}
	klog.Infof("patch workload, name: %s, request: %s", workload.Name, string(jsonutils.MarshalSilently(*req)))
	return nil, nil
}

func (h *Handler) getWorkloadPodLog(c *gin.Context) (interface{}, error) {
	workload, err := h.getAdminWorkload(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}

	k8sClients, err := h.getK8sClientFactory(v1.GetClusterId(workload))
	if err != nil {
		return nil, err
	}
	podName := strings.TrimSpace(c.Param(types.PodId))
	podLogs, err := h.getPodLog(c, k8sClients.ClientSet(),
		workload.Spec.Workspace, podName, v1.GetWorkloadMainContainer(workload))
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

func (h *Handler) getAdminWorkload(ctx context.Context, name string) (*v1.Workload, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}
	workload := &v1.Workload{}
	if err := h.Get(ctx, client.ObjectKey{Name: name}, workload); err != nil {
		klog.ErrorS(err, "failed to get admin workload", "workload", name)
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
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
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
	if workload.Name == "" {
		workload.Name = commonutils.GenerateNameWithPrefix(req.DisplayName)
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
		req, _ := labels.NewRequirement(v1.WorkloadKindLabel, selection.Equals, []string{query.Kind})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector
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
	if req.ShareMemory != nil {
		adminWorkload.Spec.Resource.ShareMemory = *req.ShareMemory
	}
	if req.Image != nil && *req.Image != "" {
		adminWorkload.Spec.Image = *req.Image
	}
	if req.EntryPoint != nil && *req.EntryPoint != "" {
		adminWorkload.Spec.EntryPoint = *req.EntryPoint
	}
	if req.Description != nil {
		metav1.SetMetaDataAnnotation(&adminWorkload.ObjectMeta, v1.DescriptionAnnotation, *req.Description)
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

func cvtToWorkloadResponse(w *v1.Workload, isNeedDetail bool) types.GetWorkloadResponseItem {
	result := types.GetWorkloadResponseItem{
		WorkloadId:     w.Name,
		Cluster:        v1.GetClusterId(w),
		UserName:       v1.GetUserName(w),
		Phase:          string(w.Status.Phase),
		CreatedTime:    timeutil.FormatRFC3339(&w.CreationTimestamp.Time),
		SchedulerOrder: w.Status.SchedulerOrder,
		DispatchCount:  v1.GetWorkloadDispatchCnt(w),
		CreateWorkloadRequest: types.CreateWorkloadRequest{
			DisplayName: v1.GetDisplayName(w),
			Description: v1.GetDescription(w),
			UserName:    v1.GetUserName(w),
			WorkloadSpec: v1.WorkloadSpec{
				Priority:  w.Spec.Priority,
				Workspace: w.Spec.Workspace,
				Timeout:   w.Spec.Timeout,
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
		buildWorkloadDetail(w, &result)
	}
	return result
}

func buildWorkloadDetail(w *v1.Workload, result *types.GetWorkloadResponseItem) {
	result.WorkloadSpec = w.Spec
	result.Conditions = string(jsonutils.MarshalSilently(w.Status.Conditions))
	result.Pods = string(jsonutils.MarshalSilently(w.Status.Pods))
	result.Nodes = string(jsonutils.MarshalSilently(w.Status.Nodes))
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
