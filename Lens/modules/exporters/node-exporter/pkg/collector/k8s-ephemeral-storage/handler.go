// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package k8s_ephemeral_storage

import (
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/goroutineUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/kubelet"
	statsapi "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
	"time"
)

var (
	singleton *Handler
)

func InitHandler() (*Handler, error) {
	var err error
	if singleton == nil {
		singleton, err = newHandler()
		if err != nil {
			return nil, err
		}
	}
	return singleton, nil
}

func newHandler() (*Handler, error) {
	h := &Handler{
		lg: log.GlobalLogger().WithField("module", "k8s-ephemeral-storage"),
	}
	return h, nil
}

type Handler struct {
	lg logger.Logger
}

func (h *Handler) Init(ctx context.Context) error {
	goroutineUtil.RunGoroutineWithLog(func() {
		h.runReadEphemeralStorageMetrics(ctx, 10*time.Second)
	})
	return nil
}

func (h *Handler) runReadEphemeralStorageMetrics(ctx context.Context, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			h.lg.Infof("read ephemeral storage metrics")
			err := h.readEphemeralStorageMetrics(ctx)
			if err != nil {
				h.lg.Errorf("read ephemeral storage metrics failed %s", err)
			} else {
				h.lg.Infof("read ephemeral storage metrics success")
			}
			time.Sleep(interval)
		}
	}
}

func (h *Handler) readEphemeralStorageMetrics(ctx context.Context) error {
	stat := kubelet.GetKubeletClient().GetKubeletStats(ctx)
	if stat == nil {
		return nil
	}
	nodeName := kubelet.GetNodeName()
	h.getPodEphemeralStorageMetrics(ctx, nodeName, stat)
	h.getNodeEphemeralStorageMetrics(ctx, nodeName, stat)
	return nil
}

func (h *Handler) getPodEphemeralStorageMetrics(ctx context.Context, nodeName string, stat *statsapi.Summary) {
	for _, pod := range stat.Pods {
		if pod.EphemeralStorage != nil {
			usage := float64(0)
			if pod.EphemeralStorage.UsedBytes != nil {
				usage = float64(*pod.EphemeralStorage.UsedBytes)
				PodEphemeralStorageUsageBytes.Set(usage, pod.PodRef.Namespace, pod.PodRef.Name, nodeName)
			}
		}
	}
}

func (h *Handler) getNodeEphemeralStorageMetrics(ctx context.Context, nodeName string, stat *statsapi.Summary) {
	if stat.Node.Fs == nil {
		log.GlobalLogger()
		return
	}
	usage := float64(0)
	if stat.Node.Fs.UsedBytes != nil {
		usage = float64(*stat.Node.Fs.UsedBytes)
		NodeEphemeralStorageUsageBytes.Set(usage, nodeName)
	}
	available := float64(0)
	if stat.Node.Fs.AvailableBytes != nil {
		available = float64(*stat.Node.Fs.AvailableBytes)
		NodeEphemeralStorageAvailableBytes.Set(available, nodeName)
	}
	capacity := float64(0)
	if stat.Node.Fs.CapacityBytes != nil {
		capacity = float64(*stat.Node.Fs.CapacityBytes)
		NodeEphemeralStorageCapacityBytes.Set(capacity, nodeName)
	}
	if capacity != 0 && usage != 0 {
		percent := usage / capacity
		NodeEphemeralStorageUsagePercent.Set(percent, nodeName)
	}
}
