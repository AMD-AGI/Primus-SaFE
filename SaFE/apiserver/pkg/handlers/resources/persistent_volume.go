/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// ListPersistentVolume handles listing Persistent Volumes with filtering capabilities.
func (h *Handler) ListPersistentVolume(c *gin.Context) {
	handle(c, h.listPersistentVolume)
}

// listPersistentVolume implements the Persistent Volumes listing logic.
// Parses query parameters, builds label selectors, retrieves matching Persistent Volumes,
func (h *Handler) listPersistentVolume(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	query, err := parseListPersistentVolumeQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	ctx := c.Request.Context()
	workspace, err := h.getAdminWorkspace(ctx, query.WorkspaceID)
	if err != nil {
		return nil, err
	}

	// The PV's access permissions are consistent with those of the workspace.
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workspace,
		Verb:       v1.ListVerb,
		Workspaces: []string{workspace.Name},
		User:       requestUser,
	}); err != nil {
		return nil, err
	}

	labelSelector, err := buildListPersistentVolumeSelector(query)
	if err != nil {
		return nil, err
	}
	k8sClients, err := commonutils.GetK8sClientFactory(h.clientManager, workspace.Spec.Cluster)
	if err != nil {
		return nil, err
	}
	pvList, err := k8sClients.ClientSet().CoreV1().PersistentVolumes().List(ctx,
		metav1.ListOptions{LabelSelector: labelSelector.String()})
	if err != nil {
		return nil, err
	}
	result := &view.ListPersistentVolumeResponse{}
	for _, item := range pvList.Items {
		result.Items = append(result.Items, cvtToPersistentVolumeItem(item))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// parseListPersistentVolumeQuery parses and validates the query parameters for PersistentVolume listing.
func parseListPersistentVolumeQuery(c *gin.Context) (*view.ListPersistentVolumeRequest, error) {
	query := &view.ListPersistentVolumeRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

// buildListPersistentVolumeSelector constructs a label selector based on PersistentVolume list query parameters.
func buildListPersistentVolumeSelector(query *view.ListPersistentVolumeRequest) (labels.Selector, error) {
	var labelSelector = labels.NewSelector()
	if query.WorkspaceID != "" {
		req, _ := labels.NewRequirement(v1.WorkspaceIdLabel, selection.Equals, []string{query.WorkspaceID})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector, nil
}

// cvtToPersistentVolumeItem converts a PersistentVolume record to a response item format.
func cvtToPersistentVolumeItem(item corev1.PersistentVolume) view.PersistentVolumeItem {
	result := view.PersistentVolumeItem{
		Capacity:                      item.Spec.Capacity,
		AccessModes:                   item.Spec.AccessModes,
		ClaimRef:                      item.Spec.ClaimRef,
		VolumeMode:                    item.Spec.VolumeMode,
		StorageClassName:              item.Spec.StorageClassName,
		PersistentVolumeReclaimPolicy: item.Spec.PersistentVolumeReclaimPolicy,
		Phase:                         item.Status.Phase,
		Message:                       item.Status.Message,
	}
	result.Labels = map[string]string{
		common.PfsSelectorKey: item.Labels[common.PfsSelectorKey],
	}
	return result
}
