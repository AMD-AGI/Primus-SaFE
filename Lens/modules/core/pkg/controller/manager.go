package controller

import (
	"context"
	"reflect"

	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	logConf "github.com/AMD-AGI/primus-lens/core/pkg/logger/conf"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	controllerLogger "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var scheme = runtime.NewScheme()

type Reconciler interface {
	SetupWithManager(mgr ctrl.Manager) error
	Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error)
}

var reconcilers = []Reconciler{}

func RegisterReconciler(r Reconciler) {
	reconcilers = append(reconcilers, r)
}

func RegisterScheme(schemeBuilder *runtime.SchemeBuilder) error {
	err := schemeBuilder.AddToScheme(scheme)
	if err != nil {
		return errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithError(err).
			WithMessage("Failed to add to scheme")
	}
	return nil
}

func GetScheme() *runtime.Scheme {
	return scheme
}

func InitControllers(ctx context.Context, conf config.Config) error {
	if len(reconcilers) == 0 {
		log.Infof("No controllers registered.Skip controller initializtion")
		return nil
	}
	controllerLogger.SetLogger(logr.New(log.NewLogger(logConf.ErrorLevel)))
	cfg := conf.Controller
	k8sCfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(k8sCfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.GetMetricsBindAddress(),
		},
		HealthProbeBindAddress:  cfg.GetHealthzBindAddress(),
		PprofBindAddress:        cfg.GetPprofBindAddress(),
		ReadinessEndpointName:   "/ready",
		LivenessEndpointName:    "/health",
		LeaderElection:          true,
		LeaderElectionNamespace: cfg.Namespace,
		LeaderElectionID:        cfg.LeaderElectionId,
	})
	if err != nil {
		return err
	}
	for _, reconciler := range reconcilers {
		err := reconciler.SetupWithManager(mgr)
		if err != nil {
			log.Errorf("Failed to setup %s controller: %v", reflect.TypeOf(reconciler).Name(), err)
			return err
		}
	}
	go func() {
		err := mgr.Start(ctx)
		if err != nil {
			panic(err)
		}
	}()
	return err
}
