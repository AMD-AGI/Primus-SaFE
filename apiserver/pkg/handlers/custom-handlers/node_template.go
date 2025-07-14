/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func (h *Handler) CreateNodeTemplate(c *gin.Context) {
	handle(c, h.createNodeTemplate)
}

func (h *Handler) ListNodeTemplate(c *gin.Context) {
	handle(c, h.listNodeTemplate)
}

func (h *Handler) DeleteNodeTemplate(c *gin.Context) {
	handle(c, h.deleteNodeTemplate)
}

func (h *Handler) createNodeTemplate(c *gin.Context) (interface{}, error) {
	req := &types.CreateNodeTemplateRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	nt := generateNodeTemplate(req)
	if err = h.Create(c.Request.Context(), nt); err != nil {
		return nil, err
	}
	return &types.CreateNodeTemplateResponse{
		Id: nt.Name,
	}, nil
}

func (h *Handler) listNodeTemplate(c *gin.Context) (interface{}, error) {
	nts := &v1.NodeTemplateList{}
	err := h.List(c.Request.Context(), nts)
	if err != nil {
		return nil, err
	}
	result := types.GetNodeTemplateResponse{
		TotalCount: len(nts.Items),
	}
	for i := range nts.Items {
		result.Items = append(result.Items, types.GetNodeTemplateResponseItem{
			AddOnTemplates: nts.Items[i].Spec.AddOnTemplates,
		})
	}
	return result, nil
}

func (h *Handler) deleteNodeTemplate(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the nodeTemplateId is not found")
	}
	ctx := c.Request.Context()
	nt, err := h.getAdminNodeTemplate(ctx, name)
	if err != nil {
		return nil, err
	}
	return nil, h.Delete(ctx, nt)
}

func (h *Handler) getAdminNodeTemplate(ctx context.Context, name string) (*v1.NodeTemplate, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the nodeTemplateId is empty")
	}
	nt := &v1.NodeTemplate{}
	err := h.Get(ctx, client.ObjectKey{Name: name}, nt)
	if err != nil {
		klog.ErrorS(err, "failed to get node template")
		return nil, err
	}
	return nt.DeepCopy(), nil
}

func generateNodeTemplate(req *types.CreateNodeTemplateRequest) *v1.NodeTemplate {
	nt := &v1.NodeTemplate{}
	nt.Name = stringutil.NormalizeName(req.Name)
	metav1.SetMetaDataLabel(&nt.ObjectMeta, v1.DisplayNameLabel, req.Name)
	nt.Spec.AddOnTemplates = req.AddOnTemplates
	return nt
}
