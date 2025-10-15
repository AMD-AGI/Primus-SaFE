package image_handlers

import (
	"fmt"
	"time"
)

type ImageServiceRequest struct {
	PageNum  int    `form:"page_num" binding:"omitempty,min=0" `
	PageSize int    `form:"page_size" binding:"omitempty,min=1"`
	OrderBy  string `form:"orderBy" binding:"omitempty"`
	Order    string `form:"order" binding:"omitempty,oneof=desc asc"`
	Tag      string `form:"tag" binding:"omitempty"`
	Ready    bool   `form:"ready"  binding:"omitempty"`
	UserName string `form:"userName" binding:"omitempty"`
	Flat     bool   `form:"flat" binding:"omitempty"`
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
	DestImageName   string `json:"destImageName"`   // full name, e.g: harbor.xcs.ai/01-ai/test/nginx:latest
	OsArch          string `json:"osArch"`          // e.g: linux/amd64
	Os              string `json:"os"`              // e.g: linux
	Arch            string `json:"arch"`            // e.g: amd64
	Description     string `json:"description"`     // 描述
}

type ImportImageServiceRequest struct {
	Source         string `json:"source"`
	SourceRegistry string `json:"sourceRegistry,omitempty"`
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
}
