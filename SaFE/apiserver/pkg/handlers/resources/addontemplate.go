/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"sort"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// ListAddonTemplate handles listing all addon template resources.
// Retrieves all addon templates and returns them in a sorted list with basic information.
func (h *Handler) ListAddonTemplate(c *gin.Context) {
	handle(c, h.listAddonTemplate)
}

// GetAddonTemplate retrieves detailed information about a specific addon template.
// Returns comprehensive addon template details including configuration and status.
func (h *Handler) GetAddonTemplate(c *gin.Context) {
	handle(c, h.getAddonTemplate)
}

// listAddonTemplate implements the addon template listing logic.
// Retrieves all addon templates, sorts them by name, and converts them to response items.
// Supports filtering by type parameter from query string.
func (h *Handler) listAddonTemplate(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	addonTemplateList := &v1.AddonTemplateList{}
	if err := h.List(ctx, addonTemplateList, &client.ListOptions{}); err != nil {
		return nil, err
	}

	// Get type filter from query parameter
	typeFilter := c.Query("type")

	result := view.ListAddonTemplateResponse{}
	if len(addonTemplateList.Items) > 0 {
		sort.Slice(addonTemplateList.Items, func(i, j int) bool {
			return addonTemplateList.Items[i].Name < addonTemplateList.Items[j].Name
		})
	}
	for _, item := range addonTemplateList.Items {
		// Filter by type if specified
		if typeFilter != "" && string(item.Spec.Type) != typeFilter {
			continue
		}
		result.Items = append(result.Items, cvtToAddonTemplateResponseItem(&item))
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// getAddonTemplate implements the logic for retrieving a single addon template's detailed information.
// Gets the addon template by ID and converts it to a detailed response format.
func (h *Handler) getAddonTemplate(c *gin.Context) (interface{}, error) {
	addonTemplate, err := h.getAdminAddonTemplate(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	return cvtToGetAddonTemplateResponse(addonTemplate), nil
}

// getAdminAddonTemplate retrieves an addon template resource by ID from the k8s cluster.
// Returns an error if the addon template doesn't exist or the ID is empty.
func (h *Handler) getAdminAddonTemplate(ctx context.Context, addonTemplateId string) (*v1.AddonTemplate, error) {
	if addonTemplateId == "" {
		return nil, commonerrors.NewBadRequest("the addonTemplateId is empty")
	}
	addonTemplate := &v1.AddonTemplate{}
	err := h.Get(ctx, client.ObjectKey{Name: addonTemplateId}, addonTemplate)
	if err != nil {
		klog.ErrorS(err, "failed to get admin addon template")
		return nil, err
	}
	return addonTemplate.DeepCopy(), nil
}

// cvtToAddonTemplateResponseItem converts an addon template object to a response item format.
// Includes basic addon template information like ID, type, category, version, and creation time.
func cvtToAddonTemplateResponseItem(addonTemplate *v1.AddonTemplate) view.AddonTemplateResponseItem {
	result := view.AddonTemplateResponseItem{
		AddonTemplateId: addonTemplate.Name,
		Type:            string(addonTemplate.Spec.Type),
		Category:        addonTemplate.Spec.Category,
		Version:         addonTemplate.Spec.Version,
		Description:     addonTemplate.Spec.Description,
		GpuChip:         string(addonTemplate.Spec.GpuChip),
		Required:        addonTemplate.Spec.Required,
		CreationTime:    timeutil.FormatRFC3339(addonTemplate.CreationTimestamp.Time),
	}
	return result
}

// cvtToGetAddonTemplateResponse converts an addon template object to a detailed response format.
// Includes all addon template details, configuration parameters, and status information.
func cvtToGetAddonTemplateResponse(addonTemplate *v1.AddonTemplate) view.GetAddonTemplateResponse {
	result := view.GetAddonTemplateResponse{
		AddonTemplateResponseItem: cvtToAddonTemplateResponseItem(addonTemplate),
		URL:                       addonTemplate.Spec.URL,
		Action:                    addonTemplate.Spec.Action,
		Icon:                      addonTemplate.Spec.Icon,
		HelmDefaultValues:         addonTemplate.Spec.HelmDefaultValues,
		HelmDefaultNamespace:      addonTemplate.Spec.HelmDefaultNamespace,
	}

	// Add helm status information if available
	if addonTemplate.Status.HelmStatus.Values != "" || addonTemplate.Status.HelmStatus.ValuesYAML != "" {
		result.HelmStatus = &view.HelmStatusResponse{
			Values:     addonTemplate.Status.HelmStatus.Values,
			ValuesYAML: addonTemplate.Status.HelmStatus.ValuesYAML,
		}
	}

	return result
}
