// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"strconv"
)

const (
	FlowTypeIngress = 0
	FlowTypeEgress  = 1

	FlowTypeNameIngress = "ingress"
	FlowTypeNameEgress  = "egress"

	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

func GetDirectionName(direction int) string {
	if direction == FlowTypeIngress {
		return FlowTypeNameIngress
	}
	return FlowTypeNameEgress
}

type TcpFlowCacheKey struct {
	SAddr  string `json:"saddr"`
	Daddr  string `json:"daddr"`
	Sport  int    `json:"sport"`
	Dport  int    `json:"dport"`
	Family int    `json:"family"`
}

func (t TcpFlowCacheKey) String() string {
	return t.SAddr + "-" + t.Daddr + "-" + strconv.Itoa(t.Sport) + "-" + strconv.Itoa(t.Dport) + "-" + strconv.Itoa(t.Family)
}

type TcpFlowDataValue struct {
	RttTotal  uint64 `json:"rtt_total"`
	PktCount  uint64 `json:"pkt_count"`
	FlowData  uint64 `json:"flow_data"`
	ConnCount uint64 `json:"conn_count"`
}
