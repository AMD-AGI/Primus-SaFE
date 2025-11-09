package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/matcher"
	"github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/reconciler"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
	primusSafeV1.AddToScheme,
}

func Init(ctx context.Context, cfg *config.Config) error {
	// 启用 Jaeger tracer
	err := trace.InitTracer("primus-safe-adapter")
	if err != nil {
		log.Errorf("Failed to init tracer: %v", err)
		// 不阻断启动，降级为不追踪
	} else {
		log.Info("Jaeger tracer initialized successfully for adapter service")
	}

	// 注册 cleanup 函数
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
	}()

	if err := RegisterController(ctx); err != nil {
		return err
	}
	matcher.InitWorkloadMatcher(ctx)
	return nil
}

func RegisterController(ctx context.Context) error {
	err := controller.RegisterScheme(schemes)
	if err != nil {
		return err
	}
	workloadReconciler := &reconciler.WorkloadReconciler{}
	err = workloadReconciler.Init(ctx)
	if err != nil {
		return err
	}
	controller.RegisterReconciler(workloadReconciler)
	return nil
}
