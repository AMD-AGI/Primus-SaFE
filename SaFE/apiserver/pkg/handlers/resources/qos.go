/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// ListQos returns every default ceiling. System administrators only.
func (h *Handler) ListQos(c *gin.Context) {
	handle(c, h.listQos)
}

// GetQos returns one workspace ceiling. The name is the workspace id.
func (h *Handler) GetQos(c *gin.Context) {
	handle(c, h.getQos)
}

// CreateQos creates one workspace ceiling. System administrators only.
func (h *Handler) CreateQos(c *gin.Context) {
	handle(c, h.createQos)
}

// UpdateQos replaces one workspace ceiling. A timeout or resource left out of the request is cleared.
func (h *Handler) UpdateQos(c *gin.Context) {
	handle(c, h.updateQos)
}

// DeleteQos removes one default ceiling. System administrators only.
func (h *Handler) DeleteQos(c *gin.Context) {
	handle(c, h.deleteQos)
}

func (h *Handler) listQos(c *gin.Context) (interface{}, error) {
	if err := h.requireSystemAdmin(c); err != nil {
		return nil, err
	}
	list := &v1.QosList{}
	if err := h.List(c.Request.Context(), list); err != nil {
		return nil, err
	}
	items := make([]view.QosResponse, 0, len(list.Items))
	for i := range list.Items {
		items = append(items, view.NewQosResponse(&list.Items[i]))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return &view.ListQosResponse{Items: items}, nil
}

func (h *Handler) getQos(c *gin.Context) (interface{}, error) {
	if err := h.requireSystemAdmin(c); err != nil {
		return nil, err
	}
	name := c.GetString(common.Name)
	qos, err := h.getQosObject(c, name)
	if err != nil {
		return nil, err
	}
	return view.NewQosResponse(qos), nil
}

// readQosRequest checks the caller and normalizes one create or update body.
func (h *Handler) readQosRequest(c *gin.Context) (string, *view.PutQosRequest, error) {
	if err := h.requireSystemAdmin(c); err != nil {
		return "", nil, err
	}
	req := &view.PutQosRequest{}
	if _, err := apiutils.ParseRequestBody(c.Request, req); err != nil {
		return "", nil, err
	}
	req.MaxResources = view.NormalizeQosResources(req.MaxResources)
	if len(req.MaxRuntime) == 0 {
		req.MaxRuntime = nil
	}
	if err := view.ValidateQosLimits(req.MaxResources, req.MaxRuntime); err != nil {
		return "", nil, err
	}
	name := c.GetString(common.Name)
	if name == "" {
		return "", nil, commonerrors.NewBadRequest("qos name is required")
	}
	return name, req, nil
}

// applyQosSpec copies a request onto a Qos. Nil fields clear that dimension.
func applyQosSpec(qos *v1.Qos, req *view.PutQosRequest) {
	qos.Spec.MaxResources = req.MaxResources
	qos.Spec.MaxRuntime = req.MaxRuntime
}

func (h *Handler) createQos(c *gin.Context) (interface{}, error) {
	name, req, err := h.readQosRequest(c)
	if err != nil {
		return nil, err
	}
	existing := &v1.Qos{}
	err = h.Get(c.Request.Context(), client.ObjectKey{Name: name}, existing)
	if err == nil {
		return nil, commonerrors.NewConflict(fmt.Sprintf("qos %s already exists", name))
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	qos := &v1.Qos{ObjectMeta: metav1.ObjectMeta{Name: name}}
	applyQosSpec(qos, req)
	if err = h.Create(c.Request.Context(), qos); err != nil {
		return nil, err
	}
	return view.NewQosResponse(qos), nil
}

func (h *Handler) updateQos(c *gin.Context) (interface{}, error) {
	name, req, err := h.readQosRequest(c)
	if err != nil {
		return nil, err
	}
	qos, err := h.getQosObject(c, name)
	if err != nil {
		return nil, err
	}
	applyQosSpec(qos, req)
	if err = h.Update(c.Request.Context(), qos); err != nil {
		klog.ErrorS(err, "failed to update qos", "name", name)
		return nil, err
	}
	return view.NewQosResponse(qos), nil
}

func (h *Handler) deleteQos(c *gin.Context) (interface{}, error) {
	if err := h.requireSystemAdmin(c); err != nil {
		return nil, err
	}
	qos, err := h.getQosObject(c, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.Delete(c.Request.Context(), qos); err != nil {
		return nil, err
	}
	return nil, nil
}

// requireSystemAdmin rejects every caller that is not a system administrator.
func (h *Handler) requireSystemAdmin(c *gin.Context) error {
	return h.accessController.AuthorizeSystemAdmin(authority.AccessInput{
		Context: c.Request.Context(),
		UserId:  c.GetString(common.UserId),
	}, false)
}

func (h *Handler) getQosObject(c *gin.Context, name string) (*v1.Qos, error) {
	qos := &v1.Qos{}
	if err := h.Get(c.Request.Context(), client.ObjectKey{Name: name}, qos); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("qos %s is not found", name))
		}
		return nil, err
	}
	return qos, nil
}
