package bootstrap

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/controller"
	"github.com/AMD-AGI/primus-lens/core/pkg/router"
	"github.com/AMD-AGI/primus-lens/core/pkg/server"
	"github.com/AMD-AGI/primus-lens/node-exporter/pkg/api"
	"github.com/AMD-AGI/primus-lens/node-exporter/pkg/collector"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

func Bootstrap(ctx context.Context) error {
	return server.InitServerWithPreInitFunc(ctx, func(ctx context.Context, cfg *config.Config) error {
		err := controller.RegisterScheme(schemes)
		if err != nil {
			return err
		}
		if err := collector.Init(ctx, *cfg); err != nil {
			return err
		}

		router.RegisterGroup(api.RegisterRouter)
		collector.Start(ctx)
		return nil
	})
}
