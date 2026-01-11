// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/auth"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	cpauth "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/conf"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

func StartServer(ctx context.Context) error {
	// Enable OpenTelemetry tracer
	err := trace.InitTracer("primus-lens-api")
	if err != nil {
		log.Errorf("Failed to init OpenTelemetry tracer: %v", err)
		// Don't block startup, degrade to no tracing
	} else {
		log.Info("OpenTelemetry tracer initialized successfully")

		// Send a test span to verify the trace pipeline
		log.Info("Sending test span to verify trace export...")
		testCtx, testSpan := trace.StartSpan(ctx, "primus-lens-api.startup.test")
		traceID := trace.GetTraceID(testCtx)
		spanID := trace.GetSpanID(testCtx)
		log.Infof("Test span created - TraceID=%s, SpanID=%s", traceID, spanID)
		trace.SetAttribute(testCtx, "test.type", "startup_verification")
		trace.SetAttribute(testCtx, "test.timestamp", "startup")
		trace.AddEvent(testCtx, "API server starting up")
		testSpan.End()
		log.Infof("Test span ended - TraceID=%s, SpanID=%s", traceID, spanID)
		log.Info("⚠️ Note: Spans are batched and sent every 5 seconds. The test span will be exported shortly.")
	}

	// Register cleanup function
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
	}()

	logConf := conf.DefaultConfig()
	logConf.Level = conf.TraceLevel
	log.InitGlobalLogger(logConf)
	err = RegisterApi(ctx)
	if err != nil {
		return err
	}
	// Use preInit callback to initialize auth system after ClusterManager is ready
	return server.InitServerWithPreInitFunc(ctx, preInitAuthSystem)
}

func RegisterApi(ctx context.Context) error {
	err := controller.RegisterScheme(schemes)
	if err != nil {
		return err
	}

	router.RegisterGroup(api.RegisterRouter)
	return nil
}

// preInitAuthSystem is called after ClusterManager is initialized
// This is the preInit callback for InitServerWithPreInitFunc
func preInitAuthSystem(ctx context.Context, cfg *config.Config) error {
	log.Info("Initializing authentication system...")

	// Check if Control Plane is enabled
	cm := clientsets.GetClusterManager()
	if cm == nil {
		log.Warn("ClusterManager not available, skipping auth system initialization")
		return nil
	}

	// Check if Control Plane is enabled
	if !cm.IsControlPlaneEnabled() {
		log.Info("Control Plane not enabled, skipping auth system initialization")
		log.Info("To enable authentication features, set controlPlane.enabled=true in config")
		return nil
	}

	// Get K8s client from cluster manager
	var k8sClient client.Client
	if cc := cm.GetCurrentClusterClients(); cc != nil && cc.K8SClientSet != nil {
		k8sClient = cc.K8SClientSet.ControllerRuntimeClient
	}
	if k8sClient == nil {
		log.Error("K8s client not available, cannot initialize auth system")
		return nil
	}

	// Create SafeDetector and Initializer
	safeDetector := cpauth.NewSafeDetector(k8sClient)
	initializer := cpauth.NewInitializer(safeDetector, k8sClient)

	// Initialize auth handlers with dependencies
	auth.InitializeAuthHandlers(initializer, safeDetector)
	log.Info("Auth handlers initialized")

	// Ensure system is initialized (creates root user if not exists)
	if err := initializer.EnsureInitialized(ctx); err != nil {
		log.Errorf("Failed to ensure system initialized: %v", err)
		// Don't block startup, auth features may be limited
		return nil
	}

	log.Info("Authentication system initialized successfully")
	return nil
}
