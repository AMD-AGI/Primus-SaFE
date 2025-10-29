package model

type ClusterOverviewHeatmapItem struct {
	NodeName string  `json:"node_name"`
	GpuId    int     `json:"gpu_id"`
	Value    float64 `json:"value"`
}

type Heatmap struct {
	Serial   int     `json:"serial"`
	Unit     string  `json:"unit"`
	YAxisMax float64 `json:"yaxis_max"`
	YAxisMin float64 `json:"yaxis_min"`
	Data     []ClusterOverviewHeatmapItem
}
