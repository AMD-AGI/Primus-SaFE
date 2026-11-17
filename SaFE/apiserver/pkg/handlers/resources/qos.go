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

// GetQos returns one default ceiling. The name is "default" or a cluster id.
func (h *Handler) GetQos(c *gin.Context) {
	handle(c, h.getQos)
}

// PutQos creates or replaces one default ceiling. System administrators only.
func (h *Handler) PutQos(c *gin.Context) {
	handle(c, h.putQos)
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
	qos, err := h.getQosObject(c, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	return view.NewQosResponse(qos), nil
}

func (h *Handler) putQos(c *gin.Context) (interface{}, error) {
	if err := h.requireSystemAdmin(c); err != nil {
		return nil, err
	}
	req := &view.PutQosRequest{}
	if _, err := apiutils.ParseRequestBody(c.Request, req); err != nil {
		return nil, err
	}
	if err := view.ValidateQosLimits(req.MaxResources, req.MaxRuntime); err != nil {
		return nil, err
	}
	name := c.GetString(common.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("qos name is required")
	}
	qos := &v1.Qos{}
	err := h.Get(c.Request.Context(), client.ObjectKey{Name: name}, qos)
	if apierrors.IsNotFound(err) {
		qos = &v1.Qos{ObjectMeta: metav1.ObjectMeta{Name: name}}
		qos.Spec.MaxResources = req.MaxResources
		qos.Spec.MaxRuntime = req.MaxRuntime
		if err = h.Create(c.Request.Context(), qos); err != nil {
			return nil, err
		}
		return view.NewQosResponse(qos), nil
	}
	if err != nil {
		return nil, err
	}
	patch := client.MergeFrom(qos.DeepCopy())
	qos.Spec.MaxResources = req.MaxResources
	qos.Spec.MaxRuntime = req.MaxRuntime
	if err = h.Patch(c.Request.Context(), qos, patch); err != nil {
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
