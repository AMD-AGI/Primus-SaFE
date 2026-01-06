package perfetto

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	pftconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/perfetto"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/registry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// CreatePodAsync creates a Perfetto pod asynchronously
func CreatePodAsync(ctx context.Context, dataClusterName string, session *model.TracelensSessions) {
	go func() {
		createCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		if err := CreatePod(createCtx, dataClusterName, session); err != nil {
			log.Errorf("Failed to create Perfetto pod for session %s: %v", session.SessionID, err)
			facade := database.GetFacadeForCluster(dataClusterName).GetTraceLensSession()
			if updateErr := facade.MarkFailed(createCtx, session.SessionID, err.Error()); updateErr != nil {
				log.Errorf("Failed to mark session as failed: %v", updateErr)
			}
		}
	}()
}

// CreatePod creates a Perfetto pod synchronously
func CreatePod(ctx context.Context, dataClusterName string, session *model.TracelensSessions) error {
	cm := clientsets.GetClusterManager()
	mgmtClients := cm.GetCurrentClusterClients()
	if mgmtClients == nil {
		return fmt.Errorf("failed to get management cluster clients")
	}

	k8sClient := mgmtClients.K8SClientSet.Clientsets
	facade := database.GetFacadeForCluster(dataClusterName).GetTraceLensSession()

	// Update session status to creating
	if err := facade.UpdateStatus(ctx, session.SessionID, pftconst.StatusCreating, "Creating pod"); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Generate pod name
	podName := generatePodName(session.SessionID)

	// Build pod spec
	pod := buildPodSpec(ctx, session, podName, dataClusterName)

	// Create pod
	createdPod, err := k8sClient.CoreV1().Pods(session.PodNamespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			log.Warnf("Pod %s already exists, reusing", podName)
			createdPod, err = k8sClient.CoreV1().Pods(session.PodNamespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get existing pod: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create pod: %w", err)
		}
	}

	// Update session with pod info
	if err := facade.UpdatePodInfo(ctx, session.SessionID, createdPod.Name, "", int32(pftconst.DefaultPodPort)); err != nil {
		return fmt.Errorf("failed to update pod info: %w", err)
	}

	// Update status to initializing
	if err := facade.UpdateStatus(ctx, session.SessionID, pftconst.StatusInitializing, "Waiting for pod to be ready"); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Wait for pod to be ready and get IP
	podIP, err := waitForPodReady(ctx, k8sClient, session.PodNamespace, podName)
	if err != nil {
		return fmt.Errorf("pod failed to become ready: %w", err)
	}

	// Mark session as ready
	if err := facade.MarkReady(ctx, session.SessionID, podIP); err != nil {
		return fmt.Errorf("failed to mark session as ready: %w", err)
	}

	log.Infof("Perfetto pod %s is ready with IP %s", podName, podIP)
	return nil
}

// DeletePodAsync deletes a Perfetto pod asynchronously
func DeletePodAsync(session *model.TracelensSessions) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := DeletePod(ctx, session); err != nil {
			log.Errorf("Failed to delete Perfetto pod for session %s: %v", session.SessionID, err)
		}
	}()
}

// DeletePod deletes a Perfetto pod synchronously
func DeletePod(ctx context.Context, session *model.TracelensSessions) error {
	cm := clientsets.GetClusterManager()
	mgmtClients := cm.GetCurrentClusterClients()
	if mgmtClients == nil {
		return fmt.Errorf("failed to get management cluster clients")
	}

	k8sClient := mgmtClients.K8SClientSet.Clientsets
	podName := session.PodName
	if podName == "" {
		podName = generatePodName(session.SessionID)
	}

	err := k8sClient.CoreV1().Pods(session.PodNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	log.Infof("Deleted Perfetto pod %s", podName)
	return nil
}

func generatePodName(sessionID string) string {
	return fmt.Sprintf("perfetto-%s", sessionID)
}

func buildPodSpec(ctx context.Context, session *model.TracelensSessions, podName, dataClusterName string) *corev1.Pod {
	// Get API base URL from environment or use default
	apiBaseURL := os.Getenv("PERFETTO_API_BASE_URL")
	if apiBaseURL == "" {
		apiBaseURL = "http://primus-lens-api.primus-lens.svc.cluster.local:8989"
	}

	// Get image URL from registry config (supports per-cluster configuration)
	// Image URL is constructed from system_config:
	// - registry: from config or default "docker.io"
	// - namespace: from config or default "primussafe"
	// - version: from config.ImageVersions["perfetto-viewer"] or default "latest"
	// Environment variable PERFETTO_IMAGE_TAG can override the version
	imageTag := os.Getenv("PERFETTO_IMAGE_TAG")
	var imageURL string
	if imageTag != "" {
		imageURL = registry.GetImageURLForCluster(ctx, dataClusterName, registry.ImagePerfettoViewer, imageTag)
	} else {
		imageURL = registry.GetDefaultImageURLForCluster(ctx, dataClusterName, registry.ImagePerfettoViewer)
	}
	log.Debugf("Using Perfetto image: %s for cluster: %s", imageURL, dataClusterName)

	// Get internal token for API authentication
	internalToken := os.Getenv("SAFE_INTERNAL_TOKEN")

	labels := map[string]string{
		"app":        "perfetto-viewer",
		"session-id": session.SessionID,
		"managed-by": "primus-lens",
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: session.PodNamespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "perfetto",
					Image:           imageURL,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: int32(pftconst.DefaultPodPort),
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: []corev1.EnvVar{
						{Name: "SESSION_ID", Value: session.SessionID},
						{Name: "PROFILER_FILE_ID", Value: fmt.Sprintf("%d", session.ProfilerFileID)},
						{Name: "API_BASE_URL", Value: apiBaseURL},
						{Name: "CLUSTER", Value: dataClusterName},
						{Name: "INTERNAL_TOKEN", Value: internalToken},
						// Base path for Perfetto UI when accessed through proxy
						{Name: "UI_BASE_PATH", Value: fmt.Sprintf("/lens/v1/perfetto/sessions/%s/ui/", session.SessionID)},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse(pftconst.PodMemoryLimit),
							corev1.ResourceCPU:    resource.MustParse(pftconst.PodCPULimit),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse(pftconst.PodMemoryLimit),
							corev1.ResourceCPU:    resource.MustParse(pftconst.PodCPULimit),
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromInt(pftconst.DefaultPodPort),
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       3,
						TimeoutSeconds:      2,
						SuccessThreshold:    1,
						FailureThreshold:    10,
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromInt(pftconst.DefaultPodPort),
							},
						},
						InitialDelaySeconds: 30,
						PeriodSeconds:       30,
						TimeoutSeconds:      5,
						FailureThreshold:    3,
					},
				},
			},
		},
	}

	return pod
}

func waitForPodReady(ctx context.Context, client kubernetes.Interface, namespace, podName string) (string, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return "", fmt.Errorf("failed to get pod status: %w", err)
			}

			// Check if pod failed
			if pod.Status.Phase == corev1.PodFailed {
				reason := getPodFailureReason(pod)
				return "", fmt.Errorf("pod failed: %s", reason)
			}

			// Check if pod is ready
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return pod.Status.PodIP, nil
				}
			}

			log.Debugf("Waiting for pod %s to be ready, current phase: %s", podName, pod.Status.Phase)
		}
	}
}

func getPodFailureReason(pod *corev1.Pod) string {
	if pod.Status.Message != "" {
		return pod.Status.Message
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.Message != "" {
			return cs.State.Terminated.Message
		}
		if cs.State.Waiting != nil && cs.State.Waiting.Message != "" {
			return cs.State.Waiting.Message
		}
	}
	return "unknown reason"
}

