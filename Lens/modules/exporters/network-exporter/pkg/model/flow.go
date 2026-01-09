// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"strconv"
	"time"
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

type TcpConnReport struct {
	Direction uint8              `json:"direction"`
	Node      string             `json:"node"`
	Ingress   *TcpConnDownstream `json:"ingress"`
	Egress    *TcpConnUpstream   `json:"egress"`
	Duration  int32              `json:"duration"`
	TimeStamp time.Time          `json:"timestamp"`
}

type TcpConnUpstream struct {
	Addr       string `json:"addr"`
	Port       int32  `json:"port"`
	Family     uint16 `json:"family"`
	ConnCount  int32  `json:"conn_count"`
	CloseCount int32  `json:"close_count"`
}

func (t TcpConnUpstream) String() string {
	return t.Addr + "-" + string(t.Port) + "-" + strconv.Itoa(int(t.Family))
}

type TcpConnDownstream struct {
	LocalPort  int32  `json:"local_port"`
	RemoteAddr string `json:"remote_addr"`
	Family     uint16 `json:"family"`
	ConnCount  int32  `json:"conn_count"`
	CloseCount int32  `json:"close_count"`
}

func (d TcpConnDownstream) String() string {
	return string(d.LocalPort) + "-" + d.RemoteAddr + "-" + strconv.Itoa(int(d.Family))
}

type TcpFlowEvent struct {
	TcpFlowCacheKey
	DataLen uint64 `json:"data_len"`
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
