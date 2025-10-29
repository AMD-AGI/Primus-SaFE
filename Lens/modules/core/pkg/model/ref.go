package model

type PodResourceRef struct {
	PodName      string          `json:"pod_name"`
	PodUid       string          `json:"pod_uid"`
	PodNamespace string          `json:"pod_namespace"`
	Gpu          []GpuRef        `json:"gpu"`
	Rdma         []RdmaDeviceRef `json:"rdma"`
}

type GpuRef struct {
	GpuId   int    `json:"gpu_id"`
	GpuGuid string `json:"gpu_guid"`
}

type RdmaDeviceRef struct {
	Guid string `json:"guid"`
	Port int    `json:"port"`
	Lid  int    `json:"lid"`
}
