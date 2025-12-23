/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/channel"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

const (
	// Status constants for DeploymentRequest
	StatusDeploying = "deploying"
	StatusDeployed  = "deployed"
	StatusFailed    = "failed"
)

// CDOpsJobReconciler reconciles CD OpsJob resources and syncs status to database.
type CDOpsJobReconciler struct {
	client.Client
	dbClient     dbclient.Interface
	emailChannel *channel.EmailChannel
}

// SetupCDOpsJobController initializes and registers the CDOpsJobReconciler with the controller manager.
func SetupCDOpsJobController(ctx context.Context, mgr manager.Manager) error {
	// Skip if database is not enabled
	if !commonconfig.IsDBEnable() {
		klog.Info("Database not enabled, skipping CD OpsJob controller")
		return nil
	}

	dbClient := dbclient.NewClient()
	if dbClient == nil {
		return fmt.Errorf("failed to create database client")
	}

	r := &CDOpsJobReconciler{
		Client:   mgr.GetClient(),
		dbClient: dbClient,
	}

	// Initialize email channel if notification is enabled
	if commonconfig.IsNotificationEnable() {
		conf, err := channel.ReadConfigFromFile(commonconfig.GetNotificationConfig())
		if err != nil {
			klog.Warningf("Failed to read notification config: %v", err)
		} else if conf.Email != nil {
			emailCh := &channel.EmailChannel{}
			if err := emailCh.Init(*conf); err != nil {
				klog.Warningf("Failed to initialize email channel: %v", err)
			} else {
				r.emailChannel = emailCh
				klog.Info("Email channel initialized for CD OpsJob controller")
			}
		}
	}

	// Create predicate to only watch CD type OpsJobs
	cdOpsJobPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			job, ok := e.Object.(*v1.OpsJob)
			return ok && job.Spec.Type == v1.OpsJobCDType
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			newJob, ok := e.ObjectNew.(*v1.OpsJob)
			if !ok || newJob.Spec.Type != v1.OpsJobCDType {
				return false
			}
			oldJob, ok := e.ObjectOld.(*v1.OpsJob)
			if !ok {
				return true
			}
			// Only reconcile if phase changed
			return oldJob.Status.Phase != newJob.Status.Phase
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			job, ok := e.Object.(*v1.OpsJob)
			return ok && job.Spec.Type == v1.OpsJobCDType
		},
	}

	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(cdOpsJobPredicate)).
		Complete(r)
	if err != nil {
		return err
	}

	klog.Info("Setup CD OpsJob Status Sync Controller successfully")
	return nil
}

// Reconcile handles OpsJob status changes and syncs to database.
func (r *CDOpsJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	job := &v1.OpsJob{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	// Skip if not CD type
	if job.Spec.Type != v1.OpsJobCDType {
		return ctrlruntime.Result{}, nil
	}

	// Get deployment request ID from job inputs
	requestIdStr := getParameterValue(job, v1.ParameterDeploymentRequestId)
	if requestIdStr == "" {
		klog.Warningf("CD OpsJob %s missing deployment request ID", job.Name)
		return ctrlruntime.Result{}, nil
	}

	requestId, err := strconv.ParseInt(requestIdStr, 10, 64)
	if err != nil {
		klog.ErrorS(err, "Invalid deployment request ID", "job", job.Name, "id", requestIdStr)
		return ctrlruntime.Result{}, nil
	}

	// Get deployment request from database
	dbReq, err := r.dbClient.GetDeploymentRequest(ctx, requestId)
	if err != nil {
		klog.ErrorS(err, "Failed to get deployment request", "id", requestId)
		return ctrlruntime.Result{}, nil
	}

	// Sync status based on OpsJob phase
	var newStatus string
	var failureReason string

	switch job.Status.Phase {
	case v1.OpsJobPending, v1.OpsJobRunning:
		newStatus = StatusDeploying
	case v1.OpsJobSucceeded:
		newStatus = StatusDeployed
		// Create snapshot on success
		if err := r.createSnapshot(ctx, requestId, dbReq.EnvConfig); err != nil {
			klog.ErrorS(err, "Failed to create snapshot", "id", requestId)
		} else {
			klog.Infof("Snapshot created for deployment request %d", requestId)
		}
	case v1.OpsJobFailed:
		newStatus = StatusFailed
		// Extract failure reason from job conditions
		failureReason = getJobFailureReason(job)
		// Send failure notification (pass job to get userId for email lookup)
		r.sendDeploymentFailureEmail(ctx, job, dbReq, failureReason)
	default:
		// Unknown phase, don't update
		return ctrlruntime.Result{}, nil
	}

	// Skip if status unchanged
	if dbReq.Status == newStatus {
		return ctrlruntime.Result{}, nil
	}

	// Update database status
	dbReq.Status = newStatus
	if failureReason != "" {
		dbReq.FailureReason = dbutils.NullString(failureReason)
	}

	if err := r.dbClient.UpdateDeploymentRequest(ctx, dbReq); err != nil {
		klog.ErrorS(err, "Failed to update deployment request status", "id", requestId)
		return ctrlruntime.Result{RequeueAfter: time.Second * 5}, nil
	}

	klog.Infof("Synced CD OpsJob %s status to database: request=%d, status=%s", job.Name, requestId, newStatus)
	return ctrlruntime.Result{}, nil
}

// getParameterValue retrieves a parameter value from job inputs.
func getParameterValue(job *v1.OpsJob, name string) string {
	param := job.GetParameter(name)
	if param != nil {
		return param.Value
	}
	return ""
}

// getJobFailureReason extracts failure reason from job conditions or outputs.
func getJobFailureReason(job *v1.OpsJob) string {
	// Check outputs first
	for _, output := range job.Status.Outputs {
		if output.Name == "result" && output.Value != "" {
			return output.Value
		}
	}

	// Check conditions
	for _, condition := range job.Status.Conditions {
		if condition.Status == "False" && condition.Message != "" {
			return condition.Message
		}
	}

	return "CD deployment failed"
}

// createSnapshot creates a deployment snapshot.
func (r *CDOpsJobReconciler) createSnapshot(ctx context.Context, requestId int64, envConfig string) error {
	snapshot := &dbclient.EnvironmentSnapshot{
		DeploymentRequestId: requestId,
		EnvConfig:           envConfig,
	}
	_, err := r.dbClient.CreateEnvironmentSnapshot(ctx, snapshot)
	return err
}

// sendDeploymentFailureEmail sends an email notification when deployment fails.
func (r *CDOpsJobReconciler) sendDeploymentFailureEmail(ctx context.Context, job *v1.OpsJob, req *dbclient.DeploymentRequest, failReason string) {
	if r.emailChannel == nil {
		return
	}

	// Get user email by userId from OpsJob
	userEmail := r.getUserEmail(ctx, v1.GetUserId(job))
	if userEmail == "" {
		klog.Warningf("Cannot send deployment failure email: user email not found for job %s", job.Name)
		return
	}

	message := &model.Message{
		Email: &model.EmailMessage{
			To:    []string{userEmail},
			Title: fmt.Sprintf("[CD Deployment Failed] Request #%d Failed", req.Id),
			Content: fmt.Sprintf(`
				<h2>Deployment Failure Notification</h2>
				<table style="border-collapse: collapse; width: 100%%;">
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Request ID</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd;">%d</td>
					</tr>
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Deployer</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
					</tr>
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Failure Reason</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd; color: #c53030;">%s</td>
					</tr>
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Time</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
					</tr>
				</table>
				<p style="margin-top: 16px; color: #666;">Please check the OpsJob logs for more details.</p>
			`, req.Id, req.DeployName, failReason, time.Now().Format(time.DateTime)),
		},
	}

	if err := r.emailChannel.Send(ctx, message); err != nil {
		klog.ErrorS(err, "Failed to send deployment failure email", "id", req.Id)
	} else {
		klog.Infof("Deployment failure email sent to %s for request %d", userEmail, req.Id)
	}
}

// getUserEmail retrieves the email address for a user by userId.
// Returns empty string if user not found or email not set.
func (r *CDOpsJobReconciler) getUserEmail(ctx context.Context, userId string) string {
	if userId == "" {
		return ""
	}

	user := &v1.User{}
	if err := r.Get(ctx, client.ObjectKey{Name: userId}, user); err != nil {
		klog.Warningf("Failed to get user %s for email lookup: %v", userId, err)
		return ""
	}

	return v1.GetAnnotation(user, v1.UserEmailAnnotation)
}
