package bootstrap

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/controller"
	"github.com/AMD-AGI/primus-lens/core/pkg/server"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/exporter"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/policy"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var handler *exporter.Handler

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

func Init(ctx context.Context, conf *config.Config) error {
	err := controller.RegisterScheme(schemes)
	if err != nil {
		return err
	}
	clientSets := clientsets.GetCurrentClusterK8SClientSet()
	err = policy.LoadDefaultPolicy(ctx, clientSets.ControllerRuntimeClient)
	if err != nil {
		return err
	}
	handler, err = exporter.InitNetHandler(conf)
	if err != nil {
		return err
	}
	err = handler.Init(ctx, conf)
	if err != nil {
		return err
	}
	server.SetDefaultGather(handler)
	return nil
}
