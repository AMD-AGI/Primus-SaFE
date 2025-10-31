package model

type GpuAllocation struct {
	Node           string  `json:"node"`
	Vendor         string  `json:"vendor"`
	Capacity       int     `json:"capacity"`
	Allocated      int     `json:"allocated"`
	AllocationRate float64 `json:"allocationRate"`
}

type GPUUtilization struct {
	AllocationRate float64 `json:"allocationRate"`
	Utilization    float64 `json:"utilization"`
}

type GpuUtilizationHistory struct {
	AllocationRate  []TimePoint `json:"allocationRate"`
	Utilization     []TimePoint `json:"utilization"`
	VramUtilization []TimePoint `json:"vramUtilization"`
}

type GpuClusterOverview struct {
	StorageStat
	RdmaClusterStat
	TotalNodes         int     `json:"totalNodes"`
	HealthyNodes       int     `json:"healthyNodes"`
	FaultyNodes        int     `json:"faultyNodes"`
	FullyIdleNodes     int     `json:"fullyIdleNodes"`
	PartiallyIdleNodes int     `json:"partiallyIdleNodes"`
	BusyNodes          int     `json:"busyNodes"`
	AllocationRate     float64 `json:"allocationRate"`
	Utilization        float64 `json:"utilization"`
}

type GpuDeviceInfo struct {
	DeviceId    int     `json:"deviceId"`
	Model       string  `json:"model"`
	Memory      string  `json:"memory"`
	Utilization float64 `json:"utilization"`
	Temperature float64 `json:"temperature"`
	Power       float64 `json:"power"`
}
