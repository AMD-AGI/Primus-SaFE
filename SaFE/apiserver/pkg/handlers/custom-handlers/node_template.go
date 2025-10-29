/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// CreateNodeTemplate handles the creation of a new node template resource.
// It authorizes the request, parses the creation request, generates a node template object,
// and persists it in the k8s cluster. Returns the created template ID on success.
func (h *Handler) CreateNodeTemplate(c *gin.Context) {
	handle(c, h.createNodeTemplate)
}

// ListNodeTemplate handles listing all node template resources.
// It retrieves all node templates, applies authorization filtering, and returns them in a list.
func (h *Handler) ListNodeTemplate(c *gin.Context) {
	handle(c, h.listNodeTemplate)
}

// DeleteNodeTemplate handles deletion of a node template resource.
// It authorizes the request and removes the specified node template from the k8s cluster.
func (h *Handler) DeleteNodeTemplate(c *gin.Context) {
	handle(c, h.deleteNodeTemplate)
}

// createNodeTemplate implements the node template creation logic.
// Validates the request, generates a node template object, and persists it in the k8s cluster.
func (h *Handler) createNodeTemplate(c *gin.Context) (interface{}, error) {
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: v1.NodeTemplateKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.CreateNodeTemplateRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	nt := generateNodeTemplate(c, req)
	if err = h.Create(c.Request.Context(), nt); err != nil {
		return nil, err
	}
	return &types.CreateNodeTemplateResponse{
		Id: nt.Name,
	}, nil
}

// listNodeTemplate implements the node template listing logic.
// Retrieves all node templates, applies authorization filtering, and converts them to response format.
func (h *Handler) listNodeTemplate(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	nts := &v1.NodeTemplateList{}
	err = h.List(c.Request.Context(), nts)
	if err != nil {
		return nil, err
	}
	result := types.ListNodeTemplateResponse{}
	roles := h.auth.GetRoles(c.Request.Context(), requestUser)
	for _, nt := range nts.Items {
		if !nt.GetDeletionTimestamp().IsZero() {
			continue
		}
		if err = h.auth.Authorize(authority.Input{
			Context:  c.Request.Context(),
			Resource: &nt,
			Verb:     v1.ListVerb,
			User:     requestUser,
			Roles:    roles,
		}); err != nil {
			continue
		}
		result.Items = append(result.Items, types.NodeTemplateResponseItem{
			TemplateId:     nt.Name,
			AddOnTemplates: nt.Spec.AddOnTemplates,
		})
		result.TotalCount++
	}
	return result, nil
}

// deleteNodeTemplate implements node template deletion logic.
// Retrieves the node template by name and removes it from the k8s cluster.
func (h *Handler) deleteNodeTemplate(c *gin.Context) (interface{}, error) {
	name := c.GetString(common.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the nodeTemplateId is not found")
	}
	ctx := c.Request.Context()
	nt, err := h.getAdminNodeTemplate(ctx, name)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:  ctx,
		Resource: nt,
		Verb:     v1.DeleteVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	return nil, h.Delete(ctx, nt)
}

// getAdminNodeTemplate retrieves a node template resource by name from the k8s cluster.
// Returns an error if the node template doesn't exist or the name is empty.
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

// generateNodeTemplate creates a new node template object based on the creation request.
// Normalizes the template name and populates the node template metadata and specification.
func generateNodeTemplate(c *gin.Context, req *types.CreateNodeTemplateRequest) *v1.NodeTemplate {
	return &v1.NodeTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: stringutil.NormalizeName(req.Name),
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
				v1.UserIdLabel:      c.GetString(common.UserId),
			},
		},
		Spec: v1.NodeTemplateSpec{
			AddOnTemplates: req.AddOnTemplates,
		},
	}
}
