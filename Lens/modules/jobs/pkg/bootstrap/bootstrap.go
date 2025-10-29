package bootstrap

import (
	"context"
	"errors"

	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/controller"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/exporter"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/jobs"
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
	err := exporter.StartServer(ctx, cfg.Jobs.GrpcPort)
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
