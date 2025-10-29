package k8sUtil

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func GetTargetPod(ctx context.Context, c client.Client, namespace string, selector labels.Selector, nodeName string) (*corev1.Pod, error) {
	var podList corev1.PodList

	listOpts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: selector,
	}

	if err := c.List(ctx, &podList, listOpts); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == nodeName {
			return &pod, nil
		}
	}

	return nil, fmt.Errorf("no matching pod found")
}

func IsPodDone(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}

	if pod.Status.Phase != corev1.PodSucceeded {
		return false
	}

	return true
}

func IsPodRunning(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}

	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

func HasGPU(pod *corev1.Pod, gpuResource string) bool {
	for _, container := range pod.Spec.Containers {
		if gpuQty, ok := container.Resources.Requests[corev1.ResourceName(gpuResource)]; ok {
			if !gpuQty.IsZero() {
				return true
			}
		}
		if gpuQty, ok := container.Resources.Limits[corev1.ResourceName(gpuResource)]; ok {
			if !gpuQty.IsZero() {
				return true
			}
		}
	}
	return false
}

func GetGpuAllocated(pod *corev1.Pod, gpuResource string) int {
	total := 0
	for _, container := range pod.Spec.Containers {
		if gpuQty, ok := container.Resources.Requests[corev1.ResourceName(gpuResource)]; ok {
			if !gpuQty.IsZero() {
				total += int(gpuQty.Value())
			}
		}
	}
	return total
}

func GetCompeletedAt(pod *corev1.Pod) time.Time {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionFalse && condition.Reason == "PodCompleted" {
			return condition.LastTransitionTime.Time
		}
	}
	return time.Time{}
}
