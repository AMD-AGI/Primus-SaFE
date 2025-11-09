package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
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
	// 启用 Jaeger tracer
	err := trace.InitTracer("primus-lens-api")
	if err != nil {
		log.Errorf("Failed to init tracer: %v", err)
		// 不阻断启动，降级为不追踪
	} else {
		log.Info("Jaeger tracer initialized successfully")
	}

	// 注册 cleanup 函数
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
	}()

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
