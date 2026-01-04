package reconciler

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// EphemeralRunnerReconciler watches EphemeralRunner resources and creates workflow run records
type EphemeralRunnerReconciler struct {
	client        *clientsets.K8SClientSet
	dynamicClient dynamic.Interface
}

// NewEphemeralRunnerReconciler creates a new reconciler
func NewEphemeralRunnerReconciler() *EphemeralRunnerReconciler {
	return &EphemeralRunnerReconciler{}
}

// Init initializes the reconciler with required clients
func (r *EphemeralRunnerReconciler) Init(ctx context.Context) error {
	clusterManager := clientsets.GetClusterManager()
	currentCluster := clusterManager.GetCurrentClusterClients()
	if currentCluster.K8SClientSet == nil {
		return fmt.Errorf("K8S client not initialized in ClusterManager")
	}
	r.client = currentCluster.K8SClientSet

	if r.client.Dynamic == nil {
		return fmt.Errorf("dynamic client not initialized in K8SClientSet")
	}
	r.dynamicClient = r.client.Dynamic

	log.Info("EphemeralRunnerReconciler initialized")
	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *EphemeralRunnerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create an unstructured object for EphemeralRunner
	erExample := &unstructured.Unstructured{}
	erExample.SetGroupVersionKind(types.EphemeralRunnerGVK)

	// Watch all EphemeralRunner resources
	return ctrl.NewControllerManagedBy(mgr).
		Named("ephemeral-runner-controller").
		For(erExample).
		Complete(r)
}

// Reconcile handles EphemeralRunner events
func (r *EphemeralRunnerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic recovered: %v", rec)
			log.Errorf("Panic in EphemeralRunnerReconciler for %s/%s: %v\nStack trace:\n%s",
				req.Namespace, req.Name, rec, string(debug.Stack()))
		}
	}()

	log.Debugf("EphemeralRunnerReconciler: reconciling %s/%s", req.Namespace, req.Name)

	// Get the EphemeralRunner
	obj, err := r.dynamicClient.Resource(types.EphemeralRunnerGVR).
		Namespace(req.Namespace).
		Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Resource was deleted, no action needed
			log.Debugf("EphemeralRunnerReconciler: %s/%s not found, skipping", req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		log.Errorf("EphemeralRunnerReconciler: failed to get %s/%s: %v", req.Namespace, req.Name, err)
		return ctrl.Result{}, err
	}

	// Parse the object
	info := types.ParseEphemeralRunner(obj)

	// Only process completed runners
	if !info.IsCompleted {
		log.Debugf("EphemeralRunnerReconciler: %s/%s not completed yet (phase: %s), skipping",
			req.Namespace, req.Name, info.Phase)
		return ctrl.Result{}, nil
	}

	// Process the completed runner
	if err := r.processCompletedRunner(ctx, info); err != nil {
		log.Errorf("EphemeralRunnerReconciler: failed to process %s/%s: %v",
			req.Namespace, req.Name, err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	log.Infof("EphemeralRunnerReconciler: successfully processed %s/%s (workflow: %s, run: %d)",
		req.Namespace, req.Name, info.WorkflowName, info.GithubRunID)

	return ctrl.Result{}, nil
}

// processCompletedRunner processes a completed EphemeralRunner
func (r *EphemeralRunnerReconciler) processCompletedRunner(ctx context.Context, info *types.EphemeralRunnerInfo) error {
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runFacade := database.GetFacade().GetGithubWorkflowRun()

	// Find the runner set for this ephemeral runner
	runnerSet, err := runnerSetFacade.GetByNamespaceName(ctx, info.Namespace, info.RunnerSetName)
	if err != nil {
		return fmt.Errorf("failed to get runner set %s/%s: %w", info.Namespace, info.RunnerSetName, err)
	}

	if runnerSet == nil {
		log.Warnf("EphemeralRunnerReconciler: runner set not found for %s/%s, skipping", info.Namespace, info.RunnerSetName)
		return nil
	}

	// Check if we already have a run record for this workload
	existingRun, err := runFacade.GetByRunnerSetAndWorkload(ctx, runnerSet.ID, info.UID)
	if err != nil {
		return fmt.Errorf("failed to check existing run for %s: %w", info.UID, err)
	}

	if existingRun != nil {
		log.Debugf("EphemeralRunnerReconciler: run record already exists for %s/%s", info.Namespace, info.Name)
		return nil
	}

	// Optionally find matching config for additional metadata
	var configID int64
	configFacade := database.GetFacade().GetGithubWorkflowConfig()
	configs, err := configFacade.ListByRunnerSetID(ctx, runnerSet.ID)
	if err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to list configs for runner set %d: %v", runnerSet.ID, err)
	} else if len(configs) > 0 {
		// Use the first matching enabled config
		for _, config := range configs {
			if config.Enabled && r.matchesConfig(info, config) {
				configID = config.ID
				break
			}
		}
	}

	// Create a new run record - always create, regardless of config existence
	run := &model.GithubWorkflowRuns{
		RunnerSetID:         runnerSet.ID,
		RunnerSetName:       runnerSet.Name,
		RunnerSetNamespace:  runnerSet.Namespace,
		ConfigID:            configID, // Optional, 0 if no matching config
		WorkloadUID:         info.UID,
		WorkloadName:        info.Name,
		WorkloadNamespace:   info.Namespace,
		GithubRunID:         info.GithubRunID,
		GithubRunNumber:     int32(info.GithubRunNumber),
		GithubJobID:         info.GithubJobID,
		HeadSha:             info.HeadSHA,
		HeadBranch:          info.Branch,
		WorkflowName:        info.WorkflowName,
		Status:              database.WorkflowRunStatusPending,
		TriggerSource:       database.WorkflowRunTriggerRealtime,
		WorkloadStartedAt:   info.CreationTimestamp.Time,
		WorkloadCompletedAt: info.CompletionTime.Time,
	}

	// If completion time is zero, use now
	if run.WorkloadCompletedAt.IsZero() {
		run.WorkloadCompletedAt = time.Now()
	}

	if err := runFacade.Create(ctx, run); err != nil {
		return fmt.Errorf("failed to create run record for %s: %w", info.Name, err)
	}

	log.Infof("EphemeralRunnerReconciler: created run record for %s/%s (runner_set: %s, config_id: %d, run_id: %d)",
		info.Namespace, info.Name, runnerSet.Name, configID, run.ID)

	return nil
}

// matchesConfig checks if an EphemeralRunner matches a workflow config
func (r *EphemeralRunnerReconciler) matchesConfig(info *types.EphemeralRunnerInfo, config *model.GithubWorkflowConfigs) bool {
	// Check namespace matches
	if config.RunnerSetNamespace != "" && config.RunnerSetNamespace != info.Namespace {
		return false
	}

	// Check runner set name if specified
	if config.RunnerSetName != "" && config.RunnerSetName != info.RunnerSetName {
		return false
	}

	// Check workflow filter if set
	if config.WorkflowFilter != "" && info.WorkflowName != "" && info.WorkflowName != config.WorkflowFilter {
		return false
	}

	// Check branch filter if set
	if config.BranchFilter != "" && info.Branch != "" && info.Branch != config.BranchFilter {
		return false
	}

	return true
}

