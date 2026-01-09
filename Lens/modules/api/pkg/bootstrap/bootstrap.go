// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/auth"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
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
	return server.InitServer(ctx)
}

func RegisterApi(ctx context.Context) error {
	err := controller.RegisterScheme(schemes)
	if err != nil {
		return err
	}

	// Initialize auth system
	if err := initializeAuthSystem(ctx); err != nil {
		log.Errorf("Failed to initialize auth system: %v", err)
		// Don't block startup, auth features will be disabled
	}

	router.RegisterGroup(api.RegisterRouter)
	return nil
}

// initializeAuthSystem initializes the authentication system and ensures root user exists
func initializeAuthSystem(ctx context.Context) error {
	log.Info("Initializing authentication system...")

	// Try to get K8s client from cluster manager
	var k8sClient client.Client
	cm := clientsets.GetClusterManager()
	if cm != nil {
		if cc := cm.GetCurrentClusterClients(); cc != nil && cc.K8SClientSet != nil {
			k8sClient = cc.K8SClientSet.ControllerRuntimeClient
			if k8sClient != nil {
				log.Info("K8s client available, will store root password in Secret")
			}
		}
	}

	// Create SafeDetector
	var safeDetector *cpauth.SafeDetector
	if k8sClient != nil {
		safeDetector = cpauth.NewSafeDetector(k8sClient)
	} else {
		safeDetector = cpauth.NewSafeDetectorWithoutK8s()
		log.Warn("K8s client not available, SafeDetector will have limited functionality")
	}

	// Create Initializer with K8s client
	var initializer *cpauth.Initializer
	if k8sClient != nil {
		initializer = cpauth.NewInitializerWithK8s(safeDetector, k8sClient)
	} else {
		initializer = cpauth.NewInitializer(safeDetector)
	}

	// Initialize auth handlers with dependencies
	auth.InitializeAuthHandlers(initializer, safeDetector)
	log.Info("Auth handlers initialized")

	// Ensure system is initialized (creates root user if not exists)
	if err := initializer.EnsureInitialized(ctx); err != nil {
		return err
	}

	log.Info("Authentication system initialized successfully")
	return nil
}
