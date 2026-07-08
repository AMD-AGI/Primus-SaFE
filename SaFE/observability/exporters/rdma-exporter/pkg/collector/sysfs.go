// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/rdma-exporter/pkg/model"
	"github.com/prometheus/client_golang/prometheus"
)

var keyHwCounters = map[string]struct{}{
	// bnxt_re / mlx5 common counters
	"rx_bytes": {}, "tx_bytes": {}, "rx_pkts": {}, "tx_pkts": {},
	"rx_roce_only_bytes": {}, "tx_roce_only_bytes": {}, "rx_roce_only_pkts": {}, "tx_roce_only_pkts": {},
	"rx_good_bytes": {}, "rx_good_pkts": {},
	"tx_write_req": {}, "rx_write_req": {}, "tx_read_req": {}, "rx_read_req": {},
	"tx_send_req": {}, "rx_send_req": {}, "tx_atomic_req": {}, "rx_atomic_req": {}, "tx_read_resp": {},
	"active_qps": {}, "active_rc_qps": {}, "active_ud_qps": {},
	"to_retransmits": {}, "seq_err_naks_rcvd": {}, "max_retry_exceeded": {}, "rnr_naks_rcvd": {},
	"tx_cnp_pkts": {}, "rx_cnp_pkts": {}, "rx_ecn_marked_pkts": {}, "oos_drop_count": {},
	"recoverable_errors": {}, "unrecoverable_err": {},
	"rx_roce_errors": {}, "tx_roce_errors": {}, "rx_roce_discards": {}, "tx_roce_discards": {},

	// ionic RDMA traffic counters
	"rx_rdma_ucast_bytes": {}, "tx_rdma_ucast_bytes": {},
	"rx_rdma_ucast_pkts": {}, "tx_rdma_ucast_pkts": {},
	"rx_rdma_mcast_bytes": {}, "tx_rdma_mcast_bytes": {},
	"rx_rdma_mcast_pkts": {}, "tx_rdma_mcast_pkts": {},
	"rx_rdma_cnp_pkts": {}, "tx_rdma_cnp_pkts": {},
	"rx_rdma_ecn_pkts": {},
	"rx_rdma_ccl_cts_bytes": {}, "tx_rdma_ccl_cts_bytes": {},
	"rx_rdma_ccl_cts_pkts": {}, "tx_rdma_ccl_cts_pkts": {},
	"rx_rdma_mtu_discard_pkts": {},
	"tx_rdma_retx_bytes": {}, "tx_rdma_retx_pkts": {},
	"tx_rdma_ccl_cts_retx_bytes": {}, "tx_rdma_ccl_cts_retx_pkts": {},
	"tx_rdma_ack_timeout": {}, "tx_rdma_ccl_cts_ack_timeout": {},

	// ionic RDMA error counters
	"req_rx_cqe_err": {}, "req_rx_cqe_flush": {},
	"req_rx_dup_response": {}, "req_rx_impl_nak_seq_err": {},
	"req_rx_inval_pkts": {}, "req_rx_oper_err": {},
	"req_rx_pkt_seq_err": {}, "req_rx_rmt_acc_err": {},
	"req_rx_rmt_req_err": {}, "req_rx_rnr_retry_err": {},
	"req_tx_loc_acc_err": {}, "req_tx_loc_oper_err": {},
	"req_tx_loc_sgl_inv_err": {}, "req_tx_mem_mgmt_err": {},
	"req_tx_retry_excd_err": {},
	"resp_rx_ccl_cts_outouf_seq": {}, "resp_rx_cqe_err": {},
	"resp_rx_cqe_flush": {}, "resp_rx_dup_request": {},
	"resp_rx_inval_request": {}, "resp_rx_loc_len_err": {},
	"resp_rx_loc_oper_err": {}, "resp_rx_outof_atomic": {},
	"resp_rx_outof_buf": {}, "resp_rx_outouf_seq": {},
	"resp_rx_s0_table_err": {}, "resp_tx_loc_sgl_inv_err": {},
	"resp_tx_pkt_seq_err": {}, "resp_tx_rmt_acc_err": {},
	"resp_tx_rmt_inval_req_err": {}, "resp_tx_rmt_oper_err": {},
	"resp_tx_rnr_retry_err": {},
}

type SysfsCollector struct {
	basePath  string
	nodeName  string
	metrics   map[string]*prometheus.GaugeVec
	metricsMu sync.Mutex
}

func NewSysfsCollector(nodeName string) *SysfsCollector {
	return &SysfsCollector{
		basePath: "/sys/class/infiniband",
		nodeName: nodeName,
		metrics:  make(map[string]*prometheus.GaugeVec),
	}
}

func (c *SysfsCollector) Collect(devices []model.RDMADevice, addr map[string]string) {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	for _, dev := range devices {
		devAddr := addr[dev.IfName]
		portsDir := filepath.Join(c.basePath, dev.IfName, "ports")
		entries, err := os.ReadDir(portsDir)
		if err != nil {
			slog.Debug("skip device ports", "device", dev.IfName, "error", err)
			continue
		}
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			port := ent.Name()
			hwDir := filepath.Join(portsDir, port, "hw_counters")
			counterFiles, err := os.ReadDir(hwDir)
			if err != nil {
				slog.Debug("skip hw_counters", "path", hwDir, "error", err)
				continue
			}
			for _, cf := range counterFiles {
				if cf.IsDir() {
					continue
				}
				name := cf.Name()
				if _, ok := keyHwCounters[name]; !ok {
					continue
				}
				data, err := os.ReadFile(filepath.Join(hwDir, name))
				if err != nil {
					slog.Debug("read counter", "path", filepath.Join(hwDir, name), "error", err)
					continue
				}
				val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
				if err != nil {
					slog.Debug("parse counter", "name", name, "error", err)
					continue
				}
				if val < 0 {
					val = int64(uint32(val))
				}
				metricName := "rdma_hw_" + sanitizeMetricName(name)
				gauge, exists := c.metrics[metricName]
				if !exists {
					gauge = prometheus.NewGaugeVec(
						prometheus.GaugeOpts{
							Name: metricName,
							Help: "RDMA hardware counter: " + name,
						},
						[]string{"node", "device", "address", "port"},
					)
					if err := prometheus.Register(gauge); err != nil {
						if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
							gauge = are.ExistingCollector.(*prometheus.GaugeVec)
						} else {
							slog.Warn("register metric", "name", metricName, "error", err)
							continue
						}
					}
					c.metrics[metricName] = gauge
				}
				gauge.WithLabelValues(c.nodeName, dev.IfName, devAddr, port).Set(float64(val))
			}
		}
	}
}
