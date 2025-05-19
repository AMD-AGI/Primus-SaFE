/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

func (h *Handler) GetWorkloadService(c *gin.Context) {
	handle(c, h.getWorkloadService)
}

func (h *Handler) getWorkloadService(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the serviceId is not found")
	}
	adminWorkload, err := h.getAdminWorkload(c.Request.Context(), name)
	if err != nil {
		return nil, commonerrors.NewNotFoundWithMessage("the workload is not found")
	}
	workspace := adminWorkload.Spec.Workspace

	cluster := h.clusterManager.Get(v1.GetClusterId(adminWorkload))
	if cluster == nil {
		err = fmt.Errorf("the cluster %s is not found. pls retry later", v1.GetClusterId(adminWorkload))
		return nil, commonerrors.NewInternalError(err.Error())
	}
	service, err := cluster.ClientSet.CoreV1().Services(workspace).Get(
		c.Request.Context(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return service.Spec, nil
}
