package model

import (
	"github.com/AMD-AGI/primus-lens/core/pkg/database/filter"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"strings"
)

type GPUNode struct {
	Name           string  `json:"name"`
	Ip             string  `json:"ip"`
	GpuName        string  `json:"gpu_name"`
	GpuCount       int     `json:"gpu_count"`
	GpuAllocation  int     `json:"gpu_allocation"`
	GpuUtilization float64 `json:"gpu_utilization"`
	Status         string  `json:"status"`
	StatusColor    string  `json:"status_color"`
}

type SearchGpuNodeReq struct {
	rest.Page
	Name    string `form:"name"`
	GpuName string `form:"gpu_name"`
	Status  string `form:"status"`
	OrderBy string `form:"order_by"`
	Desc    bool   `form:"desc"`
}

func (s SearchGpuNodeReq) ToNodeFilter() filter.NodeFilter {
	result := filter.NodeFilter{}
	result.Offset = (s.PageNum - 1) * s.PageSize
	result.Limit = s.PageSize
	if s.Desc {
		result.Order = "desc"
	} else {
		result.Order = "asc"
	}
	result.OrderBy = s.OrderBy
	if s.Name != "" {
		result.Name = &s.Name
	}
	if s.GpuName != "" {
		result.GPUName = &s.GpuName
	}
	if s.Status != "" {
		result.Status = strings.Split(s.Status, ",")
	}
	return result
}

type GpuNodeDetail struct {
	Name              string `json:"name"`
	Health            string `json:"health"`
	Cpu               string `json:"cpu"`
	Memory            string `json:"memory"`
	OS                string `json:"os"`
	GPUDriverVersion  string `json:"gpu_driver_version"`
	StaticGpuDetails  string `json:"static_gpu_details"`
	KubeletVersion    string `json:"kubelet_version"`
	ContainerdVersion string `json:"containerd_version"`
}
