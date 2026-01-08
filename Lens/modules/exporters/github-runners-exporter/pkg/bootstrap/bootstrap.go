package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/reconciler"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var schemes = &runtime.SchemeBuilder{}

// schemeAdder adds the AutoScalingRunnerSet and EphemeralRunner types to the scheme
func schemeAdder(scheme *runtime.Scheme) error {
	// Register the unstructured types for GitHub Actions Runner Controller CRDs
	scheme.AddKnownTypeWithName(
		types.AutoScalingRunnerSetGVK,
		&runtime.Unknown{},
	)
	scheme.AddKnownTypeWithName(
		types.EphemeralRunnerGVK,
		&runtime.Unknown{},
	)

	// Add GroupVersion to registry
	metaGV := schema.GroupVersion{Group: "actions.github.com", Version: "v1alpha1"}
	scheme.AddKnownTypes(metaGV)

	return nil
}

func init() {
	schemes.Register(schemeAdder)
}

// Init initializes the github-runners-exporter
func Init(ctx context.Context, cfg *config.Config) error {
	if err := RegisterController(ctx); err != nil {
		return err
	}
	log.Info("GitHub Runners Exporter initialized successfully")
	return nil
}

// RegisterController registers the reconcilers with the controller manager
func RegisterController(ctx context.Context) error {
	if err := controller.RegisterScheme(schemes); err != nil {
		return err
	}

	// Register AutoScalingRunnerSet reconciler
	arsReconciler := reconciler.NewAutoScalingRunnerSetReconciler()
	if err := arsReconciler.Init(ctx); err != nil {
		log.Errorf("Failed to initialize AutoScalingRunnerSetReconciler: %v", err)
		return err
	}
	controller.RegisterReconciler(arsReconciler)
	log.Info("AutoScalingRunnerSetReconciler registered")

	// Register EphemeralRunner reconciler
	erReconciler := reconciler.NewEphemeralRunnerReconciler()
	if err := erReconciler.Init(ctx); err != nil {
		log.Errorf("Failed to initialize EphemeralRunnerReconciler: %v", err)
		return err
	}
	controller.RegisterReconciler(erReconciler)
	log.Info("EphemeralRunnerReconciler registered")

	return nil
}

