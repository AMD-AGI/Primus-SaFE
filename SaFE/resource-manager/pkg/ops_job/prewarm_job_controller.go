/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type PrewarmJobReconciler struct {
	*OpsJobBaseReconciler
	*controller.Controller[string]
}

// SetupPrewarmJobController initializes and registers the PrewarmJobReconciler with the controller manager.
func SetupPrewarmJobController(ctx context.Context, mgr manager.Manager) error {
	workerConcurrent := commonconfig.GetPrewarmWorkerConcurrent()
	r := &PrewarmJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client:        mgr.GetClient(),
			clientManager: utils.NewObjectManagerSingleton(),
		},
	}

	// Initialize worker controller for parallel processing
	r.Controller = controller.NewController[string](r, workerConcurrent)
	r.start(ctx)

	// Register controller to watch OpsJob resources
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Complete(r)
	if err != nil {
		klog.ErrorS(err, "Failed to setup Prewarm Job Controller")
		return err
	}

	klog.Infof("Setup Prewarm Job Controller successfully")
	return nil
}

// start initializes and runs the worker routines for processing prewarm jobs
func (r *PrewarmJobReconciler) start(ctx context.Context) {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
}

// Reconcile is the main control loop for PrewarmJob resources.
func (r *PrewarmJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r)
}

// observe checks if the job is already completed
func (r *PrewarmJobReconciler) observe(ctx context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

// filter determines if the job should be processed by this reconciler.
func (r *PrewarmJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobPrewarmType
}

// handle processes pending prewarm jobs by adding them to worker queue
func (r *PrewarmJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		if err := r.setJobPhase(ctx, job, v1.OpsJobRunning); err != nil {
			klog.ErrorS(err, "Failed to set job phase to Running", "job", job.Name)
			return ctrlruntime.Result{}, err
		}

		r.Add(job.Name)

		return newRequeueAfterResult(job), nil
	}
	return ctrlruntime.Result{}, nil
}

// Do processes the prewarm job by creating DaemonSet and waiting for completion
// This runs in a worker goroutine and can block for up to 1 hour
func (r *PrewarmJobReconciler) Do(ctx context.Context, jobName string) (ctrlruntime.Result, error) {
	klog.Infof("Worker started processing prewarm job %s", jobName)

	// Get the OpsJob
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobName}, job); err != nil {
		klog.ErrorS(err, "Failed to get OpsJob", "jobName", jobName)
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	// Skip if job is already completed
	if job.IsEnd() {
		klog.V(4).Infof("Prewarm job %s already ended, skipping", jobName)
		return ctrlruntime.Result{}, nil
	}

	// Extract image and workspace from inputs
	var image, workspace string
	for _, input := range job.Spec.Inputs {
		if input.Name == "image" {
			image = input.Value
		}
		if input.Name == "workspace" {
			workspace = input.Value
		}
	}
	if image == "" || workspace == "" {
		errMsg := "missing image or workspace parameter in job inputs"
		klog.ErrorS(errors.New(errMsg), "Missing image or workspace parameter in prewarm job inputs", "job", job.Name)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	dsName := fmt.Sprintf("image-prewarm-%s", job.Name)
	cluster := v1.GetClusterId(job)

	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, cluster)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get k8s client factory: %v", err)
		klog.ErrorS(err, "Failed to get k8s client factory", "job", job.Name, "cluster", cluster)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	// Check if DaemonSet already exists
	exists, _ := r.daemonSetExists(ctx, k8sClients, dsName)
	if exists {
		errMsg := fmt.Sprintf("DaemonSet is already exist")
		klog.ErrorS(err, "DaemonSet is already exist", "daemonset", dsName, "job", job.Name)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	// Create DaemonSet
	_, err = r.createPrewarmDaemonSet(ctx, k8sClients, dsName, image, workspace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create DaemonSet %s for image %s: %v", dsName, image, err)
		klog.ErrorS(err, "Failed to create DaemonSet", "daemonset", dsName, "job", job.Name, "image", image)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	// This is OK because we're in a worker goroutine, not blocking the reconcile loop
	finalReady, finalDesired, err := r.checkDaemonSetReady(ctx, k8sClients, dsName, job.Name)
	if err != nil {
		// Timeout or error occurred (DaemonSet already deleted)
		errMsg := fmt.Sprintf("image prewarming failed: %v", err)
		klog.ErrorS(err, "DaemonSet failed to become ready", "daemonset", dsName, "job", job.Name)

		// Include partial progress in outputs even on failure
		failureOutputs := []v1.Parameter{
			{Name: "status", Value: "failed"},
			{Name: "message", Value: errMsg},
		}
		if finalDesired > 0 {
			successRate := float64(finalReady) / float64(finalDesired) * 100
			failureOutputs = append(failureOutputs, v1.Parameter{
				Name:  "prewarm_progress",
				Value: fmt.Sprintf("%.2f%%", successRate),
			})
			failureOutputs = append(failureOutputs, v1.Parameter{
				Name:  "nodes_ready",
				Value: fmt.Sprintf("%d", finalReady),
			})
			failureOutputs = append(failureOutputs, v1.Parameter{
				Name:  "nodes_total",
				Value: fmt.Sprintf("%d", finalDesired),
			})
		}
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, failureOutputs)
	}

	// DaemonSet is ready and has been deleted successfully
	klog.Infof("Image prewarming completed successfully for job %s: %d/%d nodes ready",
		job.Name, finalReady, finalDesired)

	// Set job as completed with final status
	var successRate float64
	if finalDesired > 0 {
		successRate = float64(finalReady) / float64(finalDesired) * 100
	} else {
		successRate = 100.0
	}

	outputs := []v1.Parameter{
		{Name: "status", Value: "completed"},
		{Name: "message", Value: "Image prewarming completed successfully"},
		{Name: "prewarm_progress", Value: fmt.Sprintf("%.2f%%", successRate)},
		{Name: "nodes_ready", Value: fmt.Sprintf("%d", finalReady)},
		{Name: "nodes_total", Value: fmt.Sprintf("%d", finalDesired)},
	}

	klog.Infof("Setting prewarm job %s as succeeded with %d/%d nodes (%.2f%% success rate)",
		job.Name, finalReady, finalDesired, successRate)
	return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobSucceeded, "Image prewarming completed successfully", outputs)
}

// daemonSetExists checks if a DaemonSet with the given name exists
func (r *PrewarmJobReconciler) daemonSetExists(ctx context.Context, k8sClients *k8sclient.ClientFactory, dsName string) (bool, error) {
	_, err := k8sClients.ClientSet().AppsV1().DaemonSets("default").Get(ctx, dsName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "Error checking DaemonSet existence", "daemonset", dsName)
		return false, err
	}

	return true, nil
}

// createPrewarmDaemonSet creates a DaemonSet to pull the specified image on all nodes
func (r *PrewarmJobReconciler) createPrewarmDaemonSet(ctx context.Context, k8sClients *k8sclient.ClientFactory, dsName, image, workspace string) (*appsv1.DaemonSet, error) {
	klog.Infof("Creating DaemonSet %s to prewarm image: %s", dsName, image)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dsName,
			Namespace: "default",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": dsName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": dsName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "prewarm",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"sleep", "infinity"},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyAlways,
					NodeSelector: map[string]string{
						v1.WorkspaceIdLabel: workspace,
					},
				},
			},
		},
	}
	ds, err := k8sClients.ClientSet().AppsV1().DaemonSets("default").Create(ctx, ds, metav1.CreateOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to create DaemonSet", "daemonset", dsName, "image", image)
		return nil, err
	}

	return ds, nil
}

// checkDaemonSetReady waits for all pods in the DaemonSet to be ready or timeout
// It waits up to configured timeout (default 3600 seconds) for the DaemonSet to become ready
// After completion (success or timeout), it deletes the DaemonSet
// Returns the final ready count, desired count, and error if timeout or any other error occurs
func (r *PrewarmJobReconciler) checkDaemonSetReady(ctx context.Context, k8sClients *k8sclient.ClientFactory, dsName string, jobName string) (int32, int32, error) {
	// Get timeout from configuration (default 3600 seconds if not configured)
	timeoutSecond := commonconfig.GetPrewarmTimeoutSecond()
	const (
		checkInterval = 5 * time.Second // Check every 5 seconds
	)
	timeout := time.Duration(timeoutSecond) * time.Second

	klog.Infof("Waiting for DaemonSet %s to be ready (timeout: %v)", dsName, timeout)
	startTime := time.Now()
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	var lastReady, lastDesired int32
	var lastRecordedReady int32 = -1 // Track last recorded ready count for progress updates

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, cleanup and return error
			klog.Warningf("Context cancelled while waiting for DaemonSet %s", dsName)
			// Use a new context for cleanup since the original one is cancelled
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if delErr := r.deleteDaemonSet(cleanupCtx, k8sClients, dsName); delErr != nil {
				klog.ErrorS(delErr, "Failed to delete DaemonSet after context cancellation", "daemonset", dsName)
			}
			return lastReady, lastDesired, fmt.Errorf("context cancelled while waiting for DaemonSet %s", dsName)

		case <-ticker.C:
			elapsed := time.Since(startTime)

			// Check if timeout reached
			if elapsed >= timeout {
				klog.Errorf("Timeout waiting for DaemonSet %s to be ready after %v (ready: %d, desired: %d)",
					dsName, elapsed, lastReady, lastDesired)

				// Delete DaemonSet after timeout
				if delErr := r.deleteDaemonSet(ctx, k8sClients, dsName); delErr != nil {
					klog.ErrorS(delErr, "Failed to delete DaemonSet after timeout", "daemonset", dsName)
					return lastReady, lastDesired, fmt.Errorf("timeout waiting for DaemonSet %s and failed to delete: %v", dsName, delErr)
				}
				klog.Infof("Successfully deleted DaemonSet %s after timeout", dsName)

				return lastReady, lastDesired, fmt.Errorf("timeout after %v waiting for DaemonSet %s to be ready (ready: %d/%d)",
					timeout, dsName, lastReady, lastDesired)
			}

			// Get current DaemonSet status
			currentDs, err := k8sClients.ClientSet().AppsV1().DaemonSets("default").Get(ctx, dsName, metav1.GetOptions{})
			if err != nil {
				klog.ErrorS(err, "Failed to get DaemonSet status", "daemonset", dsName, "elapsed", elapsed)
				// Don't fail immediately on transient errors, continue waiting
				continue
			}

			ready := currentDs.Status.NumberReady
			desired := currentDs.Status.DesiredNumberScheduled

			// Update last known values
			lastReady = ready
			lastDesired = desired
			// Record progress when ready count increases
			if desired > 0 && ready > lastRecordedReady {
				successRate := float64(ready) / float64(desired) * 100
				if err := r.updatePrewarmProgress(ctx, jobName, successRate); err != nil {
					klog.ErrorS(err, "Failed to update prewarm progress", "job", jobName, "ready", ready, "desired", desired)
					// Don't fail the job, just log the error
				} else {
					klog.Infof("Prewarm progress updated for job %s: %d/%d nodes ready (%.2f%%)",
						jobName, ready, desired, successRate)
					lastRecordedReady = ready
				}
			}

			// Check if all pods are ready
			if ready == desired && desired >= 0 {
				// Delete DaemonSet after success
				if delErr := r.deleteDaemonSet(ctx, k8sClients, dsName); delErr != nil {
					klog.ErrorS(delErr, "Failed to delete DaemonSet after completion", "daemonset", dsName)
					return ready, desired, fmt.Errorf("DaemonSet %s ready but failed to delete: %v", dsName, delErr)
				}
				klog.Infof("Successfully deleted DaemonSet %s after image prewarming completed", dsName)

				// Success - all pods ready and DaemonSet deleted
				return ready, desired, nil
			}
		}
	}
}

// updatePrewarmProgress updates the job output with current prewarm progress
func (r *PrewarmJobReconciler) updatePrewarmProgress(ctx context.Context, jobName string, successRate float64) error {
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobName}, job); err != nil {
		return err
	}

	// Update or add progress output
	progressKey := "prewarm_progress"
	progressValue := fmt.Sprintf("%.2f%%", successRate)

	// Check if progress output already exists and update it
	found := false
	for i := range job.Status.Outputs {
		if job.Status.Outputs[i].Name == progressKey {
			job.Status.Outputs[i].Value = progressValue
			found = true
			break
		}
	}

	// If not found, add new output
	if !found {
		job.Status.Outputs = append(job.Status.Outputs, v1.Parameter{
			Name:  progressKey,
			Value: progressValue,
		})
	}

	if err := r.Status().Update(ctx, job); err != nil {
		return err
	}

	return nil
}

// deleteDaemonSet deletes the DaemonSet after image prewarming is complete
func (r *PrewarmJobReconciler) deleteDaemonSet(ctx context.Context, k8sClients *k8sclient.ClientFactory, dsName string) error {

	err := k8sClients.ClientSet().AppsV1().DaemonSets("default").Delete(ctx, dsName, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.V(4).Infof("DaemonSet %s already deleted", dsName)
			return nil
		}
		klog.ErrorS(err, "Failed to delete DaemonSet", "daemonset", dsName)
		return err
	}

	return nil
}
