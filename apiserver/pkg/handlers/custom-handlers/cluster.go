/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func (h *Handler) CreateCluster(c *gin.Context) {
	handle(c, h.createCluster)
}

func (h *Handler) AddClusterNodes(c *gin.Context) {
	handle(c, h.addClusterNodes)
}

func (h *Handler) RemoveClusterNodes(c *gin.Context) {
	handle(c, h.removeClusterNodes)
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

func (h *Handler) GetClusterPodLog(c *gin.Context) {
	handle(c, h.getClusterPodLog)
}

func (h *Handler) createCluster(c *gin.Context) (interface{}, error) {
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
		return nil, err
	}
	return &types.CreateClusterResponse{
		ClusterId: cluster.Name,
	}, nil
}

func (h *Handler) addClusterNodes(c *gin.Context) (interface{}, error) {
	adminCluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if !adminCluster.IsReady() {
		return nil, commonerrors.NewInternalError("the cluster is not ready")
	}

	req := &types.ClusterNodesRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	if len(req.NodeIds) == 0 {
		return nil, commonerrors.NewBadRequest("no nodeIds provided")
	}

	req.Action = types.ClusterNodeAdd
	return h.handleClusterNodes(c, req, adminCluster)
}

func (h *Handler) removeClusterNodes(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if !cluster.IsReady() {
		return nil, commonerrors.NewInternalError("the cluster is not ready")
	}

	req := &types.ClusterNodesRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	if len(req.NodeIds) == 0 {
		return nil, commonerrors.NewBadRequest("no nodeIds provided")
	}
	if err = h.removeNodesFromWorkspace(c.Request.Context(), req.NodeIds); err != nil {
		return nil, err
	}
	req.Action = types.ClusterNodeDel
	return h.handleClusterNodes(c, req, cluster)
}

func (h *Handler) removeNodesFromWorkspace(ctx context.Context, allNodeIds []string) error {
	nodeIdMap := make(map[string]*[]string)
	for _, nodeId := range allNodeIds {
		node, err := h.getAdminNode(ctx, nodeId)
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
		nodeAction := commonnodes.BuildAction(v1.NodeActionRemove, *nodeIds...)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			workspace := &v1.Workspace{}
			if err := h.Get(ctx, client.ObjectKey{Name: workspaceId}, workspace); err != nil {
				return client.IgnoreNotFound(err)
			}
			metav1.SetMetaDataAnnotation(&workspace.ObjectMeta, v1.WorkspaceNodesAction, nodeAction)
			if err := h.Update(ctx, workspace); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) handleClusterNodes(c *gin.Context,
	req *types.ClusterNodesRequest, cluster *v1.Cluster) (*types.HandleNodesResponse, error) {
	response := types.HandleNodesResponse{
		TotalCount: len(req.NodeIds),
	}
	ctx := c.Request.Context()
	specCluster := ""
	if req.Action == types.ClusterNodeAdd {
		specCluster = cluster.Name
	}

	message := ""
	for _, nodeId := range req.NodeIds {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			adminNode, err := h.getAdminNode(ctx, nodeId)
			if err != nil {
				return err
			}
			if req.Action == types.ClusterNodeDel {
				if v1.GetClusterId(adminNode) != cluster.Name {
					return nil
				}
			} else {
				if adminNode.GetSpecCluster() != "" && adminNode.GetSpecCluster() != specCluster {
					klog.Errorf("the admin node(%s) is managed by another cluster: %s, pls unmanged first",
						nodeId, adminNode.GetSpecCluster())
					return nil
				}
			}
			if adminNode.GetSpecCluster() == specCluster {
				response.SuccessCount++
				return nil
			}
			if v1.IsControlPlane(adminNode) {
				klog.Infof("the control plane node(%s) can not be changed", adminNode.Name)
				return nil
			}
			adminNode.Spec.Cluster = pointer.String(specCluster)
			if err := h.Update(ctx, adminNode); err != nil {
				return err
			}
			response.SuccessCount++
			return nil
		})
		if err != nil {
			klog.ErrorS(err, "failed to update node")
			message = err.Error()
		}
	}
	if response.SuccessCount == 0 {
		return nil, fmt.Errorf("no nodes processed successfully, message: %s", message)
	}
	return &response, nil
}

func (h *Handler) listCluster(c *gin.Context) (interface{}, error) {
	clusterList := &v1.ClusterList{}
	if err := h.List(c.Request.Context(), clusterList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	result := types.GetClusterResponse{}
	if len(clusterList.Items) > 0 {
		sort.Slice(clusterList.Items, func(i, j int) bool {
			return clusterList.Items[i].Name < clusterList.Items[j].Name
		})
	}
	for _, item := range clusterList.Items {
		result.Items = append(result.Items, cvtToGetClusterResponseItem(&item, false))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) getCluster(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	return cvtToGetClusterResponseItem(cluster, true), nil
}

func (h *Handler) deleteCluster(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		klog.ErrorS(err, "failed to get admin cluster")
		return nil, err
	}
	if v1.IsProtected(cluster) {
		return nil, commonerrors.NewForbidden("the cluster is protected, it can not be deleted")
	}
	workloads, err := h.getRunningWorkloads(c.Request.Context(), cluster.Name, nil)
	if err != nil {
		return nil, err
	}
	if len(workloads) > 0 {
		return nil, commonerrors.NewForbidden("some workloads are still in progress. Please terminate them first.")
	}
	return nil, h.Delete(c.Request.Context(), cluster)
}

func (h *Handler) patchCluster(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		klog.ErrorS(err, "failed to get admin cluster")
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
			metav1.SetMetaDataLabel(&cluster.ObjectMeta, v1.ProtectLabel, "")
		} else {
			delete(cluster.Labels, v1.ProtectLabel)
		}
		isChanged = true
	}
	if !isChanged {
		return nil, nil
	}
	return nil, h.Update(c.Request.Context(), cluster)
}

func (h *Handler) generateCluster(c *gin.Context, req *types.CreateClusterRequest, body []byte) (*v1.Cluster, error) {
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
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
		metav1.SetMetaDataLabel(&cluster.ObjectMeta, key, val)
	}
	if req.Description != "" {
		metav1.SetMetaDataAnnotation(&cluster.ObjectMeta, v1.DescriptionAnnotation, req.Description)
	}
	if req.IsProtected {
		metav1.SetMetaDataLabel(&cluster.ObjectMeta, v1.ProtectLabel, "")
	}

	if cluster.Spec.ControlPlane.KubeNetworkNodePrefix == nil {
		var networkPrefix uint32 = 24
		if req.MaxPodCount > 0 && req.MaxPodCount < 256 {
			networkPrefix = 32 - uint32(math.Log2(float64(req.MaxPodCount)))
		}
		cluster.Spec.ControlPlane.KubeNetworkNodePrefix = pointer.Uint32(networkPrefix)
	}

	if cluster.Spec.ControlPlane.ImageSecret == nil {
		imageSecret, err := h.getSecret(c.Request.Context(), common.PrimusImageSecret)
		if err != nil {
			return nil, err
		}
		cluster.Spec.ControlPlane.ImageSecret = commonutils.GenObjectReference(imageSecret.TypeMeta, imageSecret.ObjectMeta)
	}

	if cluster.Spec.ControlPlane.SSHSecret == nil && req.SSHSecretName != "" {
		sshSecret, err := h.getSecret(c.Request.Context(), req.SSHSecretName)
		if err != nil {
			return nil, err
		}
		cluster.Spec.ControlPlane.SSHSecret = commonutils.GenObjectReference(sshSecret.TypeMeta, sshSecret.ObjectMeta)
	}
	return cluster, nil
}

func (h *Handler) getClusterPodLog(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterManageClusterLabel: cluster.Name})
	podName, err := h.getLatestPodName(c, labelSelector)
	if err != nil {
		return nil, err
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
		return "", commonerrors.NewNotFoundWithMessage("no running pod found")
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

func cvtToGetClusterResponseItem(c *v1.Cluster, isNeedDetail bool) types.GetClusterResponseItem {
	result := types.GetClusterResponseItem{
		ClusterId:   c.Name,
		Phase:       string(c.Status.ControlPlaneStatus.Phase),
		IsProtected: v1.IsProtected(c),
	}
	if !c.GetDeletionTimestamp().IsZero() {
		result.Phase = string(v1.DeletingPhase)
	}
	if isNeedDetail {
		result.Storages = cvtBindingStorageView(c.Status.StorageStatus)
	}
	return result
}
