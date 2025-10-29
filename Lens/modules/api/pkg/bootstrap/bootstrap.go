package bootstrap

import (
	"context"
	"github.com/AMD-AGI/primus-lens/api/pkg/api"
	"github.com/AMD-AGI/primus-lens/core/pkg/controller"
	"github.com/AMD-AGI/primus-lens/core/pkg/router"
	"github.com/AMD-AGI/primus-lens/core/pkg/server"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

func StartServer(ctx context.Context) error {
	err := RegisterApi(ctx)
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
