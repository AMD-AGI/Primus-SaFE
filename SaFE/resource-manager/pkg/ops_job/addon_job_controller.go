/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

const (
	maxMessageLen = 256
)

type AddonJob struct {
	// Store the processing status for each node. key is the admin node name
	nodes map[string]v1.OpsJobPhase
	// List of addon templates associated with the job
	addonTemplates []*v1.AddonTemplate
	// The maximum number of node failures that the system can tolerate during job execution.
	maxFailCount int
	// The number of nodes to process simultaneously during the addon execution
	batchCount int
}

type AddonJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
	// Store all addon jobs, with the key being the job ID.
	allJobs map[string]*AddonJob
}

// SetupAddonJobController initializes and registers the AddonJobReconciler with the controller manager.
func SetupAddonJobController(mgr manager.Manager) error {
	r := &AddonJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
		allJobs: make(map[string]*AddonJob),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Addon Job Controller successfully")
	return nil
}

// handleNodeEvent creates an event handler that watches Node resource updates.
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

// handleNodeRemovedEvent handles node removal events by updating job status and cleaning up faults.
func (r *AddonJobReconciler) handleNodeRemovedEvent(ctx context.Context,
	node *v1.Node, message string, q v1.RequestWorkQueue) {
	jobList, err := r.listJobs(ctx, node.GetSpecCluster(), string(v1.OpsJobAddonType))
	if err != nil {
		return
	}
	for _, job := range jobList {
		ok := r.setNodePhase(job.Name, node.Name, v1.OpsJobFailed)
		if !ok {
			continue
		}
		r.addFailedNodeCondition(ctx, job.Name, node.Name, message)
		r.deleteFault(ctx, node.Name, common.AddonMonitorId)
		q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: job.Name}})
	}
}

// addFailedNodeCondition adds a condition to the job status indicating a node has failed.
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
		if err := r.updateCondition(ctx, job, cond); err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
	if err != nil {
		klog.ErrorS(err, "failed to update job condition", "jobId", jobId)
	}
}

// handleWorkloadEvent creates an event handler that enqueues AddonJob requests when related Workloads change.
func (r *AddonJobReconciler) handleWorkloadEvent() handler.EventHandler {
	enqueue := func(ctx context.Context, q v1.RequestWorkQueue, clusterId string) {
		jobList, err := r.listJobs(ctx, clusterId, string(v1.OpsJobAddonType))
		if err != nil {
			return
		}
		for _, job := range jobList {
			if v1.IsSecurityUpgrade(&job) {
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

// Reconcile is the main control loop for AddonJob resources.
func (r *AddonJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo, r.removeJob}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

// cleanupJobRelatedInfo cleans up job-related resources.
func (r *AddonJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedResource(ctx, r.Client, job.Name)
}

// observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *AddonJobReconciler) observe(ctx context.Context, job *v1.OpsJob) (bool, error) {
	if job.IsEnd() {
		return true, nil
	}
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

// filter determines if the job should be processed by this reconciler.
func (r *AddonJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobAddonType
}

// getNodesToProcess retrieves nodes that need to be processed based on job status and batch count.
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

// handle the main processing logic. Obtain the list of nodes to be processed,
// process the addon installation operation for each node in parallel, and set a timeout.
func (r *AddonJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if r.getJob(job.Name) == nil {
		if err := r.addJob(ctx, job); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	if job.IsPending() {
		if err := r.setJobPhase(ctx, job, v1.OpsJobRunning); err != nil {
			return ctrlruntime.Result{}, err
		}
		// ensure that job will be reconciled when it is timeout
		return newRequeueAfterResult(job), nil
	}
	targetNodes := r.getNodesToProcess(job)
	if len(targetNodes) == 0 {
		return ctrlruntime.Result{}, nil
	}
	cond := metav1.Condition{Type: JobProcessingType, Status: metav1.ConditionTrue,
		Reason: "Processing", Message: string(jsonutils.MarshalSilently(targetNodes)),
	}
	var err error
	if err = r.updateCondition(ctx, job, &cond); err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = r.handleNodes(ctx, job, targetNodes); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{Requeue: true}, nil
}

// handleNodes processes multiple nodes concurrently for the addon job.
func (r *AddonJobReconciler) handleNodes(ctx context.Context, job *v1.OpsJob, nodeNames []string) error {
	var err error
	allUsingNodes := sets.NewSet()
	if v1.IsSecurityUpgrade(job) {
		if allUsingNodes, err = commonnodes.GetUsingNodesOfCluster(ctx, r.Client, v1.GetClusterId(job)); err != nil {
			return err
		}
	}
	count := len(nodeNames)
	ch := make(chan string, count)
	defer close(ch)
	for _, n := range nodeNames {
		ch <- n
	}

	const maxRetry = 10
	waitTime := time.Millisecond * 300
	maxWaitTime := waitTime * maxRetry
	_, err = concurrent.Exec(count, func() error {
		nodeName := <-ch
		err = backoff.Retry(func() error {
			ok, innerErr := r.handleNode(ctx, job, nodeName, allUsingNodes)
			if ok {
				r.setNodePhase(job.Name, nodeName, v1.OpsJobSucceeded)
			}
			return innerErr

		}, maxWaitTime, waitTime)
		if err != nil {
			klog.ErrorS(err, "failed to handle opsJob", "job", job.Name, "node", nodeName)
			if r.setNodePhase(job.Name, nodeName, v1.OpsJobFailed) {
				r.addFailedNodeCondition(ctx, job.Name, nodeName, err.Error())
			}
			err = nil
		}
		return err
	})
	return err
}

// handleNode processes a single node for the addon job.
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
	if !adminNode.IsMachineReady() || commonfaults.HasTaintKey(adminNode.Status.Taints, key) {
		return false, fmt.Errorf("the node is not ready")
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
	defer sshClient.Close()

	isTaskRunning := false
	for _, addOn := range addonJob.addonTemplates {
		if !isMatchGpuChip(string(addOn.Spec.GpuChip), adminNode) {
			continue
		}
		if err = executeCommand(sshClient, addOn.Name, addOn.Spec.Action, addOn.Spec.Required); err != nil {
			return false, err
		}
		isTaskRunning = true
	}
	scriptParams := job.GetParameters(v1.ParameterScript)
	for _, p := range scriptParams {
		if err = executeCommand(sshClient, "", p.Value, true); err != nil {
			return false, err
		}
		isTaskRunning = true
	}

	// If the addon specified by node.template is installed on the node, save the operation result.
	// Subsequent operations can then trigger the preflight check.
	if err = r.updateNodeTemplatePhase(ctx, job, adminNode, isTaskRunning); err != nil {
		return false, err
	}
	if err = r.deleteFault(ctx, nodeName, common.AddonMonitorId); err != nil {
		return false, err
	}
	if !isTaskRunning {
		return false, fmt.Errorf("do nothing")
	}
	klog.Infof("Processing addon job %s on node %s", job.Name, nodeName)
	return true, nil
}

// updateNodeTemplatePhase updates the node template installation status annotation.
func (r *AddonJobReconciler) updateNodeTemplatePhase(ctx context.Context, job *v1.OpsJob, adminNode *v1.Node, isOk bool) error {
	if job.GetParameter(v1.ParameterNodeTemplate) == nil {
		return nil
	}
	originalNode := client.MergeFrom(adminNode.DeepCopy())
	if !v1.SetAnnotation(adminNode, v1.NodeTemplateInstalledAnnotation, strconv.FormatBool(isOk)) {
		return nil
	}
	if err := r.Patch(ctx, adminNode, originalNode); err != nil {
		return err
	}
	return nil
}

// executeCommand executes the addon action on the node via SSH.
func executeCommand(sshClient *ssh.Client, name, command string, isRequired bool) error {
	cmd := fmt.Sprintf(
		`echo '%s' | /usr/bin/base64 -d | sudo /bin/bash`,
		command,
	)
	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Capture combined stdout and stderr output
	output, err := session.CombinedOutput(cmd)
	if err == nil {
		return nil
	}
	var exitError *ssh.ExitError
	if errors.As(err, &exitError) {
		message := exitError.Error()
		message = normalizeErrorMessage(message)
		outputStr := normalizeErrorMessage(string(output))
		klog.ErrorS(err, "failed to execute command", "addon", name,
			"exit_status", exitError.ExitStatus(), "output", outputStr)
		err = commonerrors.NewInternalError(fmt.Sprintf("addon: %s, message: %s, exit_status: %d, output: %s",
			name, message, exitError.ExitStatus(), outputStr))
	} else {
		klog.ErrorS(err, "failed to execute command", "addon", name, "output", string(output))
	}
	if !isRequired {
		return nil
	}
	return err
}

// addJob initializes and adds a new addon job to the reconciler.
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
	addonJob := &AddonJob{
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
		r.allJobs[job.Name] = addonJob
	}
	return nil
}

// removeJob removes an addon job from the reconciler.
func (r *AddonJobReconciler) removeJob(_ context.Context, job *v1.OpsJob) error {
	r.Lock()
	defer r.Unlock()
	delete(r.allJobs, job.Name)
	return nil
}

// getJob retrieves an addon job by job ID.
func (r *AddonJobReconciler) getJob(jobId string) *AddonJob {
	r.RLock()
	defer r.RUnlock()
	job, ok := r.allJobs[jobId]
	if ok {
		return job
	}
	return nil
}

// setNodePhase updates the phase of a node in the addon job.
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

// getJobPhase calculates the overall job phase based on node statuses.
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

// getInputAddonTemplates retrieves addon templates specified in the job parameters.
func (r *AddonJobReconciler) getInputAddonTemplates(ctx context.Context, job *v1.OpsJob) ([]*v1.AddonTemplate, error) {
	params := job.GetParameters(v1.ParameterAddonTemplate)
	if len(params) == 0 {
		return nil, nil
	}
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
		results = append(results, addonTemplate)
	}
	return results, nil
}

// isMatchGpuChip checks if the GPU chip type matches the node's GPU.
func isMatchGpuChip(chip string, adminNode *v1.Node) bool {
	switch chip {
	case string(v1.AmdGpuChip):
		return v1.GetGpuResourceName(adminNode) == common.AmdGpu
	case string(v1.NvidiaGpuChip):
		return v1.GetGpuResourceName(adminNode) == common.NvidiaGpu
	case "":
		return true
	default:
		return false
	}
}

// normalizeErrorMessage normalizes error messages by removing extra whitespace and truncating length.
func normalizeErrorMessage(message string) string {
	if message == "" {
		return ""
	}
	if len(message) > maxMessageLen {
		message = message[:maxMessageLen]
	}
	message = strings.Replace(message, "\n", " ", -1)
	message = strings.Replace(message, "\t", " ", -1)
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(message, " ")
}
