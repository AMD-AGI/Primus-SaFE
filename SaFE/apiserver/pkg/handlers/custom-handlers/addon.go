/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// CreateAddon handles the creation of a new addon resource.
// It authorizes the request, parses the request body, generates an addon object,
// and creates it in the k8s cluster. Returns the created addon ID on success.
func (h *Handler) CreateAddon(c *gin.Context) {
	handle(c, h.createAddon)
}

// ListAddon handles listing all addon resources.
// Retrieves all addons and returns them in a sorted list with basic information.
func (h *Handler) ListAddon(c *gin.Context) {
	handle(c, h.listAddon)
}

// GetAddon retrieves detailed information about a specific addon.
// Returns comprehensive addon details including configuration and status.
func (h *Handler) GetAddon(c *gin.Context) {
	handle(c, h.getAddon)
}

// DeleteAddon handles the deletion of an addon resource.
// It performs authorization checks and deletes the addon from the cluster.
func (h *Handler) DeleteAddon(c *gin.Context) {
	handle(c, h.deleteAddon)
}

// PatchAddon handles partial updates to an addon resource.
// Authorizes the request, parses update parameters, and applies changes to the specified addon.
func (h *Handler) PatchAddon(c *gin.Context) {
	handle(c, h.patchAddon)
}

// createAddon implements the addon creation logic.
// Authorizes the request, parses the creation request, generates an addon object,
// and persists it in the k8s cluster.
func (h *Handler) createAddon(c *gin.Context) (interface{}, error) {
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: v1.AddonKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.CreateAddonRequestBody{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	addon, err := h.generateAddon(c, req, body)
	if err != nil {
		klog.ErrorS(err, "failed to generate addon")
		return nil, err
	}

	if err = h.Create(c.Request.Context(), addon); err != nil {
		klog.ErrorS(err, "failed to create addon")
		return nil, err
	}
	klog.Infof("created addon %s", addon.Name)
	return cvtToAddonResponseBody(addon), nil
}

// generateAddon converts the CreateAddonRequestBody into a v1.Addon object.
// This function requires an AddonTemplate and uses it to configure the Helm repository.
// Users can optionally override specific Helm configurations via namespace and values.
func (h *Handler) generateAddon(c *gin.Context, req *types.CreateAddonRequestBody, body []byte) (*v1.Addon, error) {
	ctx := c.Request.Context()

	// Validate template is provided
	if req.Template == "" {
		return nil, commonerrors.NewBadRequest("template is required")
	}

	// Get cluster from URL path parameter
	clusterName := c.Param("cluster")
	if clusterName == "" {
		return nil, commonerrors.NewBadRequest("cluster parameter is required in URL path")
	}

	// Validate cluster exists
	cluster, err := h.getAdminCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	// Get AddonTemplate (required)
	addonTemplate, err := h.getAdminAddonTemplate(ctx, req.Template)
	if err != nil {
		klog.ErrorS(err, "failed to get addon template", "template", req.Template)
		return nil, err
	}

	// Validate template type is Helm
	if addonTemplate.Spec.Type != v1.AddonTemplateHelm {
		return nil, commonerrors.NewBadRequest("only helm type addon templates are supported")
	}

	addonName := genAddonName(clusterName, req.Namespace, req.ReleaseName)

	// Create addon with basic info
	addon := &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: addonName,
			Labels: map[string]string{
				v1.DisplayNameLabel: req.ReleaseName,
				v1.UserIdLabel:      c.GetString(common.UserId),
			},
		},
		Spec: v1.AddonSpec{
			Cluster: &corev1.ObjectReference{
				APIVersion: cluster.APIVersion,
				Kind:       cluster.Kind,
				Name:       cluster.Name,
			},
		},
	}

	// Determine namespace - use request value or template default
	namespace := req.Namespace
	if namespace == "" {
		namespace = addonTemplate.Spec.HelmDefaultNamespace
	}

	// Determine values - use request value or template default
	values := req.Values
	if values == "" {
		values = addonTemplate.Spec.HelmDefaultValues
	}

	// Initialize Helm repository with template configuration
	addon.Spec.AddonSource.HelmRepository = &v1.HelmRepository{
		ReleaseName:  req.ReleaseName, // Use addon name as release name
		URL:          addonTemplate.Spec.URL,
		ChartVersion: addonTemplate.Spec.Version,
		Namespace:    namespace,
		Values:       values,
		PlainHTTP:    false,
		Template: &corev1.ObjectReference{
			APIVersion: "amd.com/v1",
			Kind:       v1.AddOnTemplateKind,
			Name:       addonTemplate.Name,
		},
	}

	// Add description if provided
	if req.Description != "" {
		v1.SetAnnotation(addon, v1.DescriptionAnnotation, req.Description)
	}

	return addon, nil
}

// listAddon implements the addon listing logic.
// Retrieves all addons, sorts them by name, and converts them to response items.
func (h *Handler) listAddon(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	addonList := &v1.AddonList{}
	if err := h.List(ctx, addonList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	result := types.ListAddonResponse{}
	if len(addonList.Items) > 0 {
		sort.Slice(addonList.Items, func(i, j int) bool {
			return addonList.Items[i].Name < addonList.Items[j].Name
		})
	}
	for _, item := range addonList.Items {
		result.Items = append(result.Items, cvtToAddonResponseBody(&item))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// getAddon implements the logic for retrieving a single addon's detailed information.
// Gets the addon by ID and converts it to a detailed response format.
func (h *Handler) getAddon(c *gin.Context) (interface{}, error) {
	addon, err := h.getAdminAddon(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	return cvtToAddonResponseBody(addon), nil
}

// deleteAddon handles the deletion of an addon resource.
// It performs authorization checks and deletes the addon from the cluster.
func (h *Handler) deleteAddon(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	addon, err := h.getAdminAddon(ctx, c.GetString(common.Name))
	if err != nil {
		klog.ErrorS(err, "failed to get admin addon")
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: addon,
		Verb:     v1.DeleteVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	if err = h.Delete(ctx, addon); err != nil {
		klog.ErrorS(err, "failed to delete addon")
		return nil, err
	}
	klog.Infof("deleted addon %s", addon.Name)
	return nil, nil
}

// patchAddon implements partial update logic for an addon.
// Parses the patch request and applies specified changes to the addon.
func (h *Handler) patchAddon(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	addon, err := h.getAdminAddon(ctx, c.GetString(common.Name))
	if err != nil {
		klog.ErrorS(err, "failed to get admin addon")
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: addon,
		Verb:     v1.UpdateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.PatchAddonRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	isChanged := h.updateAddon(addon, req)
	if !isChanged {
		return nil, nil
	}
	return nil, h.Update(ctx, addon)
}

// updateAddon applies updates to an addon based on the patch request.
// Handles changes to description, template and Helm values.
func (h *Handler) updateAddon(addon *v1.Addon, req *types.PatchAddonRequest) bool {
	isChanged := false

	// Update description
	if req.Description != nil && *req.Description != "" && *req.Description != v1.GetDescription(addon) {
		v1.SetAnnotation(addon, v1.DescriptionAnnotation, *req.Description)
		isChanged = true
	}

	// Template is required, update it
	if req.Template != nil && *req.Template != "" && addon.Spec.AddonSource.HelmRepository != nil {
		if addon.Spec.AddonSource.HelmRepository.Template != nil {
			if *req.Template != addon.Spec.AddonSource.HelmRepository.Template.Name {
				addon.Spec.AddonSource.HelmRepository.Template.Name = *req.Template
				isChanged = true
			}
		}
	}

	// Update values
	if req.Values != nil && *req.Values != "" && addon.Spec.AddonSource.HelmRepository != nil {
		if *req.Values != addon.Spec.AddonSource.HelmRepository.Values {
			addon.Spec.AddonSource.HelmRepository.Values = *req.Values
			isChanged = true
		}
	}

	return isChanged
}

// getAdminAddon retrieves an addon resource by ID from the k8s cluster.
// Returns an error if the addon doesn't exist or the ID is empty.
func (h *Handler) getAdminAddon(ctx context.Context, addonId string) (*v1.Addon, error) {
	if addonId == "" {
		return nil, commonerrors.NewBadRequest("the addonId is empty")
	}
	addon := &v1.Addon{}
	err := h.Get(ctx, client.ObjectKey{Name: addonId}, addon)
	if err != nil {
		klog.ErrorS(err, "failed to get admin addon")
		return nil, err
	}
	return addon.DeepCopy(), nil
}

// cvtToAddonResponseBody converts an addon object to AddonResponseBody format.
// Used for get response.
func cvtToAddonResponseBody(addon *v1.Addon) types.AddonResponseBody {
	result := types.AddonResponseBody{
		AddonBody: types.AddonBody{
			ReleaseName: addon.Spec.AddonSource.HelmRepository.ReleaseName,
			Description: v1.GetDescription(addon),
		},
		Name: addon.Name,
	}

	// Set cluster
	if addon.Spec.Cluster != nil {
		result.Cluster = addon.Spec.Cluster.Name
	}

	// Set template and namespace/values from Helm repository
	if addon.Spec.AddonSource.HelmRepository != nil {
		helmRepo := addon.Spec.AddonSource.HelmRepository

		if helmRepo.Template != nil {
			result.Template = helmRepo.Template.Name
		}
		result.Namespace = helmRepo.Namespace
		result.Values = helmRepo.Values
	}

	// Convert status information
	if addon.Status.AddonSourceStatus.HelmRepositoryStatus != nil {
		status := addon.Status.AddonSourceStatus.HelmRepositoryStatus
		result.Status = types.AddonStatus{
			FirstDeployed:   status.FirstDeployed,
			LastDeployed:    status.LastDeployed,
			Deleted:         &status.Deleted,
			Description:     status.Description,
			Status:          status.Status,
			Notes:           status.Notes,
			Version:         status.Version,
			ChartVersion:    status.ChartVersion,
			Values:          status.Values,
			PreviousVersion: status.PreviousVersion,
		}
	}

	return result
}

func genAddonName(cluster, namespace, releaseName string) string {
	if namespace == "" {
		return fmt.Sprintf("%s-%s-%s", cluster, "default", releaseName)
	}
	return fmt.Sprintf("%s-%s-%s", cluster, namespace, releaseName)
}
