// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"encoding/json"
	"log/slog"
	"strconv"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
	"github.com/prometheus/client_golang/prometheus"
)

type QPCollector struct {
	executor *CommandExecutor
	nodeName string
	qpInfo   *prometheus.GaugeVec
	qpCount  *prometheus.GaugeVec
}

func NewQPCollector(executor *CommandExecutor, nodeName string) *QPCollector {
	qpInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rdma_qp_info",
			Help: "One per active queue pair (value is always 1)",
		},
		[]string{"node", "device", "port", "lqpn", "rqpn", "type", "state", "comm"},
	)
	qpCount := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rdma_qp_count",
			Help: "Number of queue pairs per device, type, and state",
		},
		[]string{"node", "device", "type", "state"},
	)
	prometheus.MustRegister(qpInfo, qpCount)
	return &QPCollector{
		executor: executor,
		nodeName: nodeName,
		qpInfo:   qpInfo,
		qpCount:  qpCount,
	}
}

func (c *QPCollector) Collect() {
	out, err := c.executor.Execute("rdma", "res", "show", "qp", "-j")
	if err != nil {
		slog.Error("rdma res show qp", "error", err)
		return
	}
	var qps []model.RDMAQP
	if err := json.Unmarshal(out, &qps); err != nil {
		slog.Error("parse rdma qp json", "error", err)
		return
	}

	c.qpInfo.Reset()
	c.qpCount.Reset()

	for _, qp := range qps {
		if qp.Type == "GSI" {
			continue
		}
		device := qp.IfName
		portStr := strconv.Itoa(qp.Port)
		lqpn := strconv.Itoa(qp.LQPN)
		rqpn := strconv.Itoa(qp.RQPN)
		comm := qp.Comm

		c.qpInfo.WithLabelValues(
			c.nodeName, device, portStr, lqpn, rqpn, qp.Type, qp.State, comm,
		).Set(1)

		c.qpCount.WithLabelValues(c.nodeName, device, qp.Type, qp.State).Add(1)
	}
}
