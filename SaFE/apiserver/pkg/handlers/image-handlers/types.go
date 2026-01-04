/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"fmt"
	"time"
)

type ImageServiceRequest struct {
	PageNum   int    `form:"page_num" binding:"omitempty,min=0" `
	PageSize  int    `form:"page_size" binding:"omitempty,min=1"`
	OrderBy   string `form:"orderBy" binding:"omitempty"`
	Order     string `form:"order" binding:"omitempty,oneof=desc asc"`
	Tag       string `form:"tag" binding:"omitempty"`
	Ready     bool   `form:"ready"  binding:"omitempty"`
	UserName  string `form:"userName" binding:"omitempty"`
	Workload  string `form:"workload" binding:"omitempty"` // Filter by workload ID
	Image     string `form:"image" binding:"omitempty"`
	Workspace string `form:"workspace" binding:"omitempty"`
	Status    string `form:"status" binding:"omitempty"`
	Flat      bool   `form:"flat" binding:"omitempty"`
}

type ArtifactItem struct {
	ImageTag    string `json:"imageTag"`
	Description string `json:"description,omitempty"`
	CreatedTime string `json:"createdTime,omitempty"`
	UserName    string `json:"userName,omitempty"`
	Status      string `json:"status"`
	Id          int32  `json:"id"`
	Size        int64  `json:"size"`
	Arch        string `json:"arch"`
	Os          string `json:"os"`
	Digest      string `json:"digest,omitempty"`
	IncludeType string `json:"includeType"`
	SecretId    string `json:"secretId,omitempty"`
}

type GetImageResponseItem struct {
	RegistryHost string         `json:"registryHost"`
	Repo         string         `json:"repo"`
	Artifacts    []ArtifactItem `json:"artifacts"`
}

type GetImageResponse struct {
	TotalCount int                    `json:"totalCount"`
	Items      []GetImageResponseItem `json:"images,omitempty"`
}

type ImportImageResponse struct {
	State          int    `json:"state"`
	Message        string `json:"message"`
	AlreadyImageID int32  `json:"alreadyImageId"`
}

type ImportImageEnv struct {
	SourceImageName string `json:"sourceImageName"` // full name, e.g: docker.io/library/nginx:latest
	DestImageName   string `json:"destImageName"`   // full name, e.g: harbor.my.domain/my-repo/nginx:latest
	OsArch          string `json:"osArch"`          // e.g: linux/amd64
	Os              string `json:"os"`              // e.g: linux
	Arch            string `json:"arch"`            // e.g: amd64
	Description     string `json:"description"`
}

type ImportImageServiceRequest struct {
	Source   string `json:"source"`
	SecretId string `json:"secretId,omitempty"`
}

type ImportImageMetaInfo struct {
	SourceImageName string `json:"sourceImageName"`
	DestImageName   string `json:"destImageName"`
	OsArch          string `json:"osArch"`
	Os              string `json:"os"`
	Arch            string `json:"arch"`
	Size            int64  `json:"size"`
	IncludeType     int    `json:"includeType"`
	Status          string `json:"status"`
}

type ImportImageLogsResponse struct {
	Logs        string      `json:"logs"`
	ImageID     int32       `json:"image_id"`
	State       int32       `json:"state"`
	OsArch      string      `json:"os_arch"`
	SrcName     string      `json:"src_name"`
	DestName    string      `json:"dest_name"`
	CreatedTime time.Time   `json:"created_time"`
	UpdatedTime time.Time   `json:"updated_time"`
	ExecLog     interface{} `json:"exec_log"`
}

type CreateImageRequest struct {
	Registry    string `json:"registry"`
	ImageTag    string `json:"imageTag"`
	Description string `json:"description"`
	IsShare     bool   `json:"isShare"`
}

// Valid validates the request parameters.
func (c CreateImageRequest) Valid() (bool, string) {
	if c.ImageTag == "" {
		return false, "imageTag is required"
	}
	return true, ""
}

type RelationDigest struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size,omitempty"`
}

const (
	// exp: /api/v2.0/projects/test/repositories/preprocess/artifacts/sha256:f154f6e6ec10f178baf527ce78a873d030c18e8362419b79751995bec67ed918
	RegistryTypeHarborArtifact = "/api/v2.0/projects/%s/repositories/%s/artifacts/%s"
)

type CreateRegistryRequest struct {
	Id       int32  `json:"id"`
	Name     string `json:"name"`
	Url      string `json:"url"`
	UserName string `json:"username"`
	Password string `json:"password"`
	Default  bool   `json:"default"`
}

// Validate checks if the search client configuration is valid.
func (r *CreateRegistryRequest) Validate(create bool) error {
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if r.Url == "" {
		return fmt.Errorf("url is required")
	}
	if r.UserName == "" {
		return fmt.Errorf("token is required")
	}
	if create && r.Password == "" {
		return fmt.Errorf("password is required")

	}
	return nil
}

type UpdateRegistryRequest struct {
	CreateImageRequest
	Id int32 `json:"id"`
}

type RegistryView struct {
	Id        int32  `json:"id"`
	Name      string `json:"name"`
	Url       string `json:"url"`
	UserName  string `json:"user_name"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type Pagination struct {
	PageNum  int `json:"page_num" form:"pageNum,default=1"`
	PageSize int `json:"page_size" form:"pageSize,default=10"`
}

type RegistryAccountInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Url      string `json:"url"`
}

type RegistryAuth struct {
	Auths map[string]RegistryAuthItem `json:"auths"`
}

type RegistryAuthItem struct {
	Auth string `json:"auth"`
}

type ImportDetailResponse struct {
	LayersDetail map[string]interface{} `json:"layersDetail"`
}

type ImageRegistryInfo struct {
	Id        int32  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	Default   bool   `json:"default"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type ListImageFlatResponse struct {
	TotalCount int     `json:"totalCount"`
	Items      []Image `json:"images,omitempty"`
}

type Image struct {
	Id          int32  `json:"id"`
	Tag         string `json:"tag"`
	Description string `json:"description,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	CreatedBy   string `json:"created_by"`
	SecretId    string `json:"secretId,omitempty"`
}

// ExportedImageListResponse represents the response for listing exported images.
type ExportedImageListResponse struct {
	TotalCount int                     `json:"totalCount"`
	Items      []ExportedImageListItem `json:"items"`
}

// ExportedImageListItem represents a single exported image record in the list.
type ExportedImageListItem struct {
	JobId       string `json:"jobId"`       // OpsJob ID for deletion and other operations
	ImageName   string `json:"imageName"`   // Target image name, e.g. custom/library/busybox:20251113
	Workload    string `json:"workload"`    // Source workload ID from inputs
	Status      string `json:"status"`      // Export status: Pending/Failed/Succeeded/Running
	CreatedTime string `json:"createdTime"` // Export creation time (RFC3339)
	Label       string `json:"label"`       // User-defined label from inputs
	Log         string `json:"log"`         // Message from conditions field
}

// PrewarmImageListResponse represents the response for listing prewarm images.
type PrewarmImageListResponse struct {
	TotalCount int                    `json:"totalCount"`
	Items      []PrewarmImageListItem `json:"items"`
}

// PrewarmImageListItem represents a single prewarm image record in the list.
type PrewarmImageListItem struct {
	ImageName       string `json:"imageName"`
	WorkspaceId     string `json:"workspaceId"`
	WorkspaceName   string `json:"workspaceName"`
	Status          string `json:"status"`
	PrewarmProgress string `json:"prewarmProgress"`
	CreatedTime     string `json:"createdTime"`
	EndTime         string `json:"endTime"`
	UserName        string `json:"userName"`
	ErrorMessage    string `json:"errorMessage"`
}
