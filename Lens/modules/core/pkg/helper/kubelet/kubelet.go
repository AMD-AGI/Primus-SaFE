package kubelet

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	corev1 "k8s.io/api/core/v1"
)

// GetKubeletClient returns a kubelet client for the given node with cluster-specific authentication
func GetKubeletClient(node *corev1.Node, clusterName string) (*clientsets.Client, error) {
	return clientsets.GetOrInitKubeletClient(node.Name, fmt.Sprintf("https://%s:%d", node.Status.Addresses[0].Address, node.Status.DaemonEndpoints.KubeletEndpoint.Port), clusterName)
}

func GetGpuPodsByKubeletAddress(ctx context.Context, nodeName, kubeletAddress, clusterName string, vendor metadata.GpuVendor) ([]corev1.Pod, error) {
	kubeletClient, err := clientsets.GetOrInitKubeletClient(nodeName, kubeletAddress, clusterName)
	if err != nil {
		return nil, err
	}
	return getGpuPods(ctx, kubeletClient, vendor)
}

func GetGpuPods(ctx context.Context, node *corev1.Node, clusterName string, vendor metadata.GpuVendor) ([]corev1.Pod, error) {
	if !k8sUtil.NodeReady(*node) {
		return []corev1.Pod{}, nil
	}
	client, err := GetKubeletClient(node, clusterName)
	if err != nil {
		return nil, err
	}
	return getGpuPods(ctx, client, vendor)
}

func getGpuPods(ctx context.Context, kubeletClient *clientsets.Client, vendor metadata.GpuVendor) ([]corev1.Pod, error) {
	gpuResource := metadata.GetResourceName(vendor)
	pods, err := kubeletClient.GetKubeletPods(ctx)
	if err != nil {
		return nil, err
	}
	if pods == nil || pods.Items == nil || len(pods.Items) == 0 {
		return []corev1.Pod{}, nil
	}
	results := []corev1.Pod{}
	for i := range pods.Items {
		pod := pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, container := range pod.Spec.Containers {
			if quantity, ok := container.Resources.Requests[corev1.ResourceName(gpuResource)]; ok {
				if quantity.Value() <= 0 {
					continue
				}
				results = append(results, pod)
				continue
			}
		}
	}
	return results, nil
}
