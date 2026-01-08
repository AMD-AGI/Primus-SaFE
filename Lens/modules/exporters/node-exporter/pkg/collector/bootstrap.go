package collector

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/containerd"
	k8s_ephemeral_storage "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/k8s-ephemeral-storage"
	processtree "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/process-tree"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/pyspy"
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

	// Initialize Process Tree Collector
	if err := processtree.InitCollector(ctx); err != nil {
		log.Warnf("Failed to initialize Process Tree Collector: %v", err)
		// Don't block startup, collector is an optional feature
	}

	// Initialize Py-Spy Collector
	pyspyConfig := cfg.NodeExporter.GetPySpyConfig()
	if pyspyConfig.Enabled {
		if err := pyspy.InitCollector(ctx, pyspyConfig); err != nil {
			log.Warnf("Failed to initialize Py-Spy Collector: %v", err)
			// Don't block startup, collector is an optional feature
		}
	} else {
		log.Info("Py-Spy Collector is disabled")
	}

	return nil
}

func Start(ctx context.Context) {
	startRefreshGPUInfo(ctx)

	// Set GPU info provider for process-tree module
	processtree.SetGPUInfoProvider(&gpuInfoProviderImpl{})

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

// gpuInfoProviderImpl implements processtree.GPUInfoProvider
type gpuInfoProviderImpl struct{}

func (g *gpuInfoProviderImpl) GetDriCardInfoMapping() map[string]model.DriDevice {
	return cardDriDeviceMapping
}

func (g *gpuInfoProviderImpl) GetGpuDeviceInfo() []model.GPUInfo {
	return gpuDeviceInfo
}
