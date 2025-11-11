package collector

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/containerd"
	k8s_ephemeral_storage "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/k8s-ephemeral-storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/report"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/kubelet"
	"k8s.io/utils/env"
)

var (
	nodeName = ""
	nodeIp   = ""
)

func Init(ctx context.Context, cfg config.Config) error {
	err := containerd.Init(ctx, cfg.NodeExporter.ContainerdSocketPath)
	if err != nil {
		return err
	}
	nodeName = env.GetString("NODE_NAME", "default-node")
	nodeIp = env.GetString("NODE_IP", "0.0.0.0")
	err = kubelet.Init(ctx, nodeName)
	if err != nil {
		return err
	}

	// Initialize HTTP reporter for container events
	telemetryProcessorURL := cfg.NodeExporter.TelemetryProcessorURL
	if telemetryProcessorURL == "" {
		// Fallback to GrpcServer config for backward compatibility
		if cfg.NodeExporter.GrpcServer != "" {
			// Convert grpc://host:port to http://host:port format
			telemetryProcessorURL = "http://" + cfg.NodeExporter.GrpcServer
			log.Warnf("Using GrpcServer config as TelemetryProcessorURL: %s", telemetryProcessorURL)
		} else {
			log.Warnf("TelemetryProcessorURL not configured, using default")
			telemetryProcessorURL = "http://telemetry-processor:8989"
		}
	}

	err = report.InitHTTP(telemetryProcessorURL, nodeName, nodeIp)
	if err != nil {
		return err
	}

	err = TryInitDocker(ctx, "/hostrun/docker.sock")
	if err != nil {
		log.Errorf("init docker err: %v", err)
	}
	ephemeralStorageHandler, err := k8s_ephemeral_storage.InitHandler()
	if err != nil {
		return err
	}
	err = ephemeralStorageHandler.Init(ctx)
	if err != nil {
		return err
	}
	return nil
}

func Start(ctx context.Context) {
	startRefreshGPUInfo(ctx)
	go func() {
		runLoadGpuMetrics(ctx)
	}()
	go func() {
		runEventListener(ctx)
	}()
	initRdmaMetricsCollector(ctx)
	go func() {
		doLoadRdmaDevices(ctx)
	}()
}
