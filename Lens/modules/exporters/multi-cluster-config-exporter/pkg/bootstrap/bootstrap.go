package bootstrap

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/controller"
	exporterController "github.com/AMD-AGI/primus-lens/multi-cluster-config-exporter/pkg/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

var listener *exporterController.MultiClusterStorageConfigListener

func Init(ctx context.Context, cfg *config.Config) error {
	if err := RegisterController(ctx); err != nil {
		return err
	}
	err := InitListener(ctx)
	if err != nil {
		return err
	}
	return nil
}

func RegisterController(ctx context.Context) error {
	err := controller.RegisterScheme(schemes)
	if err != nil {
		return err
	}
	return nil
}

func InitListener(ctx context.Context) error {
	listener = exporterController.NewMultiClusterStorageConfigListener(ctx)
	return listener.Start()
}
