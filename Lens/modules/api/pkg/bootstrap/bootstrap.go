package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/conf"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	router.RegisterGroup(api.RegisterRouter)
	return nil
}
