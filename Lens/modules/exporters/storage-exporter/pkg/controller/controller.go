// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package controller

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/config"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

const (
	wekafsProvisioner = "csi.weka.io"
	labelManagedBy    = "app.kubernetes.io/managed-by"
	labelFilesystem   = "storage-exporter.primus-lens/filesystem"
	labelStorageType  = "storage-exporter.primus-lens/storage-type"
	managedByValue    = "storage-exporter"
	collectorImage    = "alpine:3.19"
	pvPrefix          = "storage-exporter-pv-"
	pvcPrefix         = "storage-exporter-"
	podPrefix         = "storage-collector-"
)

// FilesystemInfo represents a discovered filesystem
type FilesystemInfo struct {
	Name             string
	StorageClassName string
	FilesystemName   string
	StorageType      string
	VolumeType       string
}

// StorageMetrics contains collected metrics for a filesystem
type StorageMetrics struct {
	FilesystemName string
	StorageType    string
	TotalBytes     uint64
	UsedBytes      uint64
	AvailableBytes uint64
	UsagePercent   float64
	TotalInodes    uint64
	UsedInodes     uint64
	FreeInodes     uint64
	Error          error
	CollectedAt    time.Time
}

// Controller manages dynamic PVC and Pod creation for storage metrics collection
type Controller struct {
	client    kubernetes.Interface
	namespace string
	config    *config.StorageExporterConfig

	// Discovered filesystems
	mu          sync.RWMutex
	filesystems map[string]FilesystemInfo
	metrics     map[string]StorageMetrics

	// Stop channel
	stopCh chan struct{}
}

// NewController creates a new storage controller
func NewController(client kubernetes.Interface, namespace string, cfg *config.StorageExporterConfig) *Controller {
	return &Controller{
		client:      client,
		namespace:   namespace,
		config:      cfg,
		filesystems: make(map[string]FilesystemInfo),
		metrics:     make(map[string]StorageMetrics),
		stopCh:      make(chan struct{}),
	}
}

// Start starts the controller
func (c *Controller) Start(ctx context.Context) error {
	log.Info("Starting storage controller")

	// Initial scan
	if err := c.scanStorageClasses(ctx); err != nil {
		log.Errorf("Initial StorageClass scan failed: %v", err)
	}

	// Start watching StorageClasses
	go c.watchStorageClasses(ctx)

	// Start collection loop
	go c.collectionLoop(ctx)

	return nil
}

// Stop stops the controller
func (c *Controller) Stop() {
	close(c.stopCh)
}

// GetMetrics returns the current metrics
func (c *Controller) GetMetrics() map[string]StorageMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]StorageMetrics, len(c.metrics))
	for k, v := range c.metrics {
		result[k] = v
	}
	return result
}

// GetFilesystems returns discovered filesystems
func (c *Controller) GetFilesystems() map[string]FilesystemInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]FilesystemInfo, len(c.filesystems))
	for k, v := range c.filesystems {
		result[k] = v
	}
	return result
}

// scanStorageClasses scans all StorageClasses for WekaFS
func (c *Controller) scanStorageClasses(ctx context.Context) error {
	scList, err := c.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list StorageClasses: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear and rebuild
	newFilesystems := make(map[string]FilesystemInfo)

	for _, sc := range scList.Items {
		if sc.Provisioner != wekafsProvisioner {
			continue
		}

		fsName := getParameterValue(sc, "filesystemName")
		if fsName == "" {
			continue
		}

		// Only keep one StorageClass per filesystem
		if _, exists := newFilesystems[fsName]; !exists {
			newFilesystems[fsName] = FilesystemInfo{
				Name:             sanitizeName(fsName),
				StorageClassName: sc.Name,
				FilesystemName:   fsName,
				StorageType:      "wekafs",
				VolumeType:       getParameterValue(sc, "volumeType"),
			}
			log.Infof("Discovered WekaFS filesystem: %s (StorageClass: %s)", fsName, sc.Name)
		}
	}

	c.filesystems = newFilesystems
	log.Infof("Total discovered filesystems: %d", len(c.filesystems))

	return nil
}

// watchStorageClasses watches for StorageClass changes
func (c *Controller) watchStorageClasses(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		default:
		}

		watcher, err := c.client.StorageV1().StorageClasses().Watch(ctx, metav1.ListOptions{})
		if err != nil {
			log.Errorf("Failed to watch StorageClasses: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}

		for event := range watcher.ResultChan() {
			switch event.Type {
			case watch.Added, watch.Modified, watch.Deleted:
				log.Debug("StorageClass changed, rescanning...")
				if err := c.scanStorageClasses(ctx); err != nil {
					log.Errorf("Failed to rescan StorageClasses: %v", err)
				}
			}
		}
	}
}

// collectionLoop runs the periodic collection
func (c *Controller) collectionLoop(ctx context.Context) {
	interval := c.config.Storage.GetScrapeInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial collection
	c.collectAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.collectAll(ctx)
		}
	}
}

// collectAll collects metrics from all filesystems
func (c *Controller) collectAll(ctx context.Context) {
	c.mu.RLock()
	filesystems := make([]FilesystemInfo, 0, len(c.filesystems))
	for _, fs := range c.filesystems {
		filesystems = append(filesystems, fs)
	}
	c.mu.RUnlock()

	if len(filesystems) == 0 {
		log.Debug("No filesystems to collect")
		return
	}

	log.Infof("Collecting metrics from %d filesystems", len(filesystems))

	var wg sync.WaitGroup
	results := make(chan StorageMetrics, len(filesystems))

	for _, fs := range filesystems {
		wg.Add(1)
		go func(fs FilesystemInfo) {
			defer wg.Done()
			metrics := c.collectFilesystem(ctx, fs)
			results <- metrics
		}(fs)
	}

	// Wait and close results channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// Update metrics
	c.mu.Lock()
	for metrics := range results {
		c.metrics[metrics.FilesystemName] = metrics
		if metrics.Error != nil {
			log.Errorf("Failed to collect %s: %v", metrics.FilesystemName, metrics.Error)
		} else {
			log.Infof("Collected %s: total=%d, used=%d, avail=%d (%.1f%%)",
				metrics.FilesystemName, metrics.TotalBytes, metrics.UsedBytes,
				metrics.AvailableBytes, metrics.UsagePercent)
		}
	}
	c.mu.Unlock()
}

// collectFilesystem collects metrics from a single filesystem
func (c *Controller) collectFilesystem(ctx context.Context, fs FilesystemInfo) StorageMetrics {
	metrics := StorageMetrics{
		FilesystemName: fs.FilesystemName,
		StorageType:    fs.StorageType,
		CollectedAt:    time.Now(),
	}

	// Ensure PVC exists
	pvcName := pvcPrefix + fs.Name
	if err := c.ensurePVC(ctx, fs, pvcName); err != nil {
		metrics.Error = fmt.Errorf("failed to ensure PVC: %w", err)
		return metrics
	}

	// Create collector pod and get results
	result, err := c.runCollectorPod(ctx, fs, pvcName)
	if err != nil {
		metrics.Error = fmt.Errorf("failed to run collector: %w", err)
		return metrics
	}

	// Parse df output
	if err := parseDfOutput(result, &metrics); err != nil {
		metrics.Error = fmt.Errorf("failed to parse df output: %w", err)
		return metrics
	}

	return metrics
}

// ensurePVC ensures the static PV and PVC exist for the filesystem (no quota)
func (c *Controller) ensurePVC(ctx context.Context, fs FilesystemInfo, pvcName string) error {
	pvName := pvPrefix + fs.Name

	// Check if PVC already exists and is bound
	pvc, err := c.client.CoreV1().PersistentVolumeClaims(c.namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err == nil && pvc.Status.Phase == corev1.ClaimBound {
		return nil // PVC exists and bound
	}

	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	// First, ensure static PV exists (without quota)
	if err := c.ensureStaticPV(ctx, fs, pvName, pvcName); err != nil {
		return fmt.Errorf("failed to ensure static PV: %w", err)
	}

	// Then create PVC if not exists
	if errors.IsNotFound(err) {
		emptyStorageClass := ""
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: c.namespace,
				Labels: map[string]string{
					labelManagedBy:   managedByValue,
					labelFilesystem:  fs.FilesystemName,
					labelStorageType: fs.StorageType,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany},
				// Use empty StorageClassName for static PV binding
				StorageClassName: &emptyStorageClass,
				VolumeName:       pvName,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Pi"),
					},
				},
			},
		}

		_, err = c.client.CoreV1().PersistentVolumeClaims(c.namespace).Create(ctx, pvc, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}

		log.Infof("Created PVC %s bound to static PV %s for filesystem %s", pvcName, pvName, fs.FilesystemName)
	}

	// Wait for PVC to be bound
	return c.waitForPVCBound(ctx, pvcName)
}

// ensureStaticPV creates a static PV without quota for the filesystem
func (c *Controller) ensureStaticPV(ctx context.Context, fs FilesystemInfo, pvName, pvcName string) error {
	_, err := c.client.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err == nil {
		return nil // PV exists
	}

	if !errors.IsNotFound(err) {
		return err
	}

	// Create static PV with volumeHandle pointing to the filesystem root (no quota)
	// Format: dir/v1/{filesystemName}/csi-volumes/storage-exporter-{fsName}
	volumeHandle := fmt.Sprintf("dir/v1/%s/csi-volumes/storage-exporter-%s", fs.FilesystemName, fs.Name)

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Labels: map[string]string{
				labelManagedBy:   managedByValue,
				labelFilesystem:  fs.FilesystemName,
				labelStorageType: fs.StorageType,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Pi"),
			},
			VolumeMode: func() *corev1.PersistentVolumeMode {
				mode := corev1.PersistentVolumeFilesystem
				return &mode
			}(),
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              "",
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       wekafsProvisioner,
					VolumeHandle: volumeHandle,
					VolumeAttributes: map[string]string{
						"filesystemName": fs.FilesystemName,
						"volumeType":     "dir/v1",
					},
				},
			},
			ClaimRef: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
				Namespace:  c.namespace,
				Name:       pvcName,
			},
		},
	}

	_, err = c.client.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	log.Infof("Created static PV %s for filesystem %s (no quota)", pvName, fs.FilesystemName)
	return nil
}

// waitForPVCBound waits for PVC to be bound
func (c *Controller) waitForPVCBound(ctx context.Context, pvcName string) error {
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for PVC %s to be bound", pvcName)
		case <-ticker.C:
			pvc, err := c.client.CoreV1().PersistentVolumeClaims(c.namespace).Get(ctx, pvcName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if pvc.Status.Phase == corev1.ClaimBound {
				return nil
			}
			log.Debugf("PVC %s status: %s", pvcName, pvc.Status.Phase)
		}
	}
}

// runCollectorPod creates a pod to run df and returns the output
func (c *Controller) runCollectorPod(ctx context.Context, fs FilesystemInfo, pvcName string) (string, error) {
	podName := podPrefix + fs.Name + "-" + fmt.Sprintf("%d", time.Now().Unix())
	mountPath := "/mnt/storage"

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: c.namespace,
			Labels: map[string]string{
				labelManagedBy:   managedByValue,
				labelFilesystem:  fs.FilesystemName,
				labelStorageType: fs.StorageType,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "collector",
					Image: collectorImage,
					Command: []string{
						"sh", "-c",
						fmt.Sprintf("df -k %s && echo '---INODE---' && df -i %s", mountPath, mountPath),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "storage",
							MountPath: mountPath,
							ReadOnly:  true,
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("16Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "storage",
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

	// Create pod
	_, err := c.client.CoreV1().Pods(c.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create collector pod: %w", err)
	}

	// Ensure cleanup
	defer func() {
		deleteCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = c.client.CoreV1().Pods(c.namespace).Delete(deleteCtx, podName, metav1.DeleteOptions{})
	}()

	// Wait for pod to complete
	output, err := c.waitForPodCompletion(ctx, podName)
	if err != nil {
		return "", err
	}

	return output, nil
}

// waitForPodCompletion waits for pod to complete and returns logs
func (c *Controller) waitForPodCompletion(ctx context.Context, podName string) (string, error) {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for pod %s to complete", podName)
		case <-ticker.C:
			pod, err := c.client.CoreV1().Pods(c.namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return "", err
			}

			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				// Get logs
				return c.getPodLogs(ctx, podName)
			case corev1.PodFailed:
				logs, _ := c.getPodLogs(ctx, podName)
				return "", fmt.Errorf("pod failed: %s", logs)
			}
		}
	}
}

// getPodLogs gets logs from a pod
func (c *Controller) getPodLogs(ctx context.Context, podName string) (string, error) {
	req := c.client.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{})
	logs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer logs.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(logs)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// parseDfOutput parses the df command output
func parseDfOutput(output string, metrics *StorageMetrics) error {
	parts := strings.Split(output, "---INODE---")
	if len(parts) < 1 {
		return fmt.Errorf("invalid df output")
	}

	// Parse disk usage
	lines := strings.Split(strings.TrimSpace(parts[0]), "\n")
	if len(lines) < 2 {
		return fmt.Errorf("invalid df output: not enough lines")
	}

	// Parse the data line (skip header)
	dataLine := lines[len(lines)-1]
	fields := strings.Fields(dataLine)
	if len(fields) < 4 {
		return fmt.Errorf("invalid df output: not enough fields")
	}

	// df -k outputs in 1K blocks
	totalKB, _ := strconv.ParseUint(fields[1], 10, 64)
	usedKB, _ := strconv.ParseUint(fields[2], 10, 64)
	availKB, _ := strconv.ParseUint(fields[3], 10, 64)

	metrics.TotalBytes = totalKB * 1024
	metrics.UsedBytes = usedKB * 1024
	metrics.AvailableBytes = availKB * 1024

	if metrics.TotalBytes > 0 {
		metrics.UsagePercent = float64(metrics.UsedBytes) / float64(metrics.TotalBytes) * 100
	}

	// Parse inode info if available
	if len(parts) >= 2 {
		inodeLines := strings.Split(strings.TrimSpace(parts[1]), "\n")
		if len(inodeLines) >= 2 {
			inodeDataLine := inodeLines[len(inodeLines)-1]
			inodeFields := strings.Fields(inodeDataLine)
			if len(inodeFields) >= 4 {
				metrics.TotalInodes, _ = strconv.ParseUint(inodeFields[1], 10, 64)
				metrics.UsedInodes, _ = strconv.ParseUint(inodeFields[2], 10, 64)
				metrics.FreeInodes, _ = strconv.ParseUint(inodeFields[3], 10, 64)
			}
		}
	}

	return nil
}

func getParameterValue(sc storagev1.StorageClass, key string) string {
	if sc.Parameters == nil {
		return ""
	}
	return sc.Parameters[key]
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")
	return strings.ToLower(name)
}
