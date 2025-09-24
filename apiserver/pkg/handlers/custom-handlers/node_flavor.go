/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/floatutil"
)

func (h *Handler) CreateNodeFlavor(c *gin.Context) {
	handle(c, h.createNodeFlavor)
}

func (h *Handler) ListNodeFlavor(c *gin.Context) {
	handle(c, h.listNodeFlavor)
}

func (h *Handler) GetNodeFlavor(c *gin.Context) {
	handle(c, h.getNodeFlavor)
}

func (h *Handler) PatchNodeFlavor(c *gin.Context) {
	handle(c, h.patchNodeFlavor)
}

func (h *Handler) DeleteNodeFlavor(c *gin.Context) {
	handle(c, h.deleteNodeFlavor)
}

func (h *Handler) GetNodeFlavorAvail(c *gin.Context) {
	handle(c, h.getNodeFlavorAvail)
}

func (h *Handler) createNodeFlavor(c *gin.Context) (interface{}, error) {
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: v1.NodeFlavorKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.CreateNodeFlavorRequest{}
	body, err := getBodyFromRequest(c.Request, req)
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
	return &types.CreateNodeFlavorResponse{
		FlavorId: nodeFlavor.Name,
	}, nil
}

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

	result := types.ListNodeFlavorResponse{}
	if result.TotalCount > 0 {
		sort.Slice(nl.Items, func(i, j int) bool {
			if nl.Items[i].CreationTimestamp.Time.Equal(nl.Items[j].CreationTimestamp.Time) {
				return strings.Compare(nl.Items[i].Name, nl.Items[j].Name) < 0
			}
			return nl.Items[i].CreationTimestamp.Time.Before(nl.Items[j].CreationTimestamp.Time)
		})
	}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)
	for _, item := range nl.Items {
		if !item.GetDeletionTimestamp().IsZero() {
			continue
		}
		if err = h.auth.Authorize(authority.Input{
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

func (h *Handler) getNodeFlavor(c *gin.Context) (interface{}, error) {
	nf, err := h.getAdminNodeFlavor(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:  c.Request.Context(),
		Resource: nf,
		Verb:     v1.GetVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	return cvtToNodeFlavorResponseItem(nf), nil
}

func (h *Handler) patchNodeFlavor(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	nf, err := h.getAdminNodeFlavor(ctx, c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:  ctx,
		Resource: nf,
		Verb:     v1.UpdateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.PatchNodeFlavorRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	patch := client.MergeFrom(nf.DeepCopy())
	isShouldUpdate, err := h.updateNodeFlavor(nf, req)
	if err != nil || !isShouldUpdate {
		return nil, err
	}
	if err = h.Patch(ctx, nf, patch); err != nil {
		klog.ErrorS(err, "failed to patch nodeFlavor", "name", nf.Name)
		return nil, err
	}
	return nil, nil
}

func (h *Handler) deleteNodeFlavor(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	nf, err := h.getAdminNodeFlavor(ctx, c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
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

func (h *Handler) getAdminNodeFlavor(ctx context.Context, name string) (*v1.NodeFlavor, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the nodeFlavorId is empty")
	}
	nf := &v1.NodeFlavor{}
	err := h.Get(ctx, client.ObjectKey{Name: name}, nf)
	if err != nil {
		klog.ErrorS(err, "failed to get node flavor")
		return nil, err
	}
	return nf.DeepCopy(), nil
}

func (h *Handler) getNodeFlavorAvail(c *gin.Context) (interface{}, error) {
	nf, err := h.getAdminNodeFlavor(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
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

func (h *Handler) updateNodeFlavor(nf *v1.NodeFlavor, req *types.PatchNodeFlavorRequest) (bool, error) {
	isShouldUpdate := false
	if req.CPU != nil && !reflect.DeepEqual(nf.Spec.Cpu, *req.CPU) {
		nf.Spec.Cpu = *req.CPU
		isShouldUpdate = true
	}
	if req.Gpu != nil && (nf.Spec.Gpu == nil || !reflect.DeepEqual(*nf.Spec.Gpu, *req.Gpu)) {
		nf.Spec.Gpu = req.Gpu
		isShouldUpdate = true
	}
	if req.Memory != nil && req.Memory.Value() != nf.Spec.Memory.Value() {
		nf.Spec.Memory = *req.Memory
		isShouldUpdate = true
	}
	if req.RootDisk != nil {
		if nf.Spec.RootDisk == nil || !reflect.DeepEqual(*nf.Spec.RootDisk, *req.RootDisk) {
			nf.Spec.RootDisk = req.RootDisk
			isShouldUpdate = true
		}
	}
	if req.DataDisk != nil {
		if nf.Spec.DataDisk == nil || !reflect.DeepEqual(*nf.Spec.DataDisk, *req.DataDisk) {
			nf.Spec.DataDisk = req.DataDisk
			isShouldUpdate = true
		}
	}
	if req.ExtendResources != nil && !reflect.DeepEqual(req.ExtendResources, nf.Spec.ExtendResources) {
		nf.Spec.ExtendResources = *req.ExtendResources
		isShouldUpdate = true
	}
	return isShouldUpdate, nil
}

func generateNodeFlavor(c *gin.Context, req *types.CreateNodeFlavorRequest) (*v1.NodeFlavor, error) {
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
	_, ok := nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage]
	if !ok && nf.Spec.RootDisk != nil && !nf.Spec.RootDisk.Quantity.IsZero() {
		nf.Spec.ExtendResources[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(
			nf.Spec.RootDisk.Quantity.Value()*int64(nf.Spec.RootDisk.Count), resource.BinarySI)
	}
	return nf, nil
}

func cvtToNodeFlavorResponseItem(nf *v1.NodeFlavor) types.NodeFlavorResponseItem {
	result := types.NodeFlavorResponseItem{
		FlavorId:       nf.Name,
		NodeFlavorSpec: nf.Spec,
	}
	return result
}
