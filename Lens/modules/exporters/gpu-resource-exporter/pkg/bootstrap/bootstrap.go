package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gpu-resource-exporter/pkg/listener"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gpu-resource-exporter/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

func Init(ctx context.Context, cfg *config.Config) error {
	if err := RegisterController(ctx); err != nil {
		return err
	}
	err := listener.InitManager(ctx)
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
	controller.RegisterReconciler(reconciler.NewGpuPodsReconciler())
	controller.RegisterReconciler(reconciler.NewNodeReconciler())
	// Register Service and Endpoints reconcilers for gateway-exporter support
	controller.RegisterReconciler(reconciler.NewServiceReconciler())
	controller.RegisterReconciler(reconciler.NewEndpointsReconciler())
	return nil
}
