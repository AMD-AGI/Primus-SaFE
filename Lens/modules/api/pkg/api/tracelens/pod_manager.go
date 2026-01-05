package tracelens

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// CreatePodAsync creates a TraceLens pod asynchronously (stateless)
// Note: Pod is always created in the management cluster (where API runs),
// regardless of which data cluster the profiler file belongs to.
func CreatePodAsync(ctx context.Context, dataClusterName string, session *model.TracelensSessions, profilerFilePath string) {
	go func() {
		// Create a new context with timeout
		createCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := CreatePod(createCtx, dataClusterName, session, profilerFilePath); err != nil {
			log.Errorf("Failed to create pod for session %s: %v", session.SessionID, err)
			// Mark session as failed in the data cluster's database
			facade := database.GetFacadeForCluster(dataClusterName).GetTraceLensSession()
			if updateErr := facade.MarkFailed(createCtx, session.SessionID, err.Error()); updateErr != nil {
				log.Errorf("Failed to mark session as failed: %v", updateErr)
			}
		}
	}()
}

// CreatePod creates a TraceLens pod synchronously
// Pod is created in the management cluster (current cluster where API runs),
// but session metadata is stored in the data cluster's database.
func CreatePod(ctx context.Context, dataClusterName string, session *model.TracelensSessions, profilerFilePath string) error {
	// Get kubernetes client for the MANAGEMENT cluster (where API runs)
	// This is the current cluster, not the data cluster
	cm := clientsets.GetClusterManager()
	mgmtClients := cm.GetCurrentClusterClients()
	if mgmtClients == nil {
		return fmt.Errorf("failed to get management cluster clients")
	}

	k8sClient := mgmtClients.K8SClientSet.Clientsets
	// Session metadata is stored in the DATA cluster's database
	facade := database.GetFacadeForCluster(dataClusterName).GetTraceLensSession()

	// Update session status to creating
	if err := facade.UpdateStatus(ctx, session.SessionID, tlconst.StatusCreating, "Creating pod"); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Generate pod name (deterministic based on session ID)
	podName := generatePodName(session.SessionID)

	// Build pod spec
	pod := buildPodSpec(session, podName, profilerFilePath)

	// Create pod (K8s handles duplicate via AlreadyExists error)
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
	if err := facade.UpdatePodInfo(ctx, session.SessionID, createdPod.Name, "", int32(tlconst.DefaultPodPort)); err != nil {
		return fmt.Errorf("failed to update pod info: %w", err)
	}

	// Update status to initializing
	if err := facade.UpdateStatus(ctx, session.SessionID, tlconst.StatusInitializing, "Waiting for pod to be ready"); err != nil {
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

	log.Infof("TraceLens pod %s is ready with IP %s for session %s", podName, podIP, session.SessionID)
	return nil
}

// DeletePod deletes a TraceLens pod from the management cluster
// Note: Pods are always in the management cluster, regardless of which data cluster the session belongs to
func DeletePod(ctx context.Context, podName, namespace string) error {
	cm := clientsets.GetClusterManager()
	mgmtClients := cm.GetCurrentClusterClients()
	if mgmtClients == nil {
		return fmt.Errorf("failed to get management cluster clients")
	}

	k8sClient := mgmtClients.K8SClientSet.Clientsets

	// Delete pod with grace period
	gracePeriod := int64(0) // Force delete
	deletePolicy := metav1.DeletePropagationForeground
	err := k8sClient.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
		PropagationPolicy:  &deletePolicy,
	})

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	log.Infof("Deleted TraceLens pod %s in namespace %s", podName, namespace)
	return nil
}

// GetPodStatus gets the current status of a TraceLens pod
func GetPodStatus(ctx context.Context, clusterName, podName, namespace string) (*PodStatusInfo, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster clients: %w", err)
	}

	k8sClient := clients.K8SClientSet.Clientsets
	pod, err := k8sClient.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return &PodStatusInfo{
				Exists: false,
				Phase:  "NotFound",
			}, nil
		}
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	status := &PodStatusInfo{
		Exists: true,
		Phase:  string(pod.Status.Phase),
		PodIP:  pod.Status.PodIP,
	}

	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			status.Ready = cond.Status == corev1.ConditionTrue
			break
		}
	}

	return status, nil
}

// PodStatusInfo contains pod status information
type PodStatusInfo struct {
	Exists bool   `json:"exists"`
	Phase  string `json:"phase"`
	Ready  bool   `json:"ready"`
	PodIP  string `json:"pod_ip,omitempty"`
}

// Helper functions

func generatePodName(sessionID string) string {
	podName := fmt.Sprintf("tracelens-%s", sessionID)
	if len(podName) > 63 {
		podName = podName[:63]
	}
	return podName
}

func buildPodSpec(session *model.TracelensSessions, podName, profilerFilePath string) *corev1.Pod {
	// Get resource limits based on profile
	memoryLimit, cpuLimit := getResourceLimits(session.ResourceProfile)

	// Build base URL path for Streamlit (matches proxy route)
	baseURLPath := fmt.Sprintf("/v1/tracelens/sessions/%s/ui", session.SessionID)

	labels := map[string]string{
		"app":                            "tracelens",
		"tracelens.lens.primus/session":  session.SessionID,
		"tracelens.lens.primus/workload": session.WorkloadUID,
	}

	// API base URL for fetching profiler files
	// Since pod runs in management cluster, it can access API via cluster service
	apiBaseURL := "http://primus-lens-api.primus-lens.svc.cluster.local:8989"

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: session.PodNamespace,
			Labels:    labels,
			Annotations: map[string]string{
				"tracelens.lens.primus/profiler-file": profilerFilePath,
				"tracelens.lens.primus/expires-at":    session.ExpiresAt.Format(time.RFC3339),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "tracelens",
					Image: tlconst.DefaultTraceLensImage,
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: int32(tlconst.DefaultPodPort),
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "SESSION_ID",
							Value: session.SessionID,
						},
						{
							Name:  "PROFILER_FILE_ID",
							Value: fmt.Sprintf("%d", session.ProfilerFileID),
						},
						{
							Name:  "API_BASE_URL",
							Value: apiBaseURL,
						},
						{
							Name:  "BASE_URL_PATH",
							Value: baseURLPath,
						},
						{
							Name:  "TRACE_FILE_PATH",
							Value: profilerFilePath,
						},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse(memoryLimit),
							corev1.ResourceCPU:    resource.MustParse(cpuLimit),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse(memoryLimit),
							corev1.ResourceCPU:    resource.MustParse(cpuLimit),
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: baseURLPath + "/_stcore/health",
								Port: intstr.FromInt32(int32(tlconst.DefaultPodPort)),
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       5,
						TimeoutSeconds:      3,
						FailureThreshold:    30, // Allow up to 2.5 minutes for startup
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: baseURLPath + "/_stcore/health",
								Port: intstr.FromInt32(int32(tlconst.DefaultPodPort)),
							},
						},
						InitialDelaySeconds: 60,
						PeriodSeconds:       30,
						TimeoutSeconds:      5,
						FailureThreshold:    3,
					},
				},
			},
		},
	}
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
				return "", fmt.Errorf("failed to get pod: %w", err)
			}

			// Check pod phase
			switch pod.Status.Phase {
			case corev1.PodFailed:
				return "", fmt.Errorf("pod failed: %s", getPodFailureReason(pod))
			case corev1.PodSucceeded:
				return "", fmt.Errorf("pod completed unexpectedly")
			case corev1.PodRunning:
				// Check if ready
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						if pod.Status.PodIP != "" {
							return pod.Status.PodIP, nil
						}
					}
				}
			}
			log.Debugf("Waiting for pod %s to be ready, current phase: %s", podName, pod.Status.Phase)
		}
	}
}

func getResourceLimits(profile string) (memory, cpu string) {
	p := tlconst.GetResourceProfile(profile)
	if p != nil {
		return p.Memory, fmt.Sprintf("%d", p.CPU)
	}
	// fallback to medium if profile not found
	return "16Gi", "2"
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
