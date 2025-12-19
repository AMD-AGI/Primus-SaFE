package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/collector"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/collector/higress"
	gwconfig "github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/enricher"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/exporter"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	exp     *exporter.Exporter
	manager *collector.Manager
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
	networkingv1.AddToScheme,
}

// Init initializes the gateway exporter
func Init(ctx context.Context, conf *config.Config) error {
	// Register schemes
	if err := controller.RegisterScheme(schemes); err != nil {
		return err
	}

	// Load gateway exporter specific config
	gwConf, err := gwconfig.LoadGatewayExporterConfig()
	if err != nil {
		log.Warnf("Failed to load gateway exporter config, using defaults: %v", err)
		gwConf = &gwconfig.GatewayExporterConfig{}
	}

	// Get k8s client
	k8sClient := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet.ControllerRuntimeClient

	// Initialize collector manager
	manager = collector.NewManager(k8sClient)

	// Register collector factories
	manager.RegisterFactory(collector.GatewayTypeHigress, higress.NewHigressCollectorFactory())

	// Add collectors based on configuration
	for _, collectorConf := range gwConf.Gateway.Collectors {
		if !collectorConf.Enabled {
			continue
		}
		if err := manager.AddCollector(&collectorConf); err != nil {
			log.Warnf("Failed to add collector %s: %v", collectorConf.Type, err)
		}
	}

	// Initialize enricher (now database-based)
	enr := enricher.NewEnricher(
		nil, // k8sClient not needed anymore, using database
		gwConf.Gateway.GetCacheTTL(),
		gwConf.Enrichment.WorkloadLabels,
	)

	// Start enricher cache refresh loop
	go enr.StartCacheRefreshLoop(ctx)

	// Initialize exporter
	exp = exporter.NewExporter(manager, enr, gwConf)

	// Register metrics handler
	exp.Register()

	// Start collection loop
	go exp.StartCollectionLoop(ctx, gwConf.Gateway.GetScrapeInterval())

	// Set custom gatherer for metrics endpoint
	server.SetDefaultGather(exp)

	log.Info("Gateway exporter initialized successfully")
	return nil
}

