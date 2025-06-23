/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

type AddonNodePhase int

const (
	AddonNodePending   AddonNodePhase = 0
	AddonNodeRunning   AddonNodePhase = 1
	AddonNodeSucceeded AddonNodePhase = 2
	AddonNodeFailed    AddonNodePhase = 3
)

type AddonJob struct {
	// store the processing status for each node. key is the node name
	nodePhases map[string]AddonNodePhase
	// the maximum number of node failures that the system can tolerate during job execution.
	maxFailCount int
	// the number of nodes to process simultaneously during the addon execution
	batchCount int
}

type AddonJobReconciler struct {
	client.Client
	sync.RWMutex
	// key is job id
	allJobs map[string]*AddonJob
}

func SetupAddonJobController(mgr manager.Manager) error {
	r := &AddonJobReconciler{
		Client:  mgr.GetClient(),
		allJobs: make(map[string]*AddonJob),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, r.caredChangePredicate()))).
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Addon Job Controller successfully")
	return nil
}

func (r *AddonJobReconciler) caredChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJob, ok1 := e.ObjectOld.(*v1.OpsJob)
			newJob, ok2 := e.ObjectNew.(*v1.OpsJob)
			if !ok1 || !ok2 {
				return false
			}
			if oldJob.IsPending() && !newJob.IsPending() {
				return true
			}
			return false
		},
	}
}

func (r *AddonJobReconciler) isConcernedJob(jobType string) bool {
	return v1.OpsJobType(jobType) == v1.OpsJobAddonType
}

func (r *AddonJobReconciler) handleNodeEvent() handler.EventHandler {
	filter := func(n *v1.Node) bool {
		return v1.GetOpsJobId(n) == "" || !r.isConcernedJob(v1.GetOpsJobType(n))
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			n, ok := evt.Object.(*v1.Node)
			if !ok || filter(n) {
				return
			}
			phase, message := getAddonNodePhase(n)
			if isAddonNodeEnd(phase) {
				r.handleNodeEventImpl(ctx, n, phase, message, q)
			}
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 || filter(newNode) {
				return
			}
			if oldNode.GetSpecCluster() != "" && newNode.GetSpecCluster() == "" {
				r.handleNodeEventImpl(ctx, newNode, AddonNodeFailed, "The node is unmanaged", q)
			} else {
				oldPhase, _ := getAddonNodePhase(oldNode)
				newPhase, message := getAddonNodePhase(newNode)
				if !isAddonNodeEnd(oldPhase) && isAddonNodeEnd(newPhase) ||
					(isAddonNodeEnd(oldPhase) && isAddonNodeEnd(newPhase) && oldPhase != newPhase) {
					r.handleNodeEventImpl(ctx, newNode, newPhase, message, q)
				}
			}
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			n, ok := evt.Object.(*v1.Node)
			if !ok || !filter(n) {
				return
			}
			r.handleNodeEventImpl(ctx, n, AddonNodeFailed, "The node is deleted", q)
		},
	}
}

func (r *AddonJobReconciler) handleNodeEventImpl(ctx context.Context,
	n *v1.Node, phase AddonNodePhase, message string, q v1.RequestWorkQueue) {
	jobId := v1.GetOpsJobId(n)
	r.setAddonNodePhase(jobId, n.Name, phase)

	switch phase {
	case AddonNodeFailed:
		r.addFailedNodeToCondition(ctx, jobId, n.Name, message)
	case AddonNodeSucceeded:
		if fault, _ := getFault(ctx, r.Client, n.Name, commonconfig.GetAddonFaultId()); fault != nil {
			r.Delete(ctx, fault)
		}
	}
	q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: jobId}})
}

func (r *AddonJobReconciler) addFailedNodeToCondition(ctx context.Context, jobId, nodeName, message string) {
	cond := &metav1.Condition{
		Type:               nodeName,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "AddonFailed",
		Message:            message,
	}
	backoff.Retry(func() error {
		job := &v1.OpsJob{}
		if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := updateJobCondition(ctx, r.Client, job, cond); err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
}

func (r *AddonJobReconciler) handleWorkloadEvent() handler.EventHandler {
	enqueue := func(ctx context.Context, q v1.RequestWorkQueue, clusterId string) {
		labelSelector := labels.SelectorFromSet(map[string]string{
			v1.OpsJobTypeLabel: string(v1.OpsJobAddonType), v1.ClusterIdLabel: clusterId})
		jobList := &v1.OpsJobList{}
		if r.List(ctx, jobList, &client.ListOptions{LabelSelector: labelSelector}) != nil {
			return
		}
		for _, job := range jobList.Items {
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

func (r *AddonJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	filter := func(_ context.Context, job *v1.OpsJob) bool {
		return !r.isConcernedJob(string(job.Spec.Type))
	}
	clearFuncs := []ClearFunc{r.removeJobLabelOfNodes, r.removeJob}
	return doReconcile(ctx, r.Client, req, filter, r.observe, nil, r.handle, clearFuncs...)
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
		reason := JobFailed
		if phase == v1.OpsJobSucceeded {
			reason = JobSucceed
		}
		if err := setJobCompleted(ctx, r.Client, job, phase, reason, message); err != nil {
			return false, err
		}
	}
	return true, nil
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
	for key, val := range addonJob.nodePhases {
		if val == AddonNodeRunning {
			runningCount++
			if runningCount >= addonJob.batchCount {
				return nil
			}
		} else if val == AddonNodePending {
			allPendingNodes = append(allPendingNodes, key)
		}
	}
	sort.Strings(allPendingNodes)
	return slice.Copy(allPendingNodes, addonJob.batchCount-runningCount)
}

func (r *AddonJobReconciler) removeJobLabelOfNodes(ctx context.Context, job *v1.OpsJob) error {
	addonJob, ok := r.allJobs[job.Name]
	if !ok {
		return nil
	}
	for nodeName := range addonJob.nodePhases {
		adminNode, err := getAdminNode(ctx, r.Client, nodeName)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if adminNode == nil || v1.GetOpsJobId(adminNode) != job.Name {
			continue
		}
		patch := client.MergeFrom(adminNode.DeepCopy())
		nodesLabelAction := commonnodes.BuildAction(v1.NodeActionRemove, v1.OpsJobIdLabel, v1.OpsJobTypeLabel)
		nodesAnnotationAction := commonnodes.BuildAction(v1.NodeActionRemove, v1.OpsJobInputAnnotation)
		metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeLabelAction, nodesLabelAction)
		metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeAnnotationAction, nodesAnnotationAction)
		if err = r.Patch(ctx, adminNode, patch); err != nil {
			klog.ErrorS(err, "failed to patch node")
			return err
		}
	}
	return nil
}

func (r *AddonJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if !r.hasJob(job.Name) {
		inputNodes, err := r.getInputNodes(ctx, job)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		if err = r.addJob(job, inputNodes); err != nil {
			err = setJobCompleted(ctx, r.Client, job, v1.OpsJobFailed, JobInternalError, err.Error())
			return ctrlruntime.Result{}, err
		}
	}

	if job.IsPending() {
		patch := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = v1.OpsJobRunning
		result := ctrlruntime.Result{}
		if err := r.Status().Patch(ctx, job, patch); err != nil {
			return result, err
		}
		// ensure that job will be reconciled when it is timeout
		if job.Spec.TimeoutSecond > 0 {
			result.RequeueAfter = time.Second * time.Duration(job.Spec.TimeoutSecond)
		}
		return result, nil
	}

	return r.handleImpl(ctx, job)
}

func (r *AddonJobReconciler) handleImpl(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	targetNodes := r.getNodesToProcess(job)
	if len(targetNodes) == 0 {
		return ctrlruntime.Result{}, nil
	}
	opsJobInput, err := r.buildOpsJobInput(ctx, job)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	allUsingNodes := sets.NewSet()
	if v1.IsSecurityUpgrade(job) {
		if allUsingNodes, err = commonnodes.GetUsingNodesOfCluster(ctx, r.Client, job.Spec.Cluster); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	for _, n := range targetNodes {
		nodeInput := NodeInput{nodeName: n, allUsingNodes: allUsingNodes, opsJobInput: opsJobInput}
		if result, err := r.handleNode(ctx, job, nodeInput); err != nil || result.RequeueAfter > 0 {
			if utils.IsNonRetryableError(err) {
				r.setAddonNodePhase(job.Name, n, AddonNodeFailed)
				continue
			}
			return result, err
		}
	}
	return ctrlruntime.Result{}, nil
}

type NodeInput struct {
	nodeName      string
	allUsingNodes sets.Set
	opsJobInput   *commonjob.OpsJobInput
}

func (r *AddonJobReconciler) handleNode(ctx context.Context, job *v1.OpsJob, nodeInput NodeInput) (ctrlruntime.Result, error) {
	adminNode, err := getAdminNode(ctx, r.Client, nodeInput.nodeName)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if v1.GetOpsJobId(adminNode) == job.Name {
		return ctrlruntime.Result{}, nil
	} else if v1.GetOpsJobId(adminNode) != "" {
		klog.Errorf("another ops job(%s) is running, try later", v1.GetOpsJobId(adminNode))
		return ctrlruntime.Result{RequeueAfter: time.Second * 10}, nil
	}

	if err = r.createAddonFault(ctx, job, adminNode); err != nil {
		return ctrlruntime.Result{}, err
	}

	// This node is currently being used by another workload. Please retry later, but first apply a taint(via fault).
	if nodeInput.allUsingNodes.Has(nodeInput.nodeName) {
		return ctrlruntime.Result{}, nil
	}

	patch := client.MergeFrom(adminNode.DeepCopy())
	v1.SetLabel(adminNode, v1.OpsJobIdLabel, job.Name)
	v1.SetLabel(adminNode, v1.OpsJobTypeLabel, string(job.Spec.Type))
	nodeLabelAction := commonnodes.BuildAction(v1.NodeActionAdd, v1.OpsJobIdLabel, v1.OpsJobTypeLabel)
	v1.SetAnnotation(adminNode, v1.NodeLabelAction, nodeLabelAction)
	v1.SetAnnotation(adminNode,
		v1.OpsJobInputAnnotation, string(jsonutils.MarshalSilently(*nodeInput.opsJobInput)))
	nodeAnnotationAction := commonnodes.BuildAction(v1.NodeActionAdd, v1.OpsJobInputAnnotation)
	v1.SetAnnotation(adminNode, v1.NodeAnnotationAction, nodeAnnotationAction)
	if err = r.Patch(ctx, adminNode, patch); err != nil {
		return ctrlruntime.Result{}, err
	}
	r.setAddonNodePhase(job.Name, adminNode.Name, AddonNodeRunning)
	return ctrlruntime.Result{}, nil
}

// Create an addon fault to block workload scheduling on the node for upgrade purposes
func (r *AddonJobReconciler) createAddonFault(ctx context.Context, job *v1.OpsJob, adminNode *v1.Node) error {
	faultId := commonconfig.GetAddonFaultId()
	if _, err := getFault(ctx, r.Client, adminNode.Name, faultId); err == nil || !apierrors.IsNotFound(err) {
		return nil
	}
	config, err := getFaultConfig(ctx, r.Client, faultId)
	if err != nil {
		return err
	}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonfaults.GenerateFaultName(adminNode.Name, faultId),
			Labels: map[string]string{
				v1.ClusterIdLabel: v1.GetClusterId(job),
				v1.NodeIdLabel:    adminNode.Name,
				v1.OpsJobIdLabel:  job.Name,
			},
		},
		Spec: v1.FaultSpec{
			Id:      faultId,
			Message: "upgrade Addon",
			Action:  string(config.Action),
			Node: &v1.FaultNode{
				ClusterName: v1.GetClusterId(job),
				AdminName:   adminNode.Name,
				K8sName:     adminNode.GetK8sNodeName(),
			},
		},
	}
	if err = r.Create(ctx, fault); err != nil {
		return err
	}
	return nil
}

func (r *AddonJobReconciler) addJob(job *v1.OpsJob, inputNodes []*v1.Node) error {
	if len(inputNodes) == 0 {
		return fmt.Errorf("no nodes are found")
	}
	nodePhases := make(map[string]AddonNodePhase)
	for _, n := range inputNodes {
		nodePhases[n.Name] = AddonNodePending
	}
	addonJob := AddonJob{
		nodePhases: nodePhases,
	}
	if len(nodePhases) == 1 {
		addonJob.maxFailCount = 1
		addonJob.batchCount = 1
	} else {
		failRatio := 1 - commonconfig.GetJobAvailableRatio()
		if addonJob.maxFailCount = int(float64(len(nodePhases)) * failRatio); addonJob.maxFailCount <= 0 {
			addonJob.maxFailCount = 1
		}
		if addonJob.batchCount = v1.GetOpsJobBatchCount(job); addonJob.batchCount == 0 {
			addonJob.batchCount = addonJob.maxFailCount
		}
		if addonJob.batchCount > len(nodePhases) {
			addonJob.batchCount = len(nodePhases)
		}
	}
	r.Lock()
	defer r.Unlock()
	r.allJobs[job.Name] = &addonJob
	return nil
}

func (r *AddonJobReconciler) removeJob(_ context.Context, job *v1.OpsJob) error {
	r.Lock()
	defer r.Unlock()
	delete(r.allJobs, job.Name)
	return nil
}

func (r *AddonJobReconciler) hasJob(jobId string) bool {
	r.RLock()
	defer r.RUnlock()
	_, ok := r.allJobs[jobId]
	return ok
}

func (r *AddonJobReconciler) setAddonNodePhase(jobId, nodeName string, phase AddonNodePhase) {
	r.Lock()
	defer r.Unlock()
	addonJob, ok := r.allJobs[jobId]
	if !ok {
		return
	}
	addonJob.nodePhases[nodeName] = phase
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
	for _, p := range job.nodePhases {
		if p == AddonNodeFailed {
			totalFailCount++
		} else if p == AddonNodeSucceeded {
			totalSuccessCount++
		}
	}
	if totalFailCount >= job.maxFailCount {
		return v1.OpsJobFailed, fmt.Sprintf("The number of failures has reached the threshold(%d)", job.maxFailCount)
	} else if totalFailCount+totalSuccessCount >= len(job.nodePhases) {
		return v1.OpsJobSucceeded, fmt.Sprintf("success: %d, fail: %d", totalSuccessCount, totalFailCount)
	}
	return v1.OpsJobRunning, ""
}

func (r *AddonJobReconciler) getInputNodes(ctx context.Context, job *v1.OpsJob) ([]*v1.Node, error) {
	var results []*v1.Node
	isNodeSpecified := false
	for _, p := range job.Spec.Inputs {
		if p.Name != v1.ParameterNode {
			continue
		}
		isNodeSpecified = true
		node, err := getAdminNode(ctx, r.Client, p.Value)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			results = append(results, node)
		}
	}
	if isNodeSpecified {
		return results, nil
	}

	// If not specified the nodes, apply to all nodes in the cluster, except for the master.
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: job.Spec.Cluster})
	nodeList := &v1.NodeList{}
	if err := r.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	for i := range nodeList.Items {
		results = append(results, &nodeList.Items[i])
	}
	return results, nil
}

func (r *AddonJobReconciler) buildOpsJobInput(ctx context.Context, job *v1.OpsJob) (*commonjob.OpsJobInput, error) {
	params := job.GetParameters(v1.ParameterAddonTemplate)
	result := &commonjob.OpsJobInput{
		DispatchTime: time.Now().Unix(),
	}
	for i := range params {
		addonTemplate := &v1.AddonTemplate{}
		err := r.Get(ctx, client.ObjectKey{Name: params[i].Value}, addonTemplate)
		if err != nil {
			return nil, err
		}
		cmd := commonjob.OpsJobCommand{
			Addon:   params[i].Value,
			Action:  addonTemplate.Spec.Extensions[v1.AddOnAction],
			Observe: addonTemplate.Spec.Extensions[v1.AddOnObserve],
			Chip:    addonTemplate.Spec.Chip,
		}
		if addonTemplate.Spec.Type == v1.AddonTemplateSystemd {
			cmd.IsSystemd = true
		}
		result.Commands = append(result.Commands, cmd)
	}
	return result, nil
}

func getAddonNodePhase(node *v1.Node) (AddonNodePhase, string) {
	jobId := v1.GetOpsJobId(node)
	opsJobInput := commonjob.GetOpsJobInput(node)
	if opsJobInput == nil || opsJobInput.DispatchTime == 0 {
		return AddonNodePending, ""
	}
	condition := findCondition(node.Status.Conditions, v1.OpsJobKind, jobId)
	if condition == nil || opsJobInput.DispatchTime > condition.LastTransitionTime.Unix() {
		return AddonNodeRunning, ""
	}

	if condition.Status == corev1.ConditionTrue {
		lastTransitionTime := condition.LastTransitionTime.UTC().Format(timeutil.TimeRFC3339Short)
		klog.Infof("the addon job of node %s is successfully processed, time: %s, jobid: %s",
			node.Name, lastTransitionTime, jobId)
		return AddonNodeSucceeded, ""
	} else {
		return AddonNodeFailed, condition.Message
	}
}

func isAddonNodeEnd(phase AddonNodePhase) bool {
	if phase == AddonNodeSucceeded || phase == AddonNodeFailed {
		return true
	}
	return false
}
