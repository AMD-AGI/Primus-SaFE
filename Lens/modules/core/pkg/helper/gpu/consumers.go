package gpu

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/kubelet"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetGpuConsumerInfo gets the gpu consumer info
func GetGpuConsumerInfo(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSets *clientsets.StorageClientSet, vendor metadata.GpuVendor) ([]model.TopLevelGpuResource, error) {
	pods, err := GetGpuAllocatedPods(ctx, clientSets, vendor)
	if err != nil {
		return nil, err
	}
	nonTopParentResources := map[string]*model.TopLevelGpuResource{}
	topParentResources := map[string]*model.TopLevelGpuResource{}

	for _, pod := range pods {
		podAvgUsage, err := CalculateNodeGpuUsage(ctx, pod.Spec.NodeName, storageClientSets, vendor) //TODO temporary use node avg usage.
		topOwner, parents, err := traceTopOwner(ctx, clientSets.ControllerRuntimeClient, &pod)
		if err != nil {
			// Ignore Pods that cannot be traced (e.g., Job has been cleaned up)
			continue
		}

		gpuRequest := countGpuRequest(&pod, vendor)
		gpuPod := model.GpuPod{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Node:      pod.Spec.NodeName,
			Devices:   []string{}, // TODO: Get bound GPU UUID from containerd
			Stat: model.GpuStat{
				GpuRequest:     gpuRequest,
				GpuUtilization: podAvgUsage * 100,
			},
		}

		if _, ok := topParentResources[string(topOwner.UID)]; !ok {
			topParentResources[string(topOwner.UID)] = &model.TopLevelGpuResource{
				Kind: topOwner.Kind,
				Name: topOwner.Name,
				Uid:  string(topOwner.UID),
			}
		}
		top := topParentResources[string(topOwner.UID)]
		top.Pods = append(top.Pods, gpuPod)
		top.Stat.GpuRequest += gpuRequest

		for _, owner := range parents {
			if owner.UID == topOwner.UID {
				continue
			}
			key := string(owner.UID)
			if _, ok := nonTopParentResources[key]; !ok {
				nonTopParentResources[key] = &model.TopLevelGpuResource{
					Kind: owner.Kind,
					Name: owner.Name,
					Uid:  string(owner.UID),
				}
			}
			resource := nonTopParentResources[key]
			resource.Pods = append(resource.Pods, gpuPod)
			resource.Stat.GpuRequest += gpuRequest
		}
	}

	var result []model.TopLevelGpuResource
	for _, v := range topParentResources {
		v.CalculateGpuUsage()
		result = append(result, *v)
	}
	return result, nil
}

func traceTopOwner(
	ctx context.Context,
	k8sClient client.Client,
	pod *corev1.Pod,
) (*metav1.OwnerReference, []metav1.OwnerReference, error) {
	if len(pod.OwnerReferences) == 0 {
		return nil, nil, fmt.Errorf("pod %s/%s has no owner", pod.Namespace, pod.Name)
	}

	owner := pod.OwnerReferences[0]
	chain := []metav1.OwnerReference{owner}
	namespace := pod.Namespace

	for {
		obj, err := k8sUtil.GetOwnerObject(ctx, k8sClient, owner, namespace)
		if err != nil {
			// Object may have been deleted, such as Job expired and GC'd
			return &owner, chain, nil
		}

		nextOwners := obj.GetOwnerReferences()
		if len(nextOwners) == 0 {
			// Current owner is the top-level controller
			return &owner, chain, nil
		}

		owner = nextOwners[0]
		chain = append(chain, owner)

		if obj.GetNamespace() != "" {
			namespace = obj.GetNamespace()
		}
	}
}

func countGpuRequest(pod *corev1.Pod, vendor metadata.GpuVendor) int {
	resourceName := corev1.ResourceName(metadata.GetResourceName(vendor))
	total := 0
	for _, container := range pod.Spec.Containers {
		if quantity, ok := container.Resources.Requests[resourceName]; ok {
			total += int(quantity.Value())
		}
	}
	return total
}
func GetGpuAllocatedPods(ctx context.Context, clientSets *clientsets.K8SClientSet, vendor metadata.GpuVendor) ([]corev1.Pod, error) {
	result := []corev1.Pod{}
	nodes, err := GetGpuNodes(ctx, clientSets, vendor)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		pods, err := getGpuPodByNode(ctx, node, clientSets, vendor)
		if err != nil {
			return nil, err
		}
		result = append(result, pods...)
	}
	return result, nil
}

func getGpuPodByNode(ctx context.Context, nodeName string, clientSets *clientsets.K8SClientSet, vendor metadata.GpuVendor) ([]corev1.Pod, error) {
	node := &corev1.Node{}
	err := clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		return nil, err
	}
	gpuPods, err := kubelet.GetGpuPods(ctx, node, vendor)
	if err != nil {
		return nil, err
	}
	return gpuPods, nil
}
