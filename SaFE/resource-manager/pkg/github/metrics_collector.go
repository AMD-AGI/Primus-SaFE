/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
)

// MetricsCollector collects workflow metrics from runner Pod PVCs in data plane clusters.
type MetricsCollector struct {
	store *Store
}

func NewMetricsCollector(store *Store) *MetricsCollector {
	return &MetricsCollector{store: store}
}

// CollectFromPVC creates a temp pod in the remote cluster, reads matching files via
// K8s exec + base64, parses JSON metrics, and stores them in SaFE DB.
func (c *MetricsCollector) CollectFromPVC(ctx context.Context,
	k8sClient kubernetes.Interface, restConfig *rest.Config,
	namespace, pvcName string, filePatterns []string,
	runID int, configID int64) error {

	podName := fmt.Sprintf("safe-metrics-collect-%d-%d", runID, time.Now().Unix()%10000)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          "primus-safe-metrics-collector",
				"primus-safe.collection.run-id": fmt.Sprint(runID),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:         corev1.RestartPolicyNever,
			ActiveDeadlineSeconds: int64Ptr(300),
			Containers: []corev1.Container{
				{
					Name:    "collector",
					Image:   "busybox:1.36",
					Command: []string{"sleep", "3600"},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "workspace", MountPath: "/workspace", ReadOnly: true},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
							ReadOnly:  true,
						},
					},
				},
			},
		},
	}

	created, err := k8sClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create temp pod: %w", err)
	}
	defer func() {
		k8sClient.CoreV1().Pods(namespace).Delete(context.Background(), podName, metav1.DeleteOptions{})
		klog.V(2).Infof("[metrics-collector] deleted temp pod %s/%s", namespace, podName)
	}()

	if err := waitForPodRunning(ctx, k8sClient, namespace, created.Name, 60*time.Second); err != nil {
		return fmt.Errorf("wait temp pod: %w", err)
	}

	files, err := listMatchingFiles(ctx, k8sClient, restConfig, namespace, podName, "/workspace", filePatterns)
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}

	klog.V(2).Infof("[metrics-collector] found %d matching files in %s/%s", len(files), namespace, pvcName)

	for _, filePath := range files {
		data, err := readFileBase64(ctx, k8sClient, restConfig, namespace, podName, filePath)
		if err != nil {
			klog.V(1).Infof("[metrics-collector] read %s: %v", filePath, err)
			continue
		}

		rows, err := ParseFileToRows(data, filePath)
		if err != nil {
			klog.V(1).Infof("[metrics-collector] parse %s: %v", filePath, err)
			continue
		}

		for _, row := range rows {
			rowJSON, _ := json.Marshal(row)
			c.store.InsertMetricRow(ctx, configID, int64(runID), filePath, rowJSON)
		}
		klog.Infof("[metrics-collector] stored %d rows from %s", len(rows), filePath)
	}

	return nil
}

type parsedMetric struct {
	Timestamp  *time.Time
	Dimensions map[string]interface{}
	Metrics    map[string]interface{}
}

func parseMetricsJSON(data []byte) ([]parsedMetric, error) {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	switch v := raw.(type) {
	case []interface{}:
		var results []parsedMetric
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				results = append(results, parsedMetric{
					Metrics:    m,
					Dimensions: map[string]interface{}{},
				})
			}
		}
		return results, nil
	case map[string]interface{}:
		return []parsedMetric{{
			Metrics:    v,
			Dimensions: map[string]interface{}{},
		}}, nil
	default:
		return nil, fmt.Errorf("unexpected JSON type: %T", raw)
	}
}

func listMatchingFiles(ctx context.Context, k8sClient kubernetes.Interface, restConfig *rest.Config,
	namespace, podName, basePath string, patterns []string) ([]string, error) {

	cmd := []string{"find", basePath, "-type", "f", "-name", "*.json", "-o", "-name", "*.json.gz"}
	stdout, err := execInPod(ctx, k8sClient, restConfig, namespace, podName, cmd)
	if err != nil {
		return nil, err
	}

	var matched []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if matchesAnyPattern(line, patterns) {
			matched = append(matched, line)
		}
		if len(matched) >= 50 {
			break
		}
	}
	return matched, nil
}

func matchesAnyPattern(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	base := filepath.Base(path)
	for _, p := range patterns {
		cleanPattern := strings.ReplaceAll(p, "**/", "")
		if matched, _ := filepath.Match(cleanPattern, base); matched {
			return true
		}
		if matched, _ := filepath.Match(p, path); matched {
			return true
		}
	}
	return false
}

func readFileBase64(ctx context.Context, k8sClient kubernetes.Interface, restConfig *rest.Config,
	namespace, podName, filePath string) ([]byte, error) {

	cmd := []string{"base64", "-w0", filePath}
	stdout, err := execInPod(ctx, k8sClient, restConfig, namespace, podName, cmd)
	if err != nil {
		return nil, fmt.Errorf("base64 read: %w", err)
	}

	return base64.StdEncoding.DecodeString(strings.TrimSpace(stdout))
}

func execInPod(ctx context.Context, k8sClient kubernetes.Interface, restConfig *rest.Config,
	namespace, podName string, cmd []string) (string, error) {

	req := k8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "collector",
			Command:   cmd,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", fmt.Errorf("exec: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

func waitForPodRunning(ctx context.Context, k8sClient kubernetes.Interface, namespace, name string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("timeout waiting for pod %s/%s to be running", namespace, name)
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pod, err := k8sClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			if pod.Status.Phase == corev1.PodRunning {
				return nil
			}
			if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
				return fmt.Errorf("pod %s ended with phase %s", name, pod.Status.Phase)
			}
		}
	}
}

func int64Ptr(v int64) *int64 { return &v }
