/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"reflect"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/floatutil"
)

// CreateNodeFlavor handles the creation of a new node flavor resource.
// It authorizes the request, parses the creation request, generates a node flavor object,
// and persists it in the k8s cluster. Returns the created flavor ID on success.
func (h *Handler) CreateNodeFlavor(c *gin.Context) {
	handle(c, h.createNodeFlavor)
}

// ListNodeFlavor handles listing all node flavor resources.
// It retrieves all node flavors, sorts them, and returns them with authorization filtering.
func (h *Handler) ListNodeFlavor(c *gin.Context) {
	handle(c, h.listNodeFlavor)
}

// GetNodeFlavor retrieves detailed information about a specific node flavor.
func (h *Handler) GetNodeFlavor(c *gin.Context) {
	handle(c, h.getNodeFlavor)
}

// PatchNodeFlavor handles partial updates to a node flavor resource.
// Authorizes the request, parses update parameters, and applies changes to the specified node flavor.
func (h *Handler) PatchNodeFlavor(c *gin.Context) {
	handle(c, h.patchNodeFlavor)
}

// DeleteNodeFlavor handles deletion of a node flavor resource.
// Authorizes the request and removes the specified node flavor from the k8s cluster.
func (h *Handler) DeleteNodeFlavor(c *gin.Context) {
	handle(c, h.deleteNodeFlavor)
}

// GetNodeFlavorAvail retrieves the available resources for a specific node flavor.
// Calculates and returns the available resource quantities based on the configuration.
func (h *Handler) GetNodeFlavorAvail(c *gin.Context) {
	handle(c, h.getNodeFlavorAvail)
}

// createNodeFlavor implements the node flavor creation logic.
// Validates the request, generates a node flavor object, and persists it in the k8s cluster.
func (h *Handler) createNodeFlavor(c *gin.Context) (interface{}, error) {
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: v1.NodeFlavorKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &view.CreateNodeFlavorRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	nodeFlavor, err := generateNodeFlavor(c, req)
	if err != nil {
		klog.ErrorS(err, "failed to generate node flavor")
		return nil, err
	}

	if err = h.Create(c.Request.Context(), nodeFlavor); err != nil {
		klog.ErrorS(err, "failed to create nodeFlavor")
		return nil, err
	}
	klog.InfoS("created nodeFlavor", "nodeFlavor", nodeFlavor.Name)
	return &view.CreateNodeFlavorResponse{
		FlavorId: nodeFlavor.Name,
	}, nil
}

// listNodeFlavor implements the node flavor listing logic.
// Retrieves all node flavors, applies authorization filtering, sorts them, and converts to response format.
func (h *Handler) listNodeFlavor(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	nl := &v1.NodeFlavorList{}
	if err = h.List(c.Request.Context(), nl); err != nil {
		klog.ErrorS(err, "failed to list node flavor")
		return nil, err
	}

	result := view.ListNodeFlavorResponse{}
	if len(nl.Items) > 1 {
		sort.Slice(nl.Items, func(i, j int) bool {
			if nl.Items[i].CreationTimestamp.Time.Equal(nl.Items[j].CreationTimestamp.Time) {
				return strings.Compare(nl.Items[i].Name, nl.Items[j].Name) < 0
			}
			return nl.Items[i].CreationTimestamp.Time.Before(nl.Items[j].CreationTimestamp.Time)
		})
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)
	for _, item := range nl.Items {
		if !item.GetDeletionTimestamp().IsZero() {
			continue
		}
		if err = h.accessController.Authorize(authority.AccessInput{
			Context:  c.Request.Context(),
			Resource: &item,
			Verb:     v1.ListVerb,
			User:     requestUser,
			Roles:    roles,
		}); err != nil {
			continue
		}
		result.Items = append(result.Items, cvtToNodeFlavorResponseItem(&item))
	}
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].FlavorId < result.Items[j].FlavorId
	})
	result.TotalCount = len(result.Items)
	return result, nil
}

// getNodeFlavor implements the logic for retrieving a single node flavor's information.
// Gets the node flavor by ID and converts it to a response item format.
func (h *Handler) getNodeFlavor(c *gin.Context) (interface{}, error) {
	nf, err := h.getAdminNodeFlavor(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: nf,
		Verb:     v1.GetVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	return cvtToNodeFlavorResponseItem(nf), nil
}

// patchNodeFlavor implements partial update logic for a node flavor.
// Parses the patch request, applies specified changes, and updates the node flavor.
func (h *Handler) patchNodeFlavor(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	nf, err := h.getAdminNodeFlavor(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  ctx,
		Resource: nf,
		Verb:     v1.UpdateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &view.PatchNodeFlavorRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	originalNodeFlavor := client.MergeFrom(nf.DeepCopy())
	shouldUpdate, err := h.updateNodeFlavor(nf, req)
	if err != nil || !shouldUpdate {
		return nil, err
	}
	if err = h.Patch(ctx, nf, originalNodeFlavor); err != nil {
		klog.ErrorS(err, "failed to patch nodeFlavor", "name", nf.Name)
		return nil, err
	}
	return nil, nil
}

// deleteNodeFlavor implements node flavor deletion logic.
// Retrieves the node flavor and removes it from the k8s cluster.
func (h *Handler) deleteNodeFlavor(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	nf, err := h.getAdminNodeFlavor(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  ctx,
		Resource: nf,
		Verb:     v1.DeleteVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	if err = h.Delete(ctx, nf); err != nil {
		return nil, err
	}
	klog.Infof("delete nodeFlavor %s", nf.Name)
	return nil, nil
}

// getAdminNodeFlavor retrieves a node flavor resource by ID from the k8s cluster.
// Returns an error if the node flavor doesn't exist or the ID is empty.
func (h *Handler) getAdminNodeFlavor(ctx context.Context, flavorId string) (*v1.NodeFlavor, error) {
	if flavorId == "" {
		return nil, commonerrors.NewBadRequest("the nodeFlavorId is empty")
	}
	nf := &v1.NodeFlavor{}
	err := h.Get(ctx, client.ObjectKey{Name: flavorId}, nf)
	if err != nil {
		klog.ErrorS(err, "failed to get node flavor")
		return nil, err
	}
	return nf.DeepCopy(), nil
}

// getNodeFlavorAvail calculates and returns the available resources for a node flavor.
// It computes the available resource quantities based on the node flavor specification
// and system configuration limits.
func (h *Handler) getNodeFlavorAvail(c *gin.Context) (interface{}, error) {
	nf, err := h.getAdminNodeFlavor(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: nf,
		Verb:     v1.GetVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	nodeResources := nf.ToResourceList(commonconfig.GetRdmaName())
	availResource := quantity.GetAvailableResource(nodeResources)
	if !floatutil.FloatEqual(commonconfig.GetMaxEphemeralStorePercent(), 0) {
		maxEphemeralStoreQuantity, _ := quantity.GetMaxEphemeralStoreQuantity(nodeResources)
		if maxEphemeralStoreQuantity != nil {
			availResource[corev1.ResourceEphemeralStorage] = *maxEphemeralStoreQuantity
		}
	}
	return cvtToResourceList(availResource), nil
}

// updateNodeFlavor applies updates to a node flavor based on the patch request.
// Handles changes to CPU, GPU, memory, disk, and extended resources specifications.
// Returns whether any updates were made and any error encountered.
func (h *Handler) updateNodeFlavor(nf *v1.NodeFlavor, req *view.PatchNodeFlavorRequest) (bool, error) {
	shouldUpdate := false
	if req.CPU != nil && !reflect.DeepEqual(nf.Spec.Cpu, *req.CPU) {
		nf.Spec.Cpu = *req.CPU
		shouldUpdate = true
	}
	if req.Gpu != nil && (nf.Spec.Gpu == nil || !reflect.DeepEqual(*nf.Spec.Gpu, *req.Gpu)) {
		nf.Spec.Gpu = req.Gpu
		shouldUpdate = true
	}
	if req.Memory != nil && req.Memory.Value() != nf.Spec.Memory.Value() {
		nf.Spec.Memory = *req.Memory
		shouldUpdate = true
	}
	if req.RootDisk != nil {
		if nf.Spec.RootDisk == nil || !reflect.DeepEqual(*nf.Spec.RootDisk, *req.RootDisk) {
			nf.Spec.RootDisk = req.RootDisk
			shouldUpdate = true
		}
	}
	if req.DataDisk != nil {
		if nf.Spec.DataDisk == nil || !reflect.DeepEqual(*nf.Spec.DataDisk, *req.DataDisk) {
			nf.Spec.DataDisk = req.DataDisk
			shouldUpdate = true
		}
	}
	if req.ExtendResources != nil && !reflect.DeepEqual(req.ExtendResources, nf.Spec.ExtendResources) {
		nf.Spec.ExtendResources = *req.ExtendResources
		shouldUpdate = true
	}
	return shouldUpdate, nil
}

// generateNodeFlavor creates a new node flavor object based on the creation request.
// Populates the node flavor metadata and specification, including automatic ephemeral storage calculation.
func generateNodeFlavor(c *gin.Context, req *view.CreateNodeFlavorRequest) (*v1.NodeFlavor, error) {
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
				v1.UserIdLabel:      c.GetString(common.UserId),
			},
		},
		Spec: req.NodeFlavorSpec,
	}
	if nf.Spec.RootDisk != nil && !nf.Spec.RootDisk.Quantity.IsZero() {
		if nf.Spec.ExtendResources == nil {
			nf.Spec.ExtendResources = make(corev1.ResourceList)
		}
		if _, ok := nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage]; !ok {
			nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(
				nf.Spec.RootDisk.Quantity.Value()*int64(nf.Spec.RootDisk.Count), resource.BinarySI)
		}
	}
	return nf, nil
}

// cvtToNodeFlavorResponseItem converts a node flavor object to a response item format.
// Maps the node flavor specification to the appropriate response structure.
func cvtToNodeFlavorResponseItem(nf *v1.NodeFlavor) view.NodeFlavorResponseItem {
	result := view.NodeFlavorResponseItem{
		FlavorId:       nf.Name,
		NodeFlavorSpec: nf.Spec,
	}
	return result
}
