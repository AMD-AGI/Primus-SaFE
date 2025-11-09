package collector

import (
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/rdma"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
	"time"
)

var (
	rdmaDevices       []model.RDMADevice
	rdmaDeviceMapping map[string]model.RDMADevice
)

func GetRdmaDevices() []model.RDMADevice {
	return rdmaDevices
}

func GetRdmaMetrics() []*prometheus.GaugeVec {
	return rdma.GetMetrics()
}

func initRdmaMetricsCollector(ctx context.Context) {
	go func() {
		for {
			rdma.UpdateMetrics()
			time.Sleep(5 * time.Second)
		}
	}()
}

func doLoadRdmaDevices(ctx context.Context) {
	for {
		newDevices, err := rdma.GetRDMADevices()
		if err != nil {
			log.Errorf("Error getting rdma devices: %v", err)
		}
		newDeviceMapping := make(map[string]model.RDMADevice)
		for _, device := range newDevices {
			newDeviceMapping[strconv.Itoa(device.IfIndex)] = device
		}
		rdmaDevices = newDevices
		rdmaDeviceMapping = newDeviceMapping
		time.Sleep(60 * time.Second)
	}
}
