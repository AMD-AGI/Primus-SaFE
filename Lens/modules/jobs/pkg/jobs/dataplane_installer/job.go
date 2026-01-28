// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dataplane_installer

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// JobSchedule defines when the job runs (every 30 seconds)
	JobSchedule = "@every 30s"

	// InstallerImage is the container image for the installer
	// Can be overridden via INSTALLER_IMAGE env var
	DefaultInstallerImage = "primussafe/primus-lens-installer:latest"

	// InstallerNamespace is where installer jobs run
	InstallerNamespace = "primus-lens"

	// JobLabelKey is the label key for identifying installer jobs
	JobLabelKey = "app.kubernetes.io/name"
	JobLabelValue = "primus-lens-installer"

	// JobTaskIDLabel labels the job with the task ID
	JobTaskIDLabel = "primus-lens/task-id"
)

// DataplaneInstallerJob manages Kubernetes Jobs for dataplane installation
type DataplaneInstallerJob struct {
	facade *cpdb.ControlPlaneFacade
}

// NewDataplaneInstallerJob creates a new DataplaneInstallerJob
func NewDataplaneInstallerJob() *DataplaneInstallerJob {
	return &DataplaneInstallerJob{}
}

// Schedule returns the cron schedule for this job
func (j *DataplaneInstallerJob) Schedule() string {
	return JobSchedule
}

// Run executes the job scheduler
func (j *DataplaneInstallerJob) Run(
	ctx context.Context,
	k8sClient *clientsets.K8SClientSet,
	storageClient *clientsets.StorageClientSet,
) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()

	// Only run in control plane mode
	if !clientsets.IsControlPlaneMode() {
		return stats, nil
	}

	// Initialize facade if not already done
	if j.facade == nil {
		cpClientSet := clientsets.GetControlPlaneClientSet()
		if cpClientSet == nil {
			stats.AddMessage("control plane client not initialized")
			stats.ErrorCount = 1
			return stats, nil
		}
		j.facade = cpClientSet.Facade
	}

	// Get pending tasks
	taskFacade := j.facade.GetDataplaneInstallTask()
	tasks, err := taskFacade.GetPendingTasks(ctx, 10)
	if err != nil {
		stats.AddMessage(fmt.Sprintf("failed to get pending tasks: %v", err))
		stats.ErrorCount = 1
		return stats, nil
	}

	if len(tasks) == 0 {
		return stats, nil
	}

	log.Infof("Found %d pending dataplane install tasks", len(tasks))

	processed := 0
	errCount := int64(0)

	for _, task := range tasks {
		if err := j.processTask(ctx, k8sClient, task); err != nil {
			log.Errorf("Failed to process task %d: %v", task.ID, err)
			errCount++
		} else {
			processed++
		}
	}

	stats.AddMessage(fmt.Sprintf("processed %d tasks, %d errors", processed, errCount))
	stats.ErrorCount = errCount
	return stats, nil
}

// processTask handles a single install task idempotently
func (j *DataplaneInstallerJob) processTask(ctx context.Context, k8sClient *clientsets.K8SClientSet, task *model.DataplaneInstallTask) error {
	taskFacade := j.facade.GetDataplaneInstallTask()

	jobName := j.getJobName(task)
	namespace := InstallerNamespace

	// Check if a Job already exists for this task
	existingJob, err := k8sClient.Clientsets.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check existing job: %w", err)
	}

	if existingJob != nil && existingJob.Name != "" {
		// Job exists, check its status
		return j.handleExistingJob(ctx, k8sClient, task, existingJob)
	}

	// Check if task has a stale job reference (job was deleted externally)
	if task.JobName != "" {
		log.Warnf("Task %d has stale job reference %s, clearing it", task.ID, task.JobName)
		if err := taskFacade.ClearJobInfo(ctx, task.ID); err != nil {
			log.Warnf("Failed to clear stale job info: %v", err)
		}
	}

	// No existing Job, create one
	return j.createInstallerJob(ctx, k8sClient, task, jobName, namespace)
}

// handleExistingJob handles the case where a Job already exists
func (j *DataplaneInstallerJob) handleExistingJob(ctx context.Context, k8sClient *clientsets.K8SClientSet, task *model.DataplaneInstallTask, job *batchv1.Job) error {
	taskFacade := j.facade.GetDataplaneInstallTask()

	// Check Job status
	if job.Status.Succeeded > 0 {
		// Job completed successfully
		log.Infof("Installer job %s completed successfully", job.Name)

		// Mark task as completed if not already
		if task.Status != model.TaskStatusCompleted {
			if err := taskFacade.MarkCompleted(ctx, task.ID); err != nil {
				log.Errorf("Failed to mark task %d as completed: %v", task.ID, err)
			}
		}

		// Clean up the completed Job
		if err := j.deleteJob(ctx, k8sClient, job.Namespace, job.Name); err != nil {
			log.Warnf("Failed to delete completed job %s: %v", job.Name, err)
		}

		return nil
	}

	if job.Status.Failed > 0 {
		// Job failed
		log.Warnf("Installer job %s failed", job.Name)

		// Get failure reason from pod logs if possible
		failureReason := "Job failed"
		if job.Status.Conditions != nil {
			for _, cond := range job.Status.Conditions {
				if cond.Type == batchv1.JobFailed && cond.Message != "" {
					failureReason = cond.Message
					break
				}
			}
		}

		// Check if can retry
		if task.RetryCount < task.MaxRetries {
			log.Infof("Task %d failed, incrementing retry count (%d/%d)", task.ID, task.RetryCount+1, task.MaxRetries)
			if err := taskFacade.IncrementRetry(ctx, task.ID, failureReason); err != nil {
				log.Errorf("Failed to increment retry count: %v", err)
			}
			// Reset for retry - will create a new job on next iteration
			if err := taskFacade.ResetForRetry(ctx, task.ID); err != nil {
				log.Errorf("Failed to reset task for retry: %v", err)
			}
		} else {
			// Max retries exceeded
			if err := taskFacade.MarkFailed(ctx, task.ID, failureReason); err != nil {
				log.Errorf("Failed to mark task %d as failed: %v", task.ID, err)
			}
		}

		// Clean up the failed Job
		if err := j.deleteJob(ctx, k8sClient, job.Namespace, job.Name); err != nil {
			log.Warnf("Failed to delete failed job %s: %v", job.Name, err)
		}

		return nil
	}

	// Job is still running
	log.Infof("Installer job %s is still running for task %d", job.Name, task.ID)

	// Ensure task is marked as running
	if task.Status == model.TaskStatusPending {
		if err := taskFacade.MarkRunning(ctx, task.ID); err != nil {
			log.Errorf("Failed to mark task %d as running: %v", task.ID, err)
		}
	}

	return nil
}

// createInstallerJob creates a new Kubernetes Job for the install task
func (j *DataplaneInstallerJob) createInstallerJob(ctx context.Context, k8sClient *clientsets.K8SClientSet, task *model.DataplaneInstallTask, jobName, namespace string) error {
	taskFacade := j.facade.GetDataplaneInstallTask()

	log.Infof("Creating installer job %s for task %d (cluster: %s)", jobName, task.ID, task.ClusterName)

	// Get installer image from env or use default
	installerImage := os.Getenv("INSTALLER_IMAGE")
	if installerImage == "" {
		installerImage = DefaultInstallerImage
	}

	// Get control plane DB credentials from current pod's environment
	cpDBHost := os.Getenv("CP_DB_HOST")
	if cpDBHost == "" {
		cpDBHost = "primus-lens-control-plane-primary.primus-lens.svc.cluster.local"
	}
	cpDBUser := os.Getenv("CP_DB_USER")
	cpDBPassword := os.Getenv("CP_DB_PASSWORD")
	cpDBName := os.Getenv("CP_DB_NAME")
	if cpDBName == "" {
		cpDBName = "primus-lens-control-plane"
	}

	// Build job spec
	backoffLimit := int32(0) // Don't retry at K8s level, we handle retries ourselves
	ttlSeconds := int32(600) // TTL for job cleanup (10 minutes after completion)
	activeDeadlineSeconds := int64(3600) // 1 hour timeout

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				JobLabelKey:    JobLabelValue,
				JobTaskIDLabel: fmt.Sprintf("%d", task.ID),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						JobLabelKey:    JobLabelValue,
						JobTaskIDLabel: fmt.Sprintf("%d", task.ID),
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "primus-lens-installer",
					Containers: []corev1.Container{
						{
							Name:  "installer",
							Image: installerImage,
							Env: []corev1.EnvVar{
								{Name: "TASK_ID", Value: fmt.Sprintf("%d", task.ID)},
								{Name: "CLUSTER_NAME", Value: task.ClusterName},
								{Name: "CP_DB_HOST", Value: cpDBHost},
								{Name: "CP_DB_PORT", Value: "5432"},
								{Name: "CP_DB_NAME", Value: cpDBName},
								{Name: "CP_DB_USER", Value: cpDBUser},
								{Name: "CP_DB_PASSWORD", Value: cpDBPassword},
								{Name: "CP_DB_SSL_MODE", Value: "require"},
								{Name: "HELM_TIMEOUT", Value: "15m"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Create the Job
	_, err := k8sClient.Clientsets.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Race condition - job was created between check and create
			log.Infof("Job %s already exists (race condition), will handle on next iteration", jobName)
			return nil
		}
		return fmt.Errorf("failed to create job: %w", err)
	}

	// Update task with job info
	if err := taskFacade.SetJobInfo(ctx, task.ID, jobName, namespace); err != nil {
		log.Warnf("Failed to set job info for task %d: %v", task.ID, err)
	}

	// Mark task as running
	if err := taskFacade.MarkRunning(ctx, task.ID); err != nil {
		log.Warnf("Failed to mark task %d as running: %v", task.ID, err)
	}

	log.Infof("Created installer job %s for task %d", jobName, task.ID)
	return nil
}

// deleteJob deletes a K8s Job with propagation policy
func (j *DataplaneInstallerJob) deleteJob(ctx context.Context, k8sClient *clientsets.K8SClientSet, namespace, name string) error {
	propagationPolicy := metav1.DeletePropagationBackground
	return k8sClient.Clientsets.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
}

// getJobName generates a unique job name for a task
func (j *DataplaneInstallerJob) getJobName(task *model.DataplaneInstallTask) string {
	// Use cluster name and task ID to ensure uniqueness
	clusterName := strings.ReplaceAll(task.ClusterName, "_", "-")
	clusterName = strings.ToLower(clusterName)
	if len(clusterName) > 30 {
		clusterName = clusterName[:30]
	}
	return fmt.Sprintf("dp-installer-%s-%d", clusterName, task.ID)
}

