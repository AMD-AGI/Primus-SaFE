package kube

import (
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubeadmConfigMapName                          = "kubeadm-config"
	kubeadmConfigMapKeyClusterConfig              = "ClusterConfiguration"
	kubeadmConfigMapSubKeyNetworking              = "networking"
	kubeadmConfigClusterConfigSubKeyPodSubnet     = "podSubnet"
	kubeadmConfigClusterConfigSubKeyServiceSubnet = "serviceSubnet"
)

func GetCIDRForKubeadm(ctx context.Context, kubeClient client.Client) (string, string, error) {
	cm := &corev1.ConfigMap{}
	err := kubeClient.Get(ctx, client.ObjectKey{Name: kubeadmConfigMapName, Namespace: "kube-system"}, cm)
	if err != nil {
		return "", "", err
	}
	data := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(cm.Data[kubeadmConfigMapKeyClusterConfig]), &data)
	if err != nil {
		log.Errorf("failed to unmarshal kubeadm configmap: %v", err)
		return "", "", err
	}
	networkling, ok := data[kubeadmConfigMapSubKeyNetworking].(map[interface{}]interface{})
	if !ok {
		log.Error("failed to get networking from kubeadm configmap")
		return "", "", nil
	}
	podCIDR, _ := networkling[kubeadmConfigClusterConfigSubKeyPodSubnet].(string)
	serviceCIDR, _ := networkling[kubeadmConfigClusterConfigSubKeyServiceSubnet].(string)
	return podCIDR, serviceCIDR, nil
}
