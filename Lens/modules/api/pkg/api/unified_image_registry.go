// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Request / Response types =====

// ImageRegistryGetRequest retrieves image analysis by digest
type ImageRegistryGetRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Digest  string `json:"digest" param:"digest" mcp:"digest,description=Image SHA256 digest,required"`
}

// ImageRegistryGetResponse returns cached image registry analysis
type ImageRegistryGetResponse struct {
	ID                int64       `json:"id"`
	ImageRef          string      `json:"image_ref"`
	Digest            string      `json:"digest"`
	Registry          string      `json:"registry,omitempty"`
	Repository        string      `json:"repository,omitempty"`
	Tag               string      `json:"tag,omitempty"`
	BaseImage         string      `json:"base_image,omitempty"`
	LayerCount        int         `json:"layer_count,omitempty"`
	LayerHistory      interface{} `json:"layer_history,omitempty"`
	ImageLabels       interface{} `json:"image_labels,omitempty"`
	ImageEnv          interface{} `json:"image_env,omitempty"`
	ImageEntrypoint   string      `json:"image_entrypoint,omitempty"`
	InstalledPackages interface{} `json:"installed_packages,omitempty"`
	FrameworkHints    interface{} `json:"framework_hints,omitempty"`
	TotalSize         int64       `json:"total_size,omitempty"`
	CreatedAt         string      `json:"created_at,omitempty"`
	CachedAt          string      `json:"cached_at,omitempty"`
}

// ImageRegistryListRequest lists cached images with filters
type ImageRegistryListRequest struct {
	Cluster    string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Registry   string `json:"registry" query:"registry" mcp:"registry,description=Filter by registry host"`
	Repository string `json:"repository" query:"repository" mcp:"repository,description=Filter by repository path (partial match)"`
	PageNum    int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize   int    `json:"page_size" query:"page_size" mcp:"page_size,description=Items per page (default 20)"`
}

// ImageRegistryListResponse returns paginated image analysis results
type ImageRegistryListResponse struct {
	Data  []ImageRegistryGetResponse `json:"data"`
	Total int64                      `json:"total"`
}

// ===== Endpoint registration =====

func init() {
	unified.Register(&unified.EndpointDef[ImageRegistryGetRequest, ImageRegistryGetResponse]{
		Name:        "image_registry_get",
		Description: "Get cached image registry analysis by digest (layer history, installed packages, framework hints)",
		HTTPMethod:  "GET",
		HTTPPath:    "/image-registry/:digest",
		MCPToolName: "lens_image_registry_get",
		Handler:     handleImageRegistryGet,
	})

	unified.Register(&unified.EndpointDef[ImageRegistryListRequest, ImageRegistryListResponse]{
		Name:        "image_registry_list",
		Description: "List cached image registry analysis results with filtering",
		HTTPMethod:  "GET",
		HTTPPath:    "/image-registry",
		MCPToolName: "lens_image_registry_list",
		Handler:     handleImageRegistryList,
	})
}

// ===== Handlers =====

func handleImageRegistryGet(ctx context.Context, req *ImageRegistryGetRequest) (*ImageRegistryGetResponse, error) {
	if req.Digest == "" {
		return nil, errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("digest is required")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)
	cache, err := facade.GetImageRegistryCache().GetByDigest(ctx, req.Digest)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to get image registry cache: " + err.Error())
	}
	if cache == nil {
		return nil, errors.NewError().
			WithCode(errors.RequestDataNotExisted).
			WithMessage("no image cache found for digest: " + req.Digest)
	}

	return convertImageCacheToResponse(cache), nil
}

func handleImageRegistryList(ctx context.Context, req *ImageRegistryListRequest) (*ImageRegistryListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)

	limit := pageSize
	offset := (pageNum - 1) * pageSize

	caches, total, err := facade.GetImageRegistryCache().List(ctx, req.Registry, req.Repository, limit, offset)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to list image registry cache: " + err.Error())
	}

	data := make([]ImageRegistryGetResponse, 0, len(caches))
	for _, c := range caches {
		data = append(data, *convertImageCacheToResponse(c))
	}

	return &ImageRegistryListResponse{
		Data:  data,
		Total: total,
	}, nil
}

// ===== Conversion helpers =====

func convertImageCacheToResponse(c *dbModel.ImageRegistryCache) *ImageRegistryGetResponse {
	resp := &ImageRegistryGetResponse{
		ID:                c.ID,
		ImageRef:          c.ImageRef,
		Digest:            c.Digest,
		Registry:          c.Registry,
		Repository:        c.Repository,
		Tag:               c.Tag,
		BaseImage:         c.BaseImage,
		LayerCount:        int(c.LayerCount),
		LayerHistory:      c.LayerHistory,
		ImageLabels:       c.ImageLabels,
		ImageEnv:          c.ImageEnv,
		ImageEntrypoint:   c.ImageEntrypoint,
		InstalledPackages: c.InstalledPackages,
		FrameworkHints:    c.FrameworkHints,
		TotalSize:         c.TotalSize,
	}

	if c.ImageCreatedAt != nil {
		resp.CreatedAt = c.ImageCreatedAt.Format("2006-01-02T15:04:05Z")
	}
	resp.CachedAt = c.CachedAt.Format("2006-01-02T15:04:05Z")

	return resp
}
