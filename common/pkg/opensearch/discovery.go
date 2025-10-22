package opensearch

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	multiClusterClients = map[string]*SearchClient{}
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
	v1.AddToScheme,
}

var (
	scheme *runtime.Scheme
)

func GetOpensearchClient(clusterName string) *SearchClient {
	if searchClient, exists := multiClusterClients[clusterName]; exists {
		return searchClient
	}
	return nil
}

const (
	opensearchConfigNamespace  = "primus-safe"
	opensearchConfigSecretName = "primus-safe-opensearch-config"
	opensearchEndpointTemplate = "%s://%s.%s.svc.cluster.local:9200"
)

type opensearchSecretData struct {
	NodePort int32  `json:"nodePort"`
	Scheme   string `json:"scheme"`
	Service  string `json:"service"`
	Username string `json:"username"`
	Password string `json:"password"`
	Prefix   string `json:"index_prefix"`
}

func (o opensearchSecretData) Validate() error {
	if o.Service == "" || o.Scheme == "" || o.Username == "" || o.Password == "" || o.Prefix == "" {
		return fmt.Errorf("invalid values for opensearch secret")
	}
	return nil
}

func StartDiscover(ctx context.Context) error {
	scheme = runtime.NewScheme()
	err := schemes.AddToScheme(scheme)
	if err != nil {
		return err
	}
	cfg, err := commonclient.GetRestConfigInCluster()
	if err != nil {
		return err
	}
	controlPlaneClient, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}
	clientManager := commonutils.NewObjectManagerSingleton()
	go func() {
		syncLoop(ctx, controlPlaneClient, clientManager)
	}()
	return nil
}

func syncLoop(ctx context.Context, controlPlaneClient client.Client, clientManager *commonutils.ObjectManager) {
	for {
		err := doSync(ctx, controlPlaneClient, clientManager)
		if err != nil {
			klog.Errorf("Failed to sync opensearch clients, err: %v", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func doSync(ctx context.Context, controlPlaneClient client.Client, clientManager *commonutils.ObjectManager) error {
	clusterList := &v1.ClusterList{}
	err := controlPlaneClient.List(ctx, clusterList)
	if err != nil {
		klog.Errorf("Failed to list clusters, err: %v", err)
		return err
	}
	newClients := map[string]*SearchClient{}
	for _, cluster := range clusterList.Items {
		k8sClients, err := apiutils.GetK8sClientFactory(clientManager, cluster.Name)
		if err != nil {
			klog.Errorf("Failed to get k8s client for cluster %s, err: %v", cluster.Name, err)
			continue
		}
		controllerRuntimeClient, err := client.New(k8sClients.RestConfig(), client.Options{
			Scheme: scheme,
		})
		if err != nil {
			klog.Errorf("Failed to get controller runtime client for cluster %s, err: %v", cluster.Name, err)
			continue
		}
		opensearchConfig, err := initOpensearchClient(ctx, cluster.Name, controllerRuntimeClient, controlPlaneClient)
		if err != nil {
			klog.Errorf("Failed to init opensearch client for cluster %s, err: %v", cluster.Name, err)
			continue
		}
		opensearchClient, err := initOpensearchClient(ctx, cluster.Name, controllerRuntimeClient, controlPlaneClient)
		if err != nil {
			klog.Errorf("Failed to init opensearch client for cluster %s, err: %v", cluster.Name, err)
			continue
		}
		if opensearchClient == nil || opensearchConfig == nil {
			klog.Infof("Opensearch is not configured for cluster %s", cluster.Name)
			continue
		}
		newClients[cluster.Name] = opensearchClient
	}
	multiClusterClients = newClients
	return nil
}

func initOpensearchClient(ctx context.Context, clusterName string, clusterClient client.Client, controlPlaneClient client.Client) (*SearchClient, error) {
	if searchClient, exists := multiClusterClients[clusterName]; exists {
		return searchClient, nil
	}
	cfg, err := syncOpensearchService(ctx, clusterName, clusterClient, controlPlaneClient)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	searchClient := NewClient(*cfg)
	multiClusterClients[clusterName] = searchClient
	return searchClient, nil
}

func syncOpensearchService(ctx context.Context, clusterName string, clusterClient, controlPlaneClient client.Client) (*SearchClientConfig, error) {
	cfg, err := getOpensearchConfig(ctx, clusterClient)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	syncedService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getServiceNameForClusterOpensearch(clusterName),
			Namespace: opensearchConfigNamespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     cfg.Scheme,
					Protocol: corev1.ProtocolTCP,
					Port:     9200,
					TargetPort: intstr.IntOrString{
						IntVal: cfg.NodePort,
					},
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
	err = controlPlaneClient.Create(ctx, syncedService)
	if client.IgnoreAlreadyExists(err) != nil {
		return nil, err
	}
	endpoints, err := desireEndpoint(ctx, clusterName, clusterClient, cfg)
	if err != nil {
		return nil, err
	}
	if endpoints == nil {
		return nil, nil
	}
	err = controlPlaneClient.Create(ctx, endpoints)
	if err != nil {
		if client.IgnoreAlreadyExists(err) != nil {
			return nil, err
		}
		err = controlPlaneClient.Update(ctx, endpoints)
		if err != nil {
			return nil, err
		}
	}
	result := &SearchClientConfig{
		Username: cfg.Username,
		Password: cfg.Password,
		Endpoint: fmt.Sprintf(opensearchEndpointTemplate, cfg.Scheme, getServiceNameForClusterOpensearch(clusterName), opensearchConfigNamespace),
		Prefix:   cfg.Prefix,
	}
	return result, nil
}

func getControlPlaneNode(ctx context.Context, c client.Client) ([]*corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := c.List(ctx, nodes)
	if err != nil {
		return nil, err
	}
	var controlPlaneNodes []*corev1.Node
	for _, node := range nodes.Items {
		if _, ok := node.Labels[common.KubernetesControlPlane]; ok {
			controlPlaneNodes = append(controlPlaneNodes, &node)
		}
	}
	return controlPlaneNodes, nil
}

func desireEndpoint(ctx context.Context, clusterName string, client client.Client, cfg *opensearchSecretData) (*corev1.Endpoints, error) {
	// 获取master节点
	masterNodes, err := getControlPlaneNode(ctx, client)
	if err != nil {
		return nil, err
	}
	ep := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getServiceNameForClusterOpensearch(clusterName),
			Namespace: opensearchConfigNamespace,
		},
		Subsets: []corev1.EndpointSubset{},
	}
	ss := corev1.EndpointSubset{
		Addresses: []corev1.EndpointAddress{},
		Ports: []corev1.EndpointPort{
			{
				Name:     cfg.Scheme,
				Protocol: corev1.ProtocolTCP,
				Port:     cfg.NodePort,
			},
		},
	}
	for _, node := range masterNodes {
		ss.Addresses = append(ss.Addresses, corev1.EndpointAddress{
			IP: node.Status.Addresses[0].Address,
			TargetRef: &corev1.ObjectReference{
				Kind: "Node",
				UID:  node.UID,
			},
		})
	}
	ep.Subsets = append(ep.Subsets, ss)
	return ep, nil
}

func getServiceNameForClusterOpensearch(clusterName string) string {
	return fmt.Sprintf("primus-safe-opensearch-%s", clusterName)
}

func getOpensearchConfig(ctx context.Context, clusterClient client.Client) (*opensearchSecretData, error) {
	sec := &corev1.Secret{}
	err := clusterClient.Get(ctx, types.NamespacedName{
		Namespace: opensearchConfigNamespace,
		Name:      opensearchConfigSecretName,
	}, sec)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil
		}
		return nil, err
	}
	cfg, err := decodeOpensearchConfig(sec)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func decodeOpensearchConfig(sec *corev1.Secret) (*opensearchSecretData, error) {
	config := &opensearchSecretData{}
	if username, ok := sec.Data["username"]; ok {
		config.Username = string(username)
	}
	if password, ok := sec.Data["password"]; ok {
		config.Password = string(password)
	}
	if prefix, ok := sec.Data["index_prefix"]; ok {
		config.Prefix = string(prefix)
	}
	if service, ok := sec.Data["service"]; ok {
		config.Service = string(service)
	}
	if scheme, ok := sec.Data["scheme"]; ok {
		config.Scheme = string(scheme)
	}
	if nodePort, ok := sec.Data["nodePort"]; ok {
		var n int
		_, err := fmt.Sscanf(string(nodePort), "%d", &n)
		if err != nil {
			return nil, fmt.Errorf("invalid nodePort value: %v", err)
		}
		config.NodePort = int32(n)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}
