package bootstrap

import (
	"context"
	"errors"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/exporter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/jobs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

func Init(ctx context.Context, cfg *config.Config) error {
	if cfg.Jobs == nil {
		return errors.New("jobs config is required")
	}
	
	// 启用 Jaeger tracer
	err := trace.InitTracer("primus-lens-jobs")
	if err != nil {
		log.Errorf("Failed to init tracer: %v", err)
		// 不阻断启动，降级为不追踪
	} else {
		log.Info("Jaeger tracer initialized successfully for jobs service")
	}
	
	// 注册 cleanup 函数
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
	}()
	
	err = exporter.StartServer(ctx, cfg.Jobs.GrpcPort)
	if err != nil {
		return err
	}
	err = controller.RegisterScheme(schemes)
	if err != nil {
		return err
	}
	err = jobs.Start(ctx)
	if err != nil {
		return err
	}
	return nil
}
