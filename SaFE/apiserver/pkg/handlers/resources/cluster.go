/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// CreateCluster handles the creation of a new cluster resource.
// It authorizes the request, parses the request body, generates a cluster object,
// and creates it in the k8s cluster. Returns the created cluster ID on success.
func (h *Handler) CreateCluster(c *gin.Context) {
	handle(c, h.createCluster)
}

// ListCluster handles listing all cluster resources.
// Retrieves all clusters and returns them in a sorted list with basic information.
func (h *Handler) ListCluster(c *gin.Context) {
	handle(c, h.listCluster)
}

// GetCluster retrieves detailed information about a specific cluster.
// Returns comprehensive cluster details including configuration and status.
func (h *Handler) GetCluster(c *gin.Context) {
	handle(c, h.getCluster)
}

// DeleteCluster handles the deletion of a cluster resource.
// It performs authorization checks, validates that the cluster is not protected,
// ensures no workloads are running on the cluster, and then deletes the cluster.
// Returns nil on successful deletion or an error if the check fails
func (h *Handler) DeleteCluster(c *gin.Context) {
	handle(c, h.deleteCluster)
}

// PatchCluster handles partial updates to a cluster resource.
// Authorizes the request, parses update parameters, and applies changes to the specified cluster.
func (h *Handler) PatchCluster(c *gin.Context) {
	handle(c, h.patchCluster)
}

// ProcessClusterNodes handles the addition or removal of nodes from a cluster.
// It performs authorization checks, validates cluster readiness, and processes
// each node according to the requested action (add/remove).
// For node removal operations, it first removes nodes from their associated workspaces.
// The function returns a ProcessNodesResponse with success/failure counts.
func (h *Handler) ProcessClusterNodes(c *gin.Context) {
	handle(c, h.processClusterNodes)
}

// GetClusterPodLog retrieves the logs from the most recent pod associated with a cluster.
// It performs authorization checks, finds the latest pod for the cluster, fetches its logs,
// and returns them in a structured response format.
// Returns a GetNodePodLogResponse or an error if any step in the process fails.
func (h *Handler) GetClusterPodLog(c *gin.Context) {
	handle(c, h.getClusterPodLog)
}

// createCluster implements the cluster creation logic.
// Authorizes the request, parses the creation request, generates a cluster object,
// and persists it in the k8s cluster.
func (h *Handler) createCluster(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: v1.ClusterKind,
		Verb:         v1.CreateVerb,
		User:         requestUser,
	}); err != nil {
		return nil, err
	}

	req := &view.CreateClusterRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	cluster, err := h.generateCluster(c.Request.Context(), requestUser, req, body)
	if err != nil {
		klog.ErrorS(err, "failed to generate cluster")
		return nil, err
	}

	if err = h.Create(c.Request.Context(), cluster); err != nil {
		klog.ErrorS(err, "failed to create cluster")
		return nil, err
	}
	klog.Infof("created cluster %s", cluster.Name)
	return &view.CreateClusterResponse{
		ClusterId: cluster.Name,
	}, nil
}

// generateCluster convert the CreateClusterRequest passed from the API into a v1.Cluster object,
// then pass it to the createCluster function for invocation.
func (h *Handler) generateCluster(ctx context.Context,
	requestUser *v1.User, req *view.CreateClusterRequest, body []byte) (*v1.Cluster, error) {
	cluster := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
				v1.UserIdLabel:      requestUser.Name,
			},
		},
	}
	if err := json.Unmarshal(body, &cluster.Spec.ControlPlane); err != nil {
		return nil, err
	}
	for key, val := range req.Labels {
		if key == "" || strings.HasPrefix(key, v1.PrimusSafePrefix) {
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

	if cluster.Spec.ControlPlane.ImageSecret == nil && commonconfig.GetImageSecret() != "" {
		imageSecret, err := h.getAndAuthorizeSecret(ctx, commonconfig.GetImageSecret(), "", requestUser, v1.GetVerb)
		if err != nil {
			return nil, err
		}
		cluster.Spec.ControlPlane.ImageSecret = commonutils.GenObjectReference(imageSecret.TypeMeta, imageSecret.ObjectMeta)
	}

	if cluster.Spec.ControlPlane.SSHSecret == nil && req.SSHSecretId != "" {
		sshSecret, err := h.getAndAuthorizeSecret(ctx, req.SSHSecretId, "", requestUser, v1.GetVerb)
		if err != nil {
			return nil, err
		}
		cluster.Spec.ControlPlane.SSHSecret = commonutils.GenObjectReference(sshSecret.TypeMeta, sshSecret.ObjectMeta)
	}
	return cluster, nil
}

// listCluster implements the cluster listing logic.
// Retrieves all clusters, sorts them by name, and converts them to response items.
func (h *Handler) listCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	clusterList := &v1.ClusterList{}
	if err := h.List(ctx, clusterList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	result := view.ListClusterResponse{}
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

// getCluster implements the logic for retrieving a single cluster's detailed information.
// Gets the cluster by ID and converts it to a detailed response format.
func (h *Handler) getCluster(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	return cvtToGetClusterResponse(c.Request.Context(), h.Client, cluster), nil
}

// deleteCluster handles the deletion of a cluster resource.
// It performs authorization checks, validates that the cluster is not protected,
// ensures no workloads are running on the cluster, and then deletes the cluster.
// Returns nil on successful deletion or an error if the check fails
func (h *Handler) deleteCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	cluster, err := h.getAdminCluster(ctx, c.GetString(common.Name))
	if err != nil {
		klog.ErrorS(err, "failed to get admin cluster")
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
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

// patchCluster implements partial update logic for a cluster.
// Parses the patch request and applies specified changes to the cluster.
func (h *Handler) patchCluster(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	cluster, err := h.getAdminCluster(ctx, c.GetString(common.Name))
	if err != nil {
		klog.ErrorS(err, "failed to get admin cluster")
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: cluster,
		Verb:     v1.UpdateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &view.PatchClusterRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	isChanged, err := applyClusterPatch(cluster, req)
	if err != nil {
		return nil, err
	}
	if !isChanged {
		return nil, nil
	}
	return nil, h.Update(ctx, cluster)
}

// applyClusterPatch applies updates to a cluster based on the patch request.
// Handles changes to cluster protection status and image secret references.
func applyClusterPatch(cluster *v1.Cluster, req *view.PatchClusterRequest) (bool, error) {
	isChanged := false
	if req.IsProtected != nil && *req.IsProtected != v1.IsProtected(cluster) {
		if *req.IsProtected {
			v1.SetLabel(cluster, v1.ProtectLabel, "")
		} else {
			v1.RemoveLabel(cluster, v1.ProtectLabel)
		}
		isChanged = true
	}
	if req.Labels != nil {
		for key, _ := range cluster.Labels {
			if strings.HasPrefix(key, v1.PrimusSafePrefix) {
				continue
			}
			_, ok := (*req.Labels)[key]
			if !ok {
				if v1.RemoveLabel(cluster, key) {
					isChanged = true
				}
			}
		}
		for key, val := range *req.Labels {
			if key == "" || strings.HasPrefix(key, v1.PrimusSafePrefix) {
				continue
			}
			if v1.SetLabel(cluster, key, val) {
				isChanged = true
			}
		}
	}
	return isChanged, nil
}

// processClusterNodes handles the addition or removal of nodes from a cluster.
// It performs authorization checks, validates cluster readiness, and processes
// each node according to the requested action (add/remove).
// For node removal operations, it first removes nodes from their associated workspaces.
// The function returns a ProcessNodesResponse with success/failure counts.
func (h *Handler) processClusterNodes(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
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
		if err = h.removeNodesFromWorkspace(c, req.NodeIds, req.Force); err != nil {
			return nil, err
		}
	}

	response := view.ProcessNodesResponse{
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

// processClusterNode processes a single node for cluster operations (add/remove).
// It updates the node's cluster assignment based on the specified action.
// For NodeActionAdd, it assigns the node to the cluster.
// For NodeActionRemove, it removes the node from the cluster.
// The function handles conflict retries and validates node operations.
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
		if adminNode.GetSpecCluster() == specCluster {
			return nil
		}
		if action == v1.NodeActionRemove {
			if v1.GetClusterId(adminNode) != cluster.Name {
				return fmt.Errorf("the node does not belong to the specified cluster")
			}
		} else {
			if adminNode.GetSpecCluster() != "" && adminNode.GetSpecCluster() != specCluster {
				return fmt.Errorf("the node belongs to another cluster")
			}
		}
		if v1.IsControlPlane(adminNode) {
			return fmt.Errorf("the control plane node can not be changed")
		}
		adminNode.Spec.Cluster = pointer.String(specCluster)
		if err = h.Update(ctx, adminNode); err != nil {
			return err
		}
		return nil
	})
	return err
}

// removeNodesFromWorkspace removes nodes from their associated workspaces.
// It groups nodes by workspace ID and updates each workspace to remove the specified nodes.
func (h *Handler) removeNodesFromWorkspace(c *gin.Context, allNodeIds []string, force bool) error {
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
		if err := h.updateWorkspaceNodesAction(c, workspaceId, v1.NodeActionRemove, *nodeIds, force); err != nil {
			return err
		}
	}
	return nil
}

// getClusterPodLog retrieves the logs from the most recent pod associated with a cluster.
// It performs authorization checks, finds the latest pod for the cluster, fetches its logs,
// and returns them in a structured response format.
// Returns a GetNodePodLogResponse or an error if any step in the process fails.
func (h *Handler) getClusterPodLog(c *gin.Context) (interface{}, error) {
	cluster, err := h.getAdminCluster(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: cluster,
		// The pod log is generated when the cluster is being created.
		Verb:   v1.CreateVerb,
		UserId: c.GetString(common.UserId),
	}); err != nil {
		if commonerrors.IsForbidden(err) {
			return nil, commonerrors.NewForbidden("the user is not allowed to get cluster's log")
		}
		return nil, err
	}

	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterManageClusterLabel: cluster.Name})
	podName, err := h.getLatestPodName(c, labelSelector)
	if err != nil {
		return nil, commonerrors.NewNotImplemented("logging service is only available when creating cluster")
	}
	podLogs, err := h.getPodLog(c, h.clientSet, common.PrimusSafeNamespace, podName, "")
	if err != nil {
		return nil, err
	}
	return &view.GetNodePodLogResponse{
		ClusterId: cluster.Name,
		PodId:     podName,
		Logs:      strings.Split(string(podLogs), "\n"),
	}, nil
}

// getLatestPodName retrieves the name of the most recently created pod that matches the given label selector.
// It lists all pods in the PrimusSafe namespace with the specified labels and returns the name of the newest one.
// Returns an error if no pods are found or if there's an issue listing the pods.
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

// getPodLog retrieves the logs from a specific pod in the given namespace.
// It parses the log query parameters, constructs the log options, and fetches the logs
// from the Kubernetes API server using the provided client.
// Returns the raw log bytes or an error if the operation fails.
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

// getAdminCluster retrieves a cluster resource by ID from the k8s cluster.
// Returns an error if the cluster doesn't exist or the ID is empty.
func (h *Handler) getAdminCluster(ctx context.Context, clusterId string) (*v1.Cluster, error) {
	if clusterId == "" {
		return nil, commonerrors.NewBadRequest("the clusterId is empty")
	}
	cluster := &v1.Cluster{}
	err := h.Get(ctx, client.ObjectKey{Name: clusterId}, cluster)
	if err != nil {
		klog.ErrorS(err, "failed to get admin cluster")
		return nil, err
	}
	return cluster.DeepCopy(), nil
}

// parseProcessNodesRequest parses and validates the request for processing cluster nodes.
// Ensures that node IDs and action are provided in the request.
func parseProcessNodesRequest(c *gin.Context) (*view.ProcessNodesRequest, error) {
	req := &view.ProcessNodesRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
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

// cvtToClusterResponseItem converts a cluster object to a response item format.
// Includes basic cluster information like ID, user, phase, protection status, and creation time.
func cvtToClusterResponseItem(cluster *v1.Cluster) view.ClusterResponseItem {
	result := view.ClusterResponseItem{
		ClusterId:    cluster.Name,
		UserId:       v1.GetUserId(cluster),
		Phase:        string(cluster.Status.ControlPlaneStatus.Phase),
		IsProtected:  v1.IsProtected(cluster),
		CreationTime: timeutil.FormatRFC3339(cluster.CreationTimestamp.Time),
	}
	if !cluster.GetDeletionTimestamp().IsZero() {
		result.Phase = string(v1.DeletingPhase)
	}
	return result
}

// cvtToGetClusterResponse converts a cluster object to a detailed response format.
// Includes all cluster details, configuration parameters, and status information.
func cvtToGetClusterResponse(ctx context.Context, client client.Client, cluster *v1.Cluster) view.GetClusterResponse {
	result := view.GetClusterResponse{
		ClusterResponseItem: cvtToClusterResponseItem(cluster),
		Description:         v1.GetDescription(cluster),
		Nodes:               cluster.Spec.ControlPlane.Nodes,
		KubeSprayImage:      cluster.Spec.ControlPlane.KubeSprayImage,
		KubePodsSubnet:      cluster.Spec.ControlPlane.KubePodsSubnet,
		KubeServiceAddress:  cluster.Spec.ControlPlane.KubeServiceAddress,
		KubeNetworkPlugin:   cluster.Spec.ControlPlane.KubeNetworkPlugin,
		KubeVersion:         cluster.Spec.ControlPlane.KubeVersion,
		KubeApiServerArgs:   cluster.Spec.ControlPlane.KubeApiServerArgs,
	}
	if cluster.Spec.ControlPlane.ImageSecret != nil {
		result.ImageSecretId = cluster.Spec.ControlPlane.ImageSecret.Name
	}
	if cluster.Spec.ControlPlane.SSHSecret != nil {
		result.SSHSecretId = cluster.Spec.ControlPlane.SSHSecret.Name
	}
	for key, val := range cluster.Labels {
		if strings.HasPrefix(key, v1.PrimusSafePrefix) {
			continue
		}
		if len(result.Labels) == 0 {
			result.Labels = make(map[string]string)
		}
		result.Labels[key] = val
	}
	result.Endpoint, _ = commoncluster.GetEndpoint(ctx, client, cluster)
	return result
}
