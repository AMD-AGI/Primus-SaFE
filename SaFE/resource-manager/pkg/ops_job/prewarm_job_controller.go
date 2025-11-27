/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
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

// handle processes prewarm jobs in different phases
func (r *PrewarmJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		if err := r.setJobPhase(ctx, job, v1.OpsJobRunning); err != nil {
			klog.ErrorS(err, "Failed to set job phase to Running", "job", job.Name)
			return ctrlruntime.Result{}, err
		}
		r.Add(job.Name)
		return ctrlruntime.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if job.Status.Phase == v1.OpsJobRunning {
		return r.checkAndUpdateJobStatus(ctx, job)
	}

	return ctrlruntime.Result{}, nil
}

// Do creates the DaemonSet for prewarming. Runs once in worker goroutine.
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
		if input.Name == v1.ParameterImage {
			image = input.Value
		}
		if input.Name == v1.ParameterWorkspace {
			workspace = input.Value
		}
	}
	if image == "" || workspace == "" {
		errMsg := "missing image or workspace parameter in job inputs"
		klog.ErrorS(errors.New(errMsg), "Missing image or workspace parameter in prewarm job inputs", "job", job.Name)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	dsName := job.Name
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
		errMsg := fmt.Sprintf("DaemonSet %s already exists", dsName)
		klog.V(4).Infof("DaemonSet %s already exists for job %s, skipping creation", dsName, job.Name)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	// Create DaemonSet
	_, err = r.createPrewarmDaemonSet(ctx, k8sClients, dsName, image, workspace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create DaemonSet %s for image %s: %v", dsName, image, err)
		klog.ErrorS(err, "Failed to create DaemonSet", "daemonset", dsName, "job", job.Name, "image", image)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	klog.Infof("DaemonSet %s created successfully for prewarm job %s", dsName, job.Name)
	return ctrlruntime.Result{}, nil
}

// checkAndUpdateJobStatus checks DaemonSet status and updates job accordingly
func (r *PrewarmJobReconciler) checkAndUpdateJobStatus(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	dsName := job.Name
	cluster := v1.GetClusterId(job)

	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, cluster)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get k8s client factory: %v", err)
		klog.ErrorS(err, "Failed to get k8s client factory", "job", job.Name)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	// Get DaemonSet status
	ds, err := k8sClients.ClientSet().AppsV1().DaemonSets(common.PrimusSafeNamespace).Get(ctx, dsName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			const gracePeriod = 30 * time.Second
			if job.Status.StartedAt != nil {
				elapsed := time.Since(job.Status.StartedAt.Time)
				if elapsed < gracePeriod {
					klog.V(4).Infof("DaemonSet %s not found yet (elapsed: %v), waiting for worker to create it", dsName, elapsed)
					return ctrlruntime.Result{RequeueAfter: 5 * time.Second}, nil
				}
			}

			errMsg := "DaemonSet not found, possibly deleted externally or creation failed"
			klog.ErrorS(err, errMsg, "daemonset", dsName, "job", job.Name)
			return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
		}
		// Transient error, retry
		klog.V(4).ErrorS(err, "Failed to get DaemonSet, will retry", "daemonset", dsName)
		return ctrlruntime.Result{}, err
	}

	ready := ds.Status.NumberReady
	desired := ds.Status.DesiredNumberScheduled

	// Check for timeout
	if job.Status.StartedAt != nil {
		elapsed := time.Since(job.Status.StartedAt.Time)
		timeoutSecond := commonconfig.GetPrewarmTimeoutSecond()
		timeout := time.Duration(timeoutSecond) * time.Second

		if elapsed >= timeout {
			klog.Errorf("Prewarm job %s timeout after %v (timeout: %ds, ready: %d/%d)",
				job.Name, elapsed, timeoutSecond, ready, desired)

			// Get failed pods info before cleanup
			failedPods := r.getFailedPodsInfo(ctx, k8sClients, dsName)

			// Delete DaemonSet
			if delErr := r.deleteDaemonSet(ctx, k8sClients, dsName); delErr != nil {
				klog.ErrorS(delErr, "Failed to delete DaemonSet after timeout", "daemonset", dsName)
			}

			// Build failure message with failed pods info
			errMsg := fmt.Sprintf("Timeout after %v (ready: %d/%d)", elapsed.Round(time.Second), ready, desired)
			if len(failedPods) > 0 {
				errMsg += fmt.Sprintf(". Failed pods: %s", failedPods)
			}

			// Build outputs
			failureOutputs := r.buildJobOutputs("Failed", errMsg, ready, desired)
			return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, failureOutputs)
		}
	}

	if ready == desired && desired >= 0 {
		klog.Infof("Prewarm job %s completed: %d/%d nodes ready", job.Name, ready, desired)

		if delErr := r.deleteDaemonSet(ctx, k8sClients, dsName); delErr != nil {
			klog.ErrorS(delErr, "Failed to delete DaemonSet after completion", "daemonset", dsName)
		}

		outputs := r.buildJobOutputs("Completed", "Image prewarming completed successfully", ready, desired)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobSucceeded, "Image prewarming completed successfully", outputs)
	}

	if desired > 0 {
		successRate := int(float64(ready) / float64(desired) * 100)
		if err := r.updatePrewarmProgress(ctx, job.Name, successRate); err != nil {
			klog.V(4).ErrorS(err, "Failed to update prewarm progress", "job", job.Name)
		}
	}

	return ctrlruntime.Result{RequeueAfter: time.Minute}, nil
}

// buildJobOutputs builds the output parameters for job completion
func (r *PrewarmJobReconciler) buildJobOutputs(status, message string, ready, desired int32) []v1.Parameter {
	var successRate int
	if desired > 0 {
		successRate = int(float64(ready) / float64(desired) * 100)
	} else {
		successRate = 100
	}

	return []v1.Parameter{
		{Name: "status", Value: status},
		{Name: "message", Value: message},
		{Name: "prewarm_progress", Value: fmt.Sprintf("%d%%", successRate)},
		{Name: "nodes_ready", Value: fmt.Sprintf("%d", ready)},
		{Name: "nodes_total", Value: fmt.Sprintf("%d", desired)},
	}
}

// getFailedPodsInfo retrieves information about failed pods in the DaemonSet
func (r *PrewarmJobReconciler) getFailedPodsInfo(ctx context.Context, k8sClients *k8sclient.ClientFactory, dsName string) string {
	labelSelector := fmt.Sprintf("app=%s", dsName)
	pods, err := k8sClients.ClientSet().CoreV1().Pods(common.PrimusSafeNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		klog.V(4).ErrorS(err, "Failed to list pods for DaemonSet", "daemonset", dsName)
		return ""
	}

	var failedNodes []string
	for _, pod := range pods.Items {
		// Check if pod is not running successfully
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
			nodeName := pod.Spec.NodeName
			reason := string(pod.Status.Phase)

			// Get more detailed failure reason from container statuses
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Waiting != nil {
					reason = fmt.Sprintf("%s: %s", cs.State.Waiting.Reason, cs.State.Waiting.Message)
					break
				} else if cs.State.Terminated != nil {
					reason = fmt.Sprintf("%s: %s", cs.State.Terminated.Reason, cs.State.Terminated.Message)
					break
				}
			}

			failedInfo := fmt.Sprintf("%s(%s)", nodeName, reason)
			failedNodes = append(failedNodes, failedInfo)
			klog.Warningf("Pod %s on node %s failed: %s", pod.Name, nodeName, reason)
		}
	}

	if len(failedNodes) > 0 {
		// Limit to first 5 failed nodes to avoid too long message
		if len(failedNodes) > 5 {
			return fmt.Sprintf("%s and %d more", strings.Join(failedNodes[:5], ", "), len(failedNodes)-5)
		}
		return strings.Join(failedNodes, ", ")
	}
	return ""
}

// daemonSetExists checks if a DaemonSet with the given name exists
func (r *PrewarmJobReconciler) daemonSetExists(ctx context.Context, k8sClients *k8sclient.ClientFactory, dsName string) (bool, error) {
	_, err := k8sClients.ClientSet().AppsV1().DaemonSets(common.PrimusSafeNamespace).Get(ctx, dsName, metav1.GetOptions{})
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
			Namespace: common.PrimusSafeNamespace,
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
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("500Mi"),
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
	ds, err := k8sClients.ClientSet().AppsV1().DaemonSets(common.PrimusSafeNamespace).Create(ctx, ds, metav1.CreateOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to create DaemonSet", "daemonset", dsName, "image", image)
		return nil, err
	}

	return ds, nil
}

// updatePrewarmProgress updates the job output with current prewarm progress
func (r *PrewarmJobReconciler) updatePrewarmProgress(ctx context.Context, jobName string, successRate int) error {
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobName}, job); err != nil {
		return err
	}

	// Update or add progress output
	progressKey := "prewarm_progress"
	progressValue := fmt.Sprintf("%d%%", successRate)

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

	err := k8sClients.ClientSet().AppsV1().DaemonSets(common.PrimusSafeNamespace).Delete(ctx, dsName, metav1.DeleteOptions{})
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
