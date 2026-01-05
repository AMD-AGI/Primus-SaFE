/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"strconv"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// GetWorkloadService Obtain the service started by the data plane corresponding to this workload.
func (h *Handler) GetWorkloadService(c *gin.Context) {
	handle(c, h.getWorkloadService)
}

// getWorkloadService retrieves the service specification for a workload from the data plane.
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
		return nil, client.IgnoreNotFound(err)
	}
	workspace := adminWorkload.Spec.Workspace
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   adminWorkload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workspace},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	k8sClients, err := commonutils.GetK8sClientFactory(h.clientManager, v1.GetClusterId(adminWorkload))
	if err != nil {
		return nil, err
	}
	service, err := k8sClients.ClientSet().CoreV1().Services(workspace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if len(service.Spec.Ports) == 0 {
		return nil, commonerrors.NewNotFoundWithMessage("service does not have any ports")
	}
	result := &view.GetWorkloadServiceResponse{
		Port:      service.Spec.Ports[0],
		ClusterIp: service.Spec.ClusterIP,
		Type:      service.Spec.Type,
	}
	internalDomain := adminWorkload.Name + "." + adminWorkload.Spec.Workspace +
		".svc.cluster.local:" + strconv.Itoa(int(service.Spec.Ports[0].Port))
	result.InternalDomain = internalDomain
	if commonconfig.GetIngress() == common.HigressClassname && commonconfig.GetSystemHost() != "" {
		result.ExternalDomain = "https://" + commonconfig.GetSystemHost() +
			"/" + v1.GetClusterId(adminWorkload) + "/" + adminWorkload.Spec.Workspace + "/" + adminWorkload.Name + "/"
	}
	return result, nil
}
