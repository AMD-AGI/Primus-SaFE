/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
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
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

type AddonJob struct {
	// store the processing status for each node. key is the admin node name
	nodes map[string]v1.OpsJobPhase
	// list of addon templates associated with the job
	addonTemplates []*v1.AddonTemplate
	// the maximum number of node failures that the system can tolerate during job execution.
	maxFailCount int
	// the number of nodes to process simultaneously during the addon execution
	batchCount int
}

type AddonJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
	// key is job id
	allJobs map[string]*AddonJob
}

func SetupAddonJobController(mgr manager.Manager) error {
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
		allJobs: make(map[string]*AddonJob),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, jobPhaseChangedPredicate()))).
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Addon Job Controller successfully")
	return nil
}

func (r *AddonJobReconciler) handleNodeEvent() handler.EventHandler {
	return handler.Funcs{
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 || oldNode.GetSpecCluster() == "" {
				return
			}
			if newNode.GetSpecCluster() == "" {
				r.handleNodeRemovedEvent(ctx, oldNode, "The node is unmanaged", q)
			} else if oldNode.GetDeletionTimestamp().IsZero() && !newNode.GetDeletionTimestamp().IsZero() {
				r.handleNodeRemovedEvent(ctx, newNode, "The node is deleted", q)
			}
		},
	}
}

func (r *AddonJobReconciler) handleNodeRemovedEvent(ctx context.Context,
	node *v1.Node, message string, q v1.RequestWorkQueue) {
	jobList, err := r.listAddonJobs(ctx, node.GetSpecCluster())
	if err != nil {
		return
	}
	for _, job := range jobList {
		if job.IsEnd() {
			continue
		}
		ok := r.setNodePhase(job.Name, node.Name, v1.OpsJobFailed)
		if !ok {
			continue
		}
		r.addFailedNodeCondition(ctx, job.Name, node.Name, message)
		r.deleteFault(ctx, node.Name, common.AddonMonitorId)
		q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: job.Name}})
	}
}

func (r *AddonJobReconciler) addFailedNodeCondition(ctx context.Context, jobId, nodeName, message string) {
	cond := &metav1.Condition{
		Type:               nodeName,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "AddonFailed",
		Message:            message,
	}
	err := backoff.Retry(func() error {
		job := &v1.OpsJob{}
		if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := r.updateJobCondition(ctx, job, cond); err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
	if err != nil {
		klog.ErrorS(err, "failed to update job condition", "jobId", jobId)
	}
}

func (r *AddonJobReconciler) handleWorkloadEvent() handler.EventHandler {
	enqueue := func(ctx context.Context, q v1.RequestWorkQueue, clusterId string) {
		jobList, err := r.listAddonJobs(ctx, clusterId)
		if err != nil {
			return
		}
		for _, job := range jobList {
			if v1.IsSecurityUpgrade(&job) && !job.IsEnd() {
				q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: job.Name}})
			}
		}
	}
	return handler.Funcs{
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 {
				return
			}
			if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
				enqueue(ctx, q, v1.GetClusterId(newWorkload))
			}
		},
	}
}

func (r *AddonJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo, r.removeJob}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

func (r *AddonJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedInfo(ctx, r.Client, job.Name)
}

// Observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *AddonJobReconciler) observe(ctx context.Context, job *v1.OpsJob) (bool, error) {
	phase, message := r.getJobPhase(job.Name)
	switch phase {
	case v1.OpsJobPending, "":
		return false, nil
	case v1.OpsJobRunning:
		nodes := r.getNodesToProcess(job)
		return len(nodes) == 0, nil
	case v1.OpsJobFailed, v1.OpsJobSucceeded:
		if err := r.setJobCompleted(ctx, job, phase, message, nil); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *AddonJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobAddonType
}

func (r *AddonJobReconciler) getNodesToProcess(job *v1.OpsJob) []string {
	r.RLock()
	defer r.RUnlock()
	addonJob, ok := r.allJobs[job.Name]
	if !ok {
		return nil
	}

	runningCount := 0
	var allPendingNodes []string
	for key, val := range addonJob.nodes {
		if val == v1.OpsJobRunning {
			runningCount++
			if runningCount >= addonJob.batchCount {
				return nil
			}
		} else if val == v1.OpsJobPending || val == "" {
			allPendingNodes = append(allPendingNodes, key)
		}
	}
	sort.Strings(allPendingNodes)
	return slice.Copy(allPendingNodes, addonJob.batchCount-runningCount)
}

func (r *AddonJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if r.getJob(job.Name) == nil {
		if err := r.addJob(ctx, job); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	if job.IsPending() {
		return r.setJobRunning(ctx, job)
	}
	targetNodes := r.getNodesToProcess(job)
	if len(targetNodes) == 0 {
		return ctrlruntime.Result{}, nil
	}
	cond := metav1.Condition{Type: JobProcessingType, Status: metav1.ConditionTrue,
		Reason: "Processing", Message: string(jsonutils.MarshalSilently(targetNodes)),
	}
	var err error
	if err = r.updateJobCondition(ctx, job, &cond); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, r.handleNodes(ctx, job, targetNodes)
}

func (r *AddonJobReconciler) handleNodes(ctx context.Context, job *v1.OpsJob, nodeNames []string) error {
	var err error
	allUsingNodes := sets.NewSet()
	if v1.IsSecurityUpgrade(job) {
		if allUsingNodes, err = commonnodes.GetUsingNodesOfCluster(ctx, r.Client, job.Spec.Cluster); err != nil {
			return err
		}
	}
	count := len(nodeNames)
	ch := make(chan string, count)
	for _, n := range nodeNames {
		ch <- n
	}

	_, err = concurrent.Exec(count, func() error {
		nodeName := <-ch
		ok, innerErr := r.handleNode(ctx, job, nodeName, allUsingNodes)
		if innerErr != nil {
			klog.ErrorS(innerErr, "fail to handle opsjob", "jod", job.Name, "node", nodeName)
			if utils.IsNonRetryableError(innerErr) {
				if r.setNodePhase(job.Name, nodeName, v1.OpsJobFailed) {
					r.addFailedNodeCondition(ctx, job.Name, nodeName, innerErr.Error())
				}
				innerErr = nil
			}
		} else if ok {
			r.setNodePhase(job.Name, nodeName, v1.OpsJobSucceeded)
		}
		return innerErr
	})
	return err
}

func (r *AddonJobReconciler) handleNode(ctx context.Context,
	job *v1.OpsJob, nodeName string, allUsingNodes sets.Set) (bool, error) {
	addonJob := r.getJob(job.Name)
	if addonJob == nil {
		return false, commonerrors.NewInternalError(fmt.Sprintf("the job(%s) is not found", job.Name))
	}
	adminNode, err := r.getAdminNode(ctx, nodeName)
	if err != nil {
		return false, err
	}
	key := commonfaults.GenerateTaintKey(resource.NodeNotReady)
	if !adminNode.IsAvailable(true) || commonfaults.HasTaintKey(adminNode.Status.Taints, key) {
		return false, commonerrors.NewInternalError("the node is unavailable")
	}
	if err = r.createFault(ctx, job, adminNode, common.AddonMonitorId, "upgrade Addon"); err != nil {
		return false, err
	}
	// This node is currently being used by another workload.
	// Please retry later, but first apply a taint(via fault).
	if allUsingNodes.Has(nodeName) {
		return false, nil
	}
	sshClient, err := utils.GetSSHClient(ctx, r.Client, adminNode)
	if err != nil {
		return false, err
	}
	for _, addOn := range addonJob.addonTemplates {
		file := fmt.Sprintf("/tmp/%s.sh", addOn.Name)
		cmd := fmt.Sprintf("sudo echo \"%s\" > %s && bash %s >/dev/null 2>&1; code=$?; rm -f %s; exit $code",
			addOn.Spec.Action, file, file, file)
		var session *ssh.Session
		session, err = sshClient.NewSession()
		if err != nil {
			return false, err
		}
		if err = session.Run(cmd); err != nil {
			var exitError *ssh.ExitError
			if errors.As(err, &exitError) {
				err = commonerrors.NewInternalError(fmt.Sprintf("message: %s, code: %d",
					exitError.Error(), exitError.ExitStatus()))
				break
			}
			return false, err
		}
	}
	r.deleteFault(ctx, nodeName, common.AddonMonitorId)
	return true, err
}

func (r *AddonJobReconciler) addJob(ctx context.Context, job *v1.OpsJob) error {
	inputNodes, err := r.getInputNodes(ctx, job)
	if err != nil {
		return err
	}
	inputAddonTemplates, err := r.getInputAddonTemplates(ctx, job)
	if err != nil {
		return err
	}

	nodes := make(map[string]v1.OpsJobPhase)
	for _, n := range inputNodes {
		nodes[n.Name] = v1.OpsJobPending
	}
	addonJob := AddonJob{
		nodes:          nodes,
		addonTemplates: inputAddonTemplates,
	}
	failRatio := float64(1) - v1.GetOpsJobAvailRatio(job)
	if addonJob.maxFailCount = int(float64(len(nodes)) * failRatio); addonJob.maxFailCount <= 0 {
		addonJob.maxFailCount = 1
	}
	addonJob.batchCount = v1.GetOpsJobBatchCount(job)
	if addonJob.batchCount == 0 {
		addonJob.batchCount = 1
	} else if addonJob.batchCount > len(nodes) {
		addonJob.batchCount = len(nodes)
	}

	r.Lock()
	defer r.Unlock()
	if _, ok := r.allJobs[job.Name]; !ok {
		r.allJobs[job.Name] = &addonJob
	}
	return nil
}

func (r *AddonJobReconciler) removeJob(_ context.Context, job *v1.OpsJob) error {
	r.Lock()
	defer r.Unlock()
	delete(r.allJobs, job.Name)
	return nil
}

func (r *AddonJobReconciler) getJob(jobId string) *AddonJob {
	r.RLock()
	defer r.RUnlock()
	job, ok := r.allJobs[jobId]
	if ok {
		return job
	}
	return nil
}

func (r *AddonJobReconciler) setNodePhase(jobId, nodeId string, phase v1.OpsJobPhase) bool {
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
	klog.Infof("update node status for addon job, job.id: %s, node.id: %s, phase: %s", jobId, nodeId, phase)
	return true
}

func (r *AddonJobReconciler) getJobPhase(jobId string) (v1.OpsJobPhase, string) {
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
	if totalFailCount >= job.maxFailCount {
		return v1.OpsJobFailed, fmt.Sprintf("The number of failures has reached the threshold(%d)", job.maxFailCount)
	} else if totalFailCount+totalSuccessCount >= len(job.nodes) {
		return v1.OpsJobSucceeded, fmt.Sprintf("success: %d, fail: %d", totalSuccessCount, totalFailCount)
	}
	return v1.OpsJobRunning, ""
}

func (r *AddonJobReconciler) getInputAddonTemplates(ctx context.Context, job *v1.OpsJob) ([]*v1.AddonTemplate, error) {
	params := job.GetParameters(v1.ParameterAddonTemplate)
	results := make([]*v1.AddonTemplate, 0, len(params))
	for i := range params {
		addonTemplate := &v1.AddonTemplate{}
		err := r.Get(ctx, client.ObjectKey{Name: params[i].Value}, addonTemplate)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if addonTemplate.Spec.Action == "" {
			continue
		}
		addonTemplate.Spec.Action = stringutil.Base64Decode(addonTemplate.Spec.Action)
		results = append(results, addonTemplate)
	}
	if len(results) == 0 {
		return nil, commonerrors.NewBadRequest("no addontemplates are found")
	}
	return results, nil
}

func (r *AddonJobReconciler) listAddonJobs(ctx context.Context, clusterId string) ([]v1.OpsJob, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: clusterId,
		v1.OpsJobTypeLabel: string(v1.OpsJobAddonType)})
	jobList := &v1.OpsJobList{}
	if err := r.List(ctx, jobList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	return jobList.Items, nil
}
