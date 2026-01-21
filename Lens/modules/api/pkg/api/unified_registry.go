// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	regconf "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/registry"
)

func init() {
	// Registry Configuration endpoints (GET only)
	unified.Register(&unified.EndpointDef[RegistryConfigRequest, RegistryConfigResponse]{
		Name:        "registry_config",
		Description: "Get current container registry configuration for a cluster",
		HTTPMethod:  "GET",
		HTTPPath:    "/registry/config",
		MCPToolName: "lens_registry_config",
		Handler:     handleRegistryConfig,
	})

	unified.Register(&unified.EndpointDef[RegistryImageURLRequest, RegistryImageURLResponse]{
		Name:        "registry_image_url",
		Description: "Get full image URL for a specific image name with registry prefix",
		HTTPMethod:  "GET",
		HTTPPath:    "/registry/image-url",
		MCPToolName: "lens_registry_image_url",
		Handler:     handleRegistryImageURL,
	})
}

// ======================== Request Types ========================

type RegistryConfigRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
}

type RegistryImageURLRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	Image   string `json:"image" form:"image" binding:"required" mcp:"description=Image name (e.g. tracelens or perfetto_viewer),required"`
	Tag     string `json:"tag" form:"tag" mcp:"description=Image tag (default: latest)"`
}

// ======================== Response Types ========================

type RegistryConfigResponse struct {
	Config     *regconf.Config        `json:"config"`
	Defaults   map[string]string      `json:"defaults"`
	ImageNames map[string]string      `json:"image_names"`
}

type RegistryImageURLResponse struct {
	ImageName string `json:"image_name"`
	Tag       string `json:"tag"`
	ImageURL  string `json:"image_url"`
}

// ======================== Handler Implementations ========================

func handleRegistryConfig(ctx context.Context, req *RegistryConfigRequest) (*RegistryConfigResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	cfg, err := regconf.GetConfig(ctx, clients.ClusterName)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get registry config", errors.CodeDatabaseError)
	}

	return &RegistryConfigResponse{
		Config: cfg,
		Defaults: map[string]string{
			"registry":  regconf.DefaultRegistry,
			"namespace": regconf.DefaultNamespace,
		},
		ImageNames: map[string]string{
			"tracelens":       regconf.ImageTraceLens,
			"perfetto_viewer": regconf.ImagePerfettoViewer,
		},
	}, nil
}

func handleRegistryImageURL(ctx context.Context, req *RegistryImageURLRequest) (*RegistryImageURLResponse, error) {
	if req.Image == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("image parameter is required")
	}

	tag := req.Tag
	if tag == "" {
		tag = "latest"
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	imageURL := regconf.GetImageURLForCluster(ctx, clients.ClusterName, req.Image, tag)

	return &RegistryImageURLResponse{
		ImageName: req.Image,
		Tag:       tag,
		ImageURL:  imageURL,
	}, nil
}
