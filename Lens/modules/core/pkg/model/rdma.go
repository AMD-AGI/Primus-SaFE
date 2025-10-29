package model

type RDMADevice struct {
	IfIndex      int    `json:"ifindex"`
	IfName       string `json:"ifname"`
	NodeType     string `json:"node_type"`
	FW           string `json:"fw"`
	NodeGUID     string `json:"node_guid"`
	SysImageGUID string `json:"sys_image_guid"`
}

type RdmaClusterStat struct {
	TotalTx float64 `json:"total_tx"`
	TotalRx float64 `json:"total_rx"`
}
