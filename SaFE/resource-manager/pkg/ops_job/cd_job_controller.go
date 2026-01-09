/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	// Git repository URL for Primus-SaFE
	PrimusSaFERepoURL = "https://github.com/AMD-AGI/Primus-SaFE.git"

	// Container mount path for CD workspace (uses emptyDir, not hostpath)
	ContainerMountPath = "/home/primus-safe-cd"
)

type CDJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
}

// SetupCDJobController initializes and registers the CDJobReconciler with the controller manager.
func SetupCDJobController(mgr manager.Manager) error {
	r := &CDJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup CD Job Controller successfully")
	return nil
}

// handleWorkloadEvent creates an event handler that watches Workload resource events for CD jobs.
func (r *CDJobReconciler) handleWorkloadEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			workload, ok := evt.Object.(*v1.Workload)
			if !ok || !isCDWorkload(workload) {
				return
			}
			r.handleWorkloadEventImpl(ctx, workload)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 || !isCDWorkload(newWorkload) {
				return
			}
			if (!oldWorkload.IsEnd() && newWorkload.IsEnd()) ||
				(!oldWorkload.IsRunning() && newWorkload.IsRunning()) {
				r.handleWorkloadEventImpl(ctx, newWorkload)
			}
		},
	}
}

// isCDWorkload checks if a workload is a CD job workload.
func isCDWorkload(workload *v1.Workload) bool {
	return v1.GetOpsJobId(workload) != "" &&
		v1.GetOpsJobType(workload) == string(v1.OpsJobCDType)
}

// handleWorkloadEventImpl handles workload events by updating the corresponding OpsJob status.
func (r *CDJobReconciler) handleWorkloadEventImpl(ctx context.Context, workload *v1.Workload) {
	var phase v1.OpsJobPhase
	completionMessage := ""

	switch {
	case workload.IsEnd():
		if workload.Status.Phase == v1.WorkloadSucceeded {
			phase = v1.OpsJobSucceeded
		} else {
			phase = v1.OpsJobFailed
		}
		completionMessage = getWorkloadCompletionMessage(workload)
		if completionMessage == "" {
			completionMessage = "CD deployment completed"
		}
	case workload.IsRunning():
		phase = v1.OpsJobRunning
	default:
		phase = v1.OpsJobPending
	}

	jobId := v1.GetOpsJobId(workload)
	err := backoff.Retry(func() error {
		job := &v1.OpsJob{}
		var err error
		if err = r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		switch phase {
		case v1.OpsJobPending, v1.OpsJobRunning:
			err = r.setJobPhase(ctx, job, phase)
		default:
			output := []v1.Parameter{
				{Name: "result", Value: completionMessage},
				{Name: v1.ParameterDeployPhase, Value: workload.GetEnv("DEPLOY_PHASE")},
			}
			err = r.setJobCompleted(ctx, job, phase, completionMessage, output)
		}
		if err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
	if err != nil {
		klog.ErrorS(err, "failed to update CD job status", "jobId", jobId)
	}
}

// Reconcile is the main control loop for CD Job resources.
func (r *CDJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

// cleanupJobRelatedInfo cleans up job-related resources.
func (r *CDJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedResource(ctx, r.Client, job.Name)
}

// observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *CDJobReconciler) observe(_ context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

// filter determines if the job should be processed by this CD job reconciler.
func (r *CDJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobCDType
}

// handle processes the CD job by creating a corresponding workload.
func (r *CDJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.Status.Phase == "" {
		originalJob := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = v1.OpsJobPending
		if err := r.Status().Patch(ctx, job, originalJob); err != nil {
			return ctrlruntime.Result{}, err
		}
		// ensure that job will be reconciled when it is timeout
		return newRequeueAfterResult(job), nil
	}

	// Check if workload already exists
	workload := &v1.Workload{}
	if r.Get(ctx, client.ObjectKey{Name: job.Name}, workload) == nil {
		return ctrlruntime.Result{}, nil
	}

	// Generate CD workload
	var err error
	workload, err = r.generateCDWorkload(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to generate CD workload", "job", job.Name)
		return ctrlruntime.Result{}, err
	}

	if err = r.Create(ctx, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreAlreadyExists(err)
	}
	klog.Infof("Processing CD job %s for workload %s", job.Name, workload.Name)
	return ctrlruntime.Result{}, nil
}

// generateCDWorkload generates a CD workload based on the job specification.
func (r *CDJobReconciler) generateCDWorkload(ctx context.Context, job *v1.OpsJob) (*v1.Workload, error) {
	// Get deployment parameters from job inputs
	componentTags := getParameterValue(job, v1.ParameterComponentTags, "")
	nodeAgentTags := getParameterValue(job, v1.ParameterNodeAgentTags, "")
	envFileConfig := getParameterValue(job, v1.ParameterEnvFileConfig, "")
	deployBranch := getParameterValue(job, v1.ParameterDeployBranch, "")
	hasNodeAgent := getParameterValue(job, v1.ParameterHasNodeAgent, "false") == "true"
	hasCICD := getParameterValue(job, v1.ParameterHasCICD, "false") == "true"
	nodeAgentImage := getParameterValue(job, v1.ParameterNodeAgentImage, "")
	cicdRunnerImage := getParameterValue(job, v1.ParameterCICDRunnerImage, "")
	cicdUnifiedImage := getParameterValue(job, v1.ParameterCICDUnifiedImage, "")

	// Base64 encode the .env file content for safe passing
	envFileBase64 := ""
	if envFileConfig != "" {
		envFileBase64 = base64.StdEncoding.EncodeToString([]byte(envFileConfig))
	}

	// Simple entrypoint: git clone the repo then execute the bootstrap script
	// All configuration is passed via environment variables
	entryPoint := base64.StdEncoding.EncodeToString([]byte(
		`git clone --depth 1 -b "$DEPLOY_BRANCH" "$REPO_URL" "$REPO_DIR" && ` +
			`cd "$REPO_DIR/SaFE/bootstrap" && bash ./cd-deploy.sh`,
	))

	// Query clusters with ClusterControlPlaneLabel to get the control plane cluster ID
	clusterList := &v1.ClusterList{}
	if err := r.Client.List(ctx, clusterList, client.MatchingLabels{
		v1.ClusterControlPlaneLabel: "",
	}); err != nil {
		return nil, fmt.Errorf("failed to list clusters with control-plane label: %w", err)
	}
	if len(clusterList.Items) == 0 {
		return nil, fmt.Errorf("no cluster with control-plane label found")
	}

	// Use the first cluster with control-plane label
	clusterID := clusterList.Items[0].Name
	klog.Infof("Found control plane cluster: %s", clusterID)

	// Create workload with minimal resource requirements (no GPU needed)
	// Uses 'default' workspace with immediate scheduling (similar to preflight jobs)
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: job.Name,
			Labels: map[string]string{
				v1.ClusterIdLabel:   clusterID,
				v1.UserIdLabel:      common.UserSystem,
				v1.OpsJobIdLabel:    job.Name,
				v1.OpsJobTypeLabel:  string(job.Spec.Type),
				v1.DisplayNameLabel: job.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation:    common.UserSystem,
				v1.DescriptionAnnotation: v1.OpsJobKind,
				// Dispatch the workload immediately, skipping the queue (same as preflight)
				v1.WorkloadScheduledAnnotation: timeutil.FormatRFC3339(time.Now().UTC()),
			},
		},
		Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{{
				Replica: 1,
				CPU:     "2",
				Memory:  "4Gi",
			}},
			EntryPoint: entryPoint,
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.JobKind,
			},
			Priority:  common.HighPriorityInt,
			Workspace: corev1.NamespaceDefault, // Use 'default' namespace (same as preflight)
			Image:     commonconfig.GetCDJobImage(),
			Env: map[string]string{
				// Repository configuration
				"REPO_URL":      PrimusSaFERepoURL,
				"REPO_DIR":      ContainerMountPath + "/Primus-SaFE",
				"MOUNT_DIR":     ContainerMountPath,
				"DEPLOY_BRANCH": deployBranch,
				// Deployment parameters
				"COMPONENT_TAGS":        componentTags,
				"NODE_AGENT_TAGS":       nodeAgentTags,
				"ENV_FILE_CONFIG":       envFileBase64, // Base64 encoded
				"HAS_NODE_AGENT":        fmt.Sprintf("%t", hasNodeAgent),
				"HAS_CICD":              fmt.Sprintf("%t", hasCICD),
				"NODE_AGENT_IMAGE":      nodeAgentImage,
				"CICD_RUNNER_IMAGE":     cicdRunnerImage,
				"CICD_UNIFIED_IMAGE":    cicdUnifiedImage,
				"DEPLOYMENT_REQUEST_ID": getParameterValue(job, v1.ParameterDeploymentRequestId, ""),
			},
		},
	}

	if err := controllerutil.SetControllerReference(job, workload, r.Client.Scheme()); err != nil {
		return nil, err
	}

	if job.Spec.TimeoutSecond > 0 {
		workload.Spec.Timeout = pointer.Int(job.Spec.TimeoutSecond)
	} else {
		// Default timeout of 30 minutes for CD jobs
		workload.Spec.Timeout = pointer.Int(1800)
	}

	if job.Spec.TTLSecondsAfterFinished > 0 {
		workload.Spec.TTLSecondsAfterFinished = pointer.Int(job.Spec.TTLSecondsAfterFinished)
	} else {
		// Default TTL of 1 hour
		workload.Spec.TTLSecondsAfterFinished = pointer.Int(3600)
	}

	return workload, nil
}

// getParameterValue retrieves a parameter value from job inputs with a default fallback.
func getParameterValue(job *v1.OpsJob, name, defaultValue string) string {
	param := job.GetParameter(name)
	if param != nil && param.Value != "" {
		return param.Value
	}
	return defaultValue
}
