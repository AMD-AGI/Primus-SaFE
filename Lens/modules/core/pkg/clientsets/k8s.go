package clientsets

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/controller"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type K8SClientSet struct {
	ControllerRuntimeClient client.Client
	Clientsets              *kubernetes.Clientset
	Dynamic                 *dynamic.DynamicClient
	Config                  *rest.Config
}

var (
	currentClusterClientset        *K8SClientSet
	multiClusterK8S                = map[string]*K8SClientSet{}
	multiClusterK8SConfigJsonBytes []byte
)

// getCurrentClusterK8SClientSet returns the K8S client for current cluster
// This is internal function, external code should use ClusterManager.GetCurrentClusterClients()
func getCurrentClusterK8SClientSet() *K8SClientSet {
	if currentClusterClientset == nil {
		panic("please init ControllerRuntimeClient clientSet first")
	}
	return currentClusterClientset
}

// getK8SClientSetByClusterName returns K8S client for a specific cluster
// This is internal function, external code should use ClusterManager.GetClientSetByClusterName()
func getK8SClientSetByClusterName(clusterName string) (*K8SClientSet, error) {
	k8sClientSet, exists := multiClusterK8S[clusterName]
	if !exists {
		return nil, errors.NewError().
			WithCode(errors.RequestDataNotExisted).
			WithMessagef("K8S client set for cluster %s not found", clusterName)
	}
	return k8sClientSet, nil
}

// getAllClusterK8SClients returns all cluster K8S clientsets
// This is internal function, external code should use ClusterManager.ListAllClientSets()
func getAllClusterK8SClients() map[string]*K8SClientSet {
	return multiClusterK8S
}

// initK8SClientSets is now handled by ClusterManager
// This function is kept for backward compatibility but should not be called directly
// Use InitClusterManager instead

func initCurrentClusterK8SClientSet(ctx context.Context) error {
	var err error
	k8sCfg := ctrl.GetConfigOrDie()
	currentClusterClientset, err = initK8SClientSetByConfig(k8sCfg)
	if err != nil {
		return err
	}
	return nil
}

func initK8SClientSetByConfig(k8sCfg *rest.Config) (*K8SClientSet, error) {
	clientSet := &K8SClientSet{}
	clientSet.Config = k8sCfg
	k8sClient, err := client.New(k8sCfg, client.Options{
		Scheme: controller.GetScheme(),
	})
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to initialize k8s client").
			WithError(err)
	}
	clientSet.ControllerRuntimeClient = k8sClient
	clientSet.Clientsets = kubernetes.NewForConfigOrDie(k8sCfg)
	clientSet.Dynamic = dynamic.NewForConfigOrDie(k8sCfg)
	return clientSet, nil
}

// InitK8SClientSetByConfig exported method for initializing K8S clientset from config
func InitK8SClientSetByConfig(k8sCfg *rest.Config) (*K8SClientSet, error) {
	return initK8SClientSetByConfig(k8sCfg)
}

func doLoadMultiClusterK8SClientSet(ctx context.Context) {
	for {
		err := loadMultiClusterK8SClientSet(ctx)
		if err != nil {
			log.Errorf("Failed to load multi-cluster k8s client sets: %v", err)
		}
		time.Sleep(30 * time.Second)
	}
}

func loadMultiClusterK8SClientSet(ctx context.Context) error {
	k8sConfigs, err := loadMultiClusterK8SConfigs(ctx)
	if err != nil {
		return err
	}
	configBytes, err := json.Marshal(k8sConfigs)
	if err != nil {
		return errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to marshal multi-cluster k8s config").
			WithError(err)
	}
	if multiClusterK8SConfigJsonBytes != nil {
		if string(configBytes) == string(multiClusterK8SConfigJsonBytes) {
			return nil
		}
	}
	multiClusterK8SConfigJsonBytes = configBytes
	newMultiClusterK8S := map[string]*K8SClientSet{}
	for clusterName, k8sCfg := range k8sConfigs {
		log.Infof("Loading k8s config for cluster: %s", clusterName)
		restCfg, err := k8sCfg.ToRestConfig()
		if err != nil {
			log.Errorf("Failed to convert k8s config to rest config for cluster %s: %v", clusterName, err)
			return errors.NewError().
				WithCode(errors.CodeInitializeError).
				WithMessage("Failed to convert k8s config to rest config").
				WithError(err)
		}
		k8sClientSet, err := initK8SClientSetByConfig(restCfg)
		if err != nil {
			log.Errorf("Failed to initialize k8s client set for cluster %s: %v", clusterName, err)
			return errors.NewError().
				WithCode(errors.CodeInitializeError).
				WithMessage("Failed to initialize k8s client set").
				WithError(err)
		}
		newMultiClusterK8S[clusterName] = k8sClientSet
		log.Infof("Initialized k8s client set for cluster %s", clusterName)
	}
	multiClusterK8S = newMultiClusterK8S
	log.Infof("Initialized multi-cluster k8s client sets")
	return nil
}

func loadMultiClusterK8SConfigs(ctx context.Context) (MultiClusterConfig, error) {
	secret, err := currentClusterClientset.Clientsets.CoreV1().Secrets(StorageConfigSecretNamespace).Get(ctx, MultiK8SConfigSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to get multi-cluster k8s config secret").
			WithError(err)
	}
	cfg := MultiClusterConfig{}
	err = cfg.LoadFromSecret(secret.Data)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.CodeInitializeError).
			WithMessage("Failed to load multi-cluster k8s config from secret").
			WithError(err)
	}
	return cfg, nil
}
