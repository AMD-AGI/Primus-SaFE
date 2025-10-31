package rdma

import (
	"fmt"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/prometheus/client_golang/prometheus"
	"os/exec"
	"strconv"
	"strings"
)

var (
	rdmaMetrics = map[string]*prometheus.GaugeVec{}
)

func getRDMAStatistics() ([]map[string]interface{}, error) {
	cmd := exec.Command("rdma", "statistic", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "link ")
	if len(lines) <= 1 {
		return nil, fmt.Errorf("unexpected output: %s", string(output))
	}

	var stats []map[string]interface{}

	for _, deviceData := range lines[1:] {
		parts := strings.Fields(deviceData)
		if len(parts) < 2 {
			continue
		}
		devicePort := parts[0]

		var device string
		var port string
		if strings.Contains(devicePort, "/") {
			dp := strings.SplitN(devicePort, "/", 2)
			device = dp[0]
			port = dp[1]
		} else {
			device = devicePort
			port = "unknown"
		}

		devStats := map[string]interface{}{
			"device": device,
			"port":   port,
			"stats":  map[string]int{},
		}

		statsMap := devStats["stats"].(map[string]int)

		kvPairs := parts[1:]
		for i := 0; i+1 < len(kvPairs); i += 2 {
			key := kvPairs[i]
			valStr := kvPairs[i+1]
			val, err := strconv.Atoi(valStr)
			if err != nil {
				continue
			}
			statsMap[key] = val
		}

		stats = append(stats, devStats)
	}

	return stats, nil
}

func GetMetrics() []*prometheus.GaugeVec {
	results := []*prometheus.GaugeVec{}
	for key := range rdmaMetrics {
		metric := rdmaMetrics[key]
		results = append(results, metric)
	}
	return results
}

func UpdateMetrics() {
	stats, err := getRDMAStatistics()
	if err != nil {
		log.Errorf("Error getting RDMA statistics: %v", err)
		return
	}

	for _, dev := range stats {
		device := dev["device"].(string)
		port := dev["port"].(string)
		statsMap := dev["stats"].(map[string]int)

		for key, value := range statsMap {
			metricName := fmt.Sprintf("rdma_stat_%s", key)
			if _, ok := rdmaMetrics[metricName]; !ok {
				rdmaMetrics[metricName] = prometheus.NewGaugeVec(
					prometheus.GaugeOpts{
						Name: metricName,
						Help: fmt.Sprintf("RDMA stat %s", key),
					},
					[]string{"device", "port"},
				)
				prometheus.MustRegister(rdmaMetrics[metricName])
			}
			rdmaMetrics[metricName].WithLabelValues(device, port).Set(float64(value))
		}
	}
}
