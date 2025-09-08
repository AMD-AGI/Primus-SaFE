/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
)

type PreflightJob struct {
	// store the processing status for each node. key is the admin node name
	nodes map[string]v1.OpsJobPhase
}

type PreflightJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
	// key is job id
	allJobs map[string]*PreflightJob
}

func SetupPreflightJobController(mgr manager.Manager) error {
	if commonconfig.GetPreflightImage() == "" {
		return nil
	}
	r := &PreflightJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
		allJobs: make(map[string]*PreflightJob),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onJobRunning()))).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Preflight Job Controller successfully")
	return nil
}

func (r *PreflightJobReconciler) handleWorkloadEvent() handler.EventHandler {
	return handler.Funcs{
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 {
				return
			}
			opsJobId := v1.GetOpsJobId(newWorkload)
			if opsJobId == "" || v1.GetOpsJobType(newWorkload) != string(v1.OpsJobPreflightType) {
				return
			}
			if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
				if r.handleWorkloadEventImpl(ctx, newWorkload) {
					q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: opsJobId}})
				}
			}
		},
	}
}

func (r *PreflightJobReconciler) handleWorkloadEventImpl(ctx context.Context, workload *v1.Workload) bool {
	nodeId, _ := workload.Spec.CustomerLabels[common.K8sHostNameLabel]
	if nodeId == "" {
		return false
	}
	var phase v1.OpsJobPhase
	if workload.Status.Phase == v1.WorkloadSucceeded {
		phase = v1.OpsJobSucceeded
	} else {
		phase = v1.OpsJobFailed
	}
	opsJobId := v1.GetOpsJobId(workload)
	if !r.setNodePhase(opsJobId, nodeId, phase) {
		return false
	}
	r.addJobCondition(ctx, opsJobId, nodeId, workload, phase)
	r.deleteFault(ctx, nodeId, common.PreflightMonitorId)
	return true
}

func (r *PreflightJobReconciler) addJobCondition(ctx context.Context,
	jobId, nodeId string, workload *v1.Workload, phase v1.OpsJobPhase) {
	message := getWorkloadMessage(workload)
	if message == "" {
		message = "unknown"
	}
	condition := &metav1.Condition{
		Type:               nodeId,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Message:            message,
	}
	if phase == v1.OpsJobFailed {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "PreflightFailed"
	} else {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "PreflightSucceeded"
	}
	err := backoff.Retry(func() error {
		job := &v1.OpsJob{}
		if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := r.updateJobCondition(ctx, job, condition); err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
	if err != nil {
		klog.ErrorS(err, "failed to update job condition", "jobId", jobId)
	}
}

func (r *PreflightJobReconciler) handleNodeEvent() handler.EventHandler {
	return handler.Funcs{
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 || newNode.GetSpecCluster() == "" {
				return
			}
			if v1.IsNodeTemplateInstalled(oldNode) || !v1.IsNodeTemplateInstalled(newNode) {
				return
			}
			jobList, err := r.listOpsJobs(ctx, newNode.GetSpecCluster(), string(v1.OpsJobPreflightType))
			if err != nil {
				return
			}
			for _, job := range jobList {
				if job.HasParameter(v1.ParameterNode, newNode.Name) {
					q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: job.Name}})
				}
			}
		},
	}
}

func (r *PreflightJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo, r.removeJob}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

func (r *PreflightJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedInfo(ctx, r.Client, job.Name)
}

// Observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *PreflightJobReconciler) observe(ctx context.Context, job *v1.OpsJob) (bool, error) {
	if job.IsEnd() {
		return true, nil
	}
	phase, message := r.getJobPhase(job.Name)
	switch phase {
	case v1.OpsJobPending, "":
		return false, nil
	case v1.OpsJobRunning:
		nodes, err := r.getNodesToProcess(ctx, job)
		return len(nodes) == 0, err
	case v1.OpsJobFailed, v1.OpsJobSucceeded:
		if err := r.setJobCompleted(ctx, job, phase, message, nil); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *PreflightJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobPreflightType
}

func (r *PreflightJobReconciler) getNodesToProcess(ctx context.Context, job *v1.OpsJob) ([]*v1.Node, error) {
	r.RLock()
	preflightJob, ok := r.allJobs[job.Name]
	if !ok {
		r.RUnlock()
		return nil, nil
	}
	allPendingNodes := make([]string, 0, len(preflightJob.nodes))
	for key, val := range preflightJob.nodes {
		if val != v1.OpsJobPending {
			continue
		}
		allPendingNodes = append(allPendingNodes, key)
	}
	r.RUnlock()

	results := make([]*v1.Node, 0, len(allPendingNodes))
	for _, n := range allPendingNodes {
		node, err := r.getAdminNode(ctx, n)
		if err != nil {
			return nil, err
		}
		if !v1.IsNodeTemplateInstalled(node) {
			continue
		}
		results = append(results, node)
	}
	return results, nil
}

func (r *PreflightJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if r.getJob(job.Name) == nil {
		if err := r.addJob(ctx, job); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	if job.IsPending() {
		return r.setJobRunning(ctx, job)
	}

	targetNodes, err := r.getNodesToProcess(ctx, job)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	totalNum := len(targetNodes)
	if totalNum == 0 {
		return ctrlruntime.Result{}, nil
	}
	ch := make(chan int, totalNum)
	for i := range totalNum {
		ch <- i
	}
	_, err = concurrent.Exec(totalNum, func() error {
		i := <-ch
		workload, err := r.genPreflightWorkload(ctx, job, targetNodes[i])
		if err != nil {
			return err
		}
		if err = r.createFault(ctx, job, targetNodes[i],
			common.PreflightMonitorId, "preflight check"); err != nil {
			return err
		}
		if err = r.Create(ctx, workload); err != nil {
			return client.IgnoreAlreadyExists(err)
		}
		r.setNodePhase(job.Name, targetNodes[i].Name, v1.OpsJobRunning)
		return nil
	})
	return ctrlruntime.Result{}, err
}

func (r *PreflightJobReconciler) addJob(ctx context.Context, job *v1.OpsJob) error {
	inputNodes, err := r.getInputNodes(ctx, job)
	if err != nil {
		return err
	}
	nodes := make(map[string]v1.OpsJobPhase)
	for _, n := range inputNodes {
		nodes[n.Name] = v1.OpsJobPending
	}
	preflightJob := &PreflightJob{
		nodes: nodes,
	}

	r.Lock()
	defer r.Unlock()
	if _, ok := r.allJobs[job.Name]; !ok {
		r.allJobs[job.Name] = preflightJob
	}
	return nil
}

func (r *PreflightJobReconciler) getJob(jobId string) *PreflightJob {
	r.RLock()
	defer r.RUnlock()
	job, ok := r.allJobs[jobId]
	if ok {
		return job
	}
	return nil
}

func (r *PreflightJobReconciler) removeJob(_ context.Context, job *v1.OpsJob) error {
	r.Lock()
	defer r.Unlock()
	delete(r.allJobs, job.Name)
	return nil
}

func (r *PreflightJobReconciler) setNodePhase(jobId, nodeId string, phase v1.OpsJobPhase) bool {
	r.Lock()
	defer r.Unlock()
	addonJob, ok := r.allJobs[jobId]
	if !ok {
		return false
	}
	oldPhase, ok := addonJob.nodes[nodeId]
	if !ok {
		return false
	}
	// The job on the node has finished.
	if oldPhase == v1.OpsJobFailed || oldPhase == v1.OpsJobSucceeded {
		return false
	}
	addonJob.nodes[nodeId] = phase
	return true
}

func (r *PreflightJobReconciler) getJobPhase(jobId string) (v1.OpsJobPhase, string) {
	r.RLock()
	defer r.RUnlock()
	job, ok := r.allJobs[jobId]
	if !ok {
		return v1.OpsJobPending, ""
	}
	totalFailCount := 0
	totalSuccessCount := 0
	for _, p := range job.nodes {
		if p == v1.OpsJobFailed {
			totalFailCount++
		} else if p == v1.OpsJobSucceeded {
			totalSuccessCount++
		}
	}
	if totalFailCount+totalSuccessCount >= len(job.nodes) {
		phase := v1.OpsJobSucceeded
		if totalFailCount > 0 {
			phase = v1.OpsJobFailed
		}
		return phase, fmt.Sprintf("success: %d, fail: %d", totalSuccessCount, totalFailCount)
	}
	return v1.OpsJobRunning, ""
}

func (r *PreflightJobReconciler) genPreflightWorkload(ctx context.Context,
	job *v1.OpsJob, adminNode *v1.Node) (*v1.Workload, error) {
	res, err := r.genMaxResource(ctx, adminNode)
	if err != nil {
		return nil, err
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: job.Name + "-" + adminNode.Name,
			Labels: map[string]string{
				v1.ClusterIdLabel:    job.Spec.Cluster,
				v1.NodeFlavorIdLabel: v1.GetNodeFlavorId(adminNode),
				v1.OpsJobIdLabel:     job.Name,
				v1.OpsJobTypeLabel:   string(job.Spec.Type),
				v1.DisplayNameLabel:  job.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: common.UserSystem,
				// Dispatch the workload immediately, skipping the queue.
				v1.WorkloadScheduledAnnotation: time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: v1.WorkloadSpec{
			EntryPoint: fmt.Sprintf("GPU_PRODUCT=%s bash run.sh", v1.GetGpuProductName(job)),
			GroupVersionKind: v1.GroupVersionKind{
				Version: v1.SchemeGroupVersion.Version,
				Kind:    common.JobKind,
			},
			IsTolerateAll: true,
			Priority:      common.HighPriorityInt,
			CustomerLabels: map[string]string{
				common.K8sHostNameLabel: adminNode.Name,
			},
			Workspace: v1.GetWorkspaceId(adminNode),
			Image:     commonconfig.GetPreflightImage(),
			Env:       job.Spec.Env,
		},
	}

	workload.Spec.Resource = *res
	workload.Spec.Resource.Replica = 1
	workload.Spec.Workspace = corev1.NamespaceDefault
	if job.Spec.TimeoutSecond > 0 {
		workload.Spec.Timeout = pointer.Int(job.Spec.TimeoutSecond)
	}
	return workload, nil
}
