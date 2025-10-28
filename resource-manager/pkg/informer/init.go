package informer

import (
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/informers/externalversions"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func InitInformer(cfg *rest.Config, controllerRuntimeClient client.Client) error {
	versionedClient, err := versioned.NewForConfig(cfg)
	if err != nil {
		return err
	}
	workloadInformer := NewWorkloadInformer(controllerRuntimeClient)
	factory := externalversions.NewSharedInformerFactory(versionedClient, 0)
	err = workloadInformer.Register(factory)
	if err != nil {
		return err
	}
	return nil
}
