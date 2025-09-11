/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func (h *Handler) CreateCluster(c *gin.Context) {
	handle(c, h.createCluster)
}

func (h *Handler) ListCluster(c *gin.Context) {
	handle(c, h.listCluster)
}

func (h *Handler) GetCluster(c *gin.Context) {
	handle(c, h.getCluster)
}

func (h *Handler) DeleteCluster(c *gin.Context) {
	handle(c, h.deleteCluster)
}

func (h *Handler) PatchCluster(c *gin.Context) {
	handle(c, h.patchCluster)
}

func (h *Handler) ProcessClusterNodes(c *gin.Context) {
	handle(c, h.processClusterNodes)
}

func (h *Handler) GetClusterPodLog(c *gin.Context) {
	handle(c, h.getClusterPodLog)
}

func (h *Handler) createCluster(c *gin.Context) (interface{}, error) {
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: v1.ClusterKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.CreateClusterRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	cluster, err := h.generateCluster(c, req, body)
	if err != nil {
		klog.ErrorS(err, "failed to generate cluster")
		return nil, err
	}

	if err = h.Create(c.Request.Context(), cluster); err != nil {
		klog.ErrorS(err, "failed to create cluster")
		return nil, err
	}
	klog.Infof("created cluster %s", cluster.Name)
	return &types.CreateClusterResponse{
		ClusterId: cluster.Name,
	}, nil
}

func (h *Handler) generateCluster(c *gin.Context, req *types.CreateClusterRequest, body []byte) (*v1.Cluster, error) {
	ctx := c.Request.Context()
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
				v1.UserIdLabel:      c.GetString(common.UserId),
			},
		},
	}
	if err := json.Unmarshal(body, &cluster.Spec.ControlPlane); err != nil {
		return nil, err
	}
	for key, val := range req.Labels {
		if key == "" {
			continue
		}
		v1.SetLabel(cluster, key, val)
	}
	if req.Description != "" {
		v1.SetAnnotation(cluster, v1.DescriptionAnnotation, req.Description)
	}
	if req.IsProtected {
		v1.SetLabel(cluster, v1.ProtectLabel, "")
	}

	if cluster.Spec.ControlPlane.ImageSecret == nil {
		imageSecret, err := h.getSecret(ctx, common.PrimusImageSecret)
		if err != nil {
			return nil, err
		}
		cluster.Spec.ControlPlane.ImageSecret = commonutils.GenObjectReference(imageSecret.TypeMeta, imageSecret.ObjectMeta)
	}

	if cluster.Spec.ControlPlane.SSHSecret == nil && req.SSHSecretName != "" {
		sshSecret, err := h.getSecret(ctx, req.SSHSecretName)
		if err != nil {
			return nil, err
		}
		cluster.Spec.ControlPlane.SSHSecret = commonutils.GenObjectReference(sshSecret.TypeMeta, sshSecret.ObjectMeta)
	}
	return cluster, nil
}

func (h *Handler) listCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	clusterList := &v1.ClusterList{}
	if err := h.List(ctx, clusterList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	result := types.ListClusterResponse{}
	if len(clusterList.Items) > 0 {
		sort.Slice(clusterList.Items, func(i, j int) bool {
			return clusterList.Items[i].Name < clusterList.Items[j].Name
		})
	}
	for _, item := range clusterList.Items {
		result.Items = append(result.Items, cvtToClusterResponseItem(&item))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) getCluster(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	return cvtToGetClusterResponse(c.Request.Context(), h.Client, cluster), nil
}

func (h *Handler) deleteCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	cluster, err := h.getAdminCluster(ctx, c.GetString(types.Name))
	if err != nil {
		klog.ErrorS(err, "failed to get admin cluster")
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:  c.Request.Context(),
		Resource: cluster,
		Verb:     v1.DeleteVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	if v1.IsProtected(cluster) {
		klog.Errorf("failed to delete cluster %s, because the cluster is protected", cluster.Name)
		return nil, commonerrors.NewForbidden("the cluster is protected, it can not be deleted")
	}
	workloads, err := h.getRunningWorkloads(ctx, cluster.Name, nil)
	if err != nil {
		return nil, err
	}
	if len(workloads) > 0 {
		klog.Errorf("failed to delete cluster %s, due to running workloads", cluster.Name)
		return nil, commonerrors.NewForbidden("some workloads are still in progress. Please terminate them first.")
	}
	if err = h.Delete(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to delete cluster")
		return nil, err
	}
	klog.Infof("deleted cluster %s", cluster.Name)
	return nil, nil
}

func (h *Handler) patchCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	cluster, err := h.getAdminCluster(ctx, c.GetString(types.Name))
	if err != nil {
		klog.ErrorS(err, "failed to get admin cluster")
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:  c.Request.Context(),
		Resource: cluster,
		Verb:     v1.UpdateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.PatchClusterRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	isChanged := false
	if req.IsProtected != nil && *req.IsProtected != v1.IsProtected(cluster) {
		if *req.IsProtected {
			v1.SetLabel(cluster, v1.ProtectLabel, "")
		} else {
			v1.RemoveLabel(cluster, v1.ProtectLabel)
		}
		isChanged = true
	}
	if !isChanged {
		return nil, nil
	}
	return nil, h.Update(ctx, cluster)
}

func (h *Handler) processClusterNodes(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:  c.Request.Context(),
		Resource: cluster,
		Verb:     v1.UpdateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	if !cluster.IsReady() {
		return nil, commonerrors.NewInternalError("the cluster is not ready")
	}
	req, err := parseProcessNodesRequest(c)
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	if req.Action == v1.NodeActionRemove {
		if err = h.removeNodesFromWorkspace(c, req.NodeIds); err != nil {
			return nil, err
		}
	}

	response := types.ProcessNodesResponse{
		TotalCount: len(req.NodeIds),
	}
	message := ""
	for _, nodeId := range req.NodeIds {
		err = h.processClusterNode(ctx, cluster, nodeId, req.Action)
		if err != nil {
			klog.ErrorS(err, "failed to process node")
			message = err.Error()
		} else {
			response.SuccessCount++
		}
	}
	if response.SuccessCount == 0 {
		return nil, fmt.Errorf("no nodes processed successfully, message: %s", message)
	}
	return &response, nil
}

func (h *Handler) processClusterNode(ctx context.Context, cluster *v1.Cluster, nodeId, action string) error {
	specCluster := ""
	if action == v1.NodeActionAdd {
		specCluster = cluster.Name
	}

	nodeId = strings.TrimSpace(nodeId)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		adminNode, err := h.getAdminNode(ctx, nodeId)
		if err != nil {
			return err
		}
		if action == v1.NodeActionRemove {
			if v1.GetClusterId(adminNode) != cluster.Name {
				return fmt.Errorf("The node does not belong to the specified cluster")
			}
		} else {
			if adminNode.GetSpecCluster() != "" && adminNode.GetSpecCluster() != specCluster {
				return fmt.Errorf("The node belongs to another cluster")
			}
		}
		if adminNode.GetSpecCluster() == specCluster {
			return nil
		}
		if v1.IsControlPlane(adminNode) {
			return fmt.Errorf("the control plane node can not be changed")
		}
		v1.RemoveAnnotation(adminNode, v1.RetryCountAnnotation)
		adminNode.Spec.Cluster = pointer.String(specCluster)
		if err = h.Update(ctx, adminNode); err != nil {
			return err
		}
		return nil
	})
	return err
}

func (h *Handler) removeNodesFromWorkspace(c *gin.Context, allNodeIds []string) error {
	nodeIdMap := make(map[string]*[]string)
	for _, nodeId := range allNodeIds {
		node, err := h.getAdminNode(c.Request.Context(), nodeId)
		if err != nil {
			return err
		}
		workspaceId := v1.GetWorkspaceId(node)
		if workspaceId != "" {
			ids, ok := nodeIdMap[workspaceId]
			if !ok {
				ids2 := make([]string, 0, len(allNodeIds))
				ids2 = append(ids2, nodeId)
				nodeIdMap[workspaceId] = &ids2
			} else {
				*ids = append(*ids, nodeId)
			}
		}
	}

	for workspaceId, nodeIds := range nodeIdMap {
		if err := h.updateWorkspaceNodesAction(c, workspaceId, v1.NodeActionRemove, *nodeIds); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) getClusterPodLog(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:  c.Request.Context(),
		Resource: cluster,
		// The pod-log is generated when the cluster is creating.
		Verb:   v1.CreateVerb,
		UserId: c.GetString(common.UserId),
	}); err != nil {
		if commonerrors.IsForbidden(err) {
			return nil, commonerrors.NewForbidden("The user is not allowed to get cluster's log")
		}
		return nil, err
	}

	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterManageClusterLabel: cluster.Name})
	podName, err := h.getLatestPodName(c, labelSelector)
	if err != nil {
		return nil, commonerrors.NewNotImplemented("Logging service is only available when creating cluster")
	}
	podLogs, err := h.getPodLog(c, h.clientSet, common.PrimusSafeNamespace, podName, "")
	if err != nil {
		return nil, err
	}
	return &types.GetNodePodLogResponse{
		ClusterId: cluster.Name,
		PodId:     podName,
		Logs:      strings.Split(string(podLogs), "\n"),
	}, nil
}

func (h *Handler) getLatestPodName(c *gin.Context, labelSelector labels.Selector) (string, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	}
	podList, err := h.clientSet.CoreV1().Pods(common.PrimusSafeNamespace).List(c.Request.Context(), listOptions)
	if err != nil {
		return "", err
	}
	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no running pod found")
	}
	sort.Slice(podList.Items, func(i, j int) bool {
		return podList.Items[i].CreationTimestamp.Time.After(podList.Items[j].CreationTimestamp.Time)
	})
	return podList.Items[0].Name, nil
}

func (h *Handler) getPodLog(c *gin.Context, clientSet kubernetes.Interface,
	namespace, podName, mainContainerName string) ([]byte, error) {
	query, err := parseGetPodLogQuery(c, mainContainerName)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	opt := &corev1.PodLogOptions{
		Container: query.Container,
		TailLines: &query.TailLines,
	}
	if query.SinceSeconds > 0 {
		opt.SinceSeconds = &query.SinceSeconds
	}
	podLogs, err := clientSet.CoreV1().Pods(namespace).GetLogs(podName, opt).DoRaw(c.Request.Context())
	if err != nil {
		klog.ErrorS(err, "failed to get log of pod", "namespace", namespace, "podName", podName)
		return nil, err
	}
	return podLogs, nil
}

func (h *Handler) getAdminCluster(ctx context.Context, name string) (*v1.Cluster, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the clusterId is empty")
	}
	cluster := &v1.Cluster{}
	err := h.Get(ctx, client.ObjectKey{Name: name}, cluster)
	if err != nil {
		klog.ErrorS(err, "failed to get admin cluster")
		return nil, err
	}
	return cluster.DeepCopy(), nil
}

func parseProcessNodesRequest(c *gin.Context) (*types.ProcessNodesRequest, error) {
	req := &types.ProcessNodesRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	if len(req.NodeIds) == 0 {
		return nil, commonerrors.NewBadRequest("no nodeIds provided")
	}
	if len(req.Action) == 0 {
		return nil, commonerrors.NewBadRequest("no action provided")
	}
	return req, nil
}

func cvtToClusterResponseItem(cluster *v1.Cluster) types.ClusterResponseItem {
	result := types.ClusterResponseItem{
		ClusterId:   cluster.Name,
		UserId:      v1.GetUserId(cluster),
		Phase:       string(cluster.Status.ControlPlaneStatus.Phase),
		IsProtected: v1.IsProtected(cluster),
	}
	if !cluster.GetDeletionTimestamp().IsZero() {
		result.Phase = string(v1.DeletingPhase)
	}
	return result
}

func cvtToGetClusterResponse(ctx context.Context, client client.Client, cluster *v1.Cluster) types.GetClusterResponse {
	result := types.GetClusterResponse{
		ClusterResponseItem: types.ClusterResponseItem{
			ClusterId:   cluster.Name,
			UserId:      v1.GetUserId(cluster),
			Phase:       string(cluster.Status.ControlPlaneStatus.Phase),
			IsProtected: v1.IsProtected(cluster),
		},
	}
	if !cluster.GetDeletionTimestamp().IsZero() {
		result.Phase = string(v1.DeletingPhase)
	}
	result.Endpoint, _ = commoncluster.GetEndpoint(ctx, client, cluster)
	result.Storages = cvtBindingStorageView(cluster.Status.StorageStatus)
	return result
}
