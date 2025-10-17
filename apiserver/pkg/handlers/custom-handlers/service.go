/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// GetWorkloadService: Obtain the service started by the data plane corresponding to this workload.
func (h *Handler) GetWorkloadService(c *gin.Context) {
	handle(c, h.getWorkloadService)
}

// getWorkloadService: retrieves the service specification for a workload from the data plane.
// It validates the workload exists, authorizes the user's access to the workload's workspace,
// obtains the appropriate Kubernetes client for the workload's cluster, and fetches the service
// specification from the target namespace. Returns the service specification or an error.
func (h *Handler) getWorkloadService(c *gin.Context) (interface{}, error) {
	name := c.GetString(common.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the serviceId is empty")
	}

	ctx := c.Request.Context()
	adminWorkload, err := h.getAdminWorkload(ctx, name)
	if err != nil {
		return nil, commonerrors.NewNotFoundWithMessage(err.Error())
	}
	workspace := adminWorkload.Spec.Workspace
	if err = h.auth.Authorize(authority.Input{
		Context:    ctx,
		Resource:   adminWorkload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workspace},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	k8sClients, err := apiutils.GetK8sClientFactory(h.clientManager, v1.GetClusterId(adminWorkload))
	if err != nil {
		return nil, err
	}
	service, err := k8sClients.ClientSet().CoreV1().Services(workspace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return service.Spec, nil
}
