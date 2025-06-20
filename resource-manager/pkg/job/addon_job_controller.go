/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package job

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
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/job"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

type NodeJobPhase int

const (
	NodeJobPending   NodeJobPhase = 0
	NodeJobRunning   NodeJobPhase = 1
	NodeJobSucceeded NodeJobPhase = 2
	NodeJobFailed    NodeJobPhase = 3
)

type AddonJob struct {
	// store the processing status for each node. key is the node name
	nodePhases map[string]NodeJobPhase
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
	// related fault config
	addonFaultConfig *resource.FaultConfig
}

func SetupAddonJobController(ctx context.Context, mgr manager.Manager) error {
	r := &AddonJobReconciler{
		Client:  mgr.GetClient(),
		allJobs: make(map[string]*AddonJob),
	}
	var err error
	r.addonFaultConfig, err = getFaultConfig(ctx, r.Client, commonconfig.GetAddonFaultId())
	if err != nil {
		return err
	}
	if !r.addonFaultConfig.IsEnable() {
		return fmt.Errorf("the addon fault is disabled")
	}
	err = ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Job{}, builder.WithPredicates(predicate.Or(
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
			oldJob, ok1 := e.ObjectOld.(*v1.Job)
			newJob, ok2 := e.ObjectNew.(*v1.Job)
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
	return v1.JobType(jobType) == v1.JobAddonType
}

func (r *AddonJobReconciler) handleNodeEvent() handler.EventHandler {
	filter := func(n *v1.Node) bool {
		return v1.GetJobId(n) == "" || !r.isConcernedJob(v1.GetJobType(n))
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			n, ok := evt.Object.(*v1.Node)
			if !ok || filter(n) {
				return
			}
			phase, message := getNodeJobPhase(n)
			if isNodeJobEnd(phase) {
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
				r.handleNodeEventImpl(ctx, newNode, NodeJobFailed, "The node is unmanaged", q)
			} else {
				oldPhase, _ := getNodeJobPhase(oldNode)
				newPhase, message := getNodeJobPhase(newNode)
				if !isNodeJobEnd(oldPhase) && isNodeJobEnd(newPhase) ||
					(isNodeJobEnd(oldPhase) && isNodeJobEnd(newPhase) && oldPhase != newPhase) {
					r.handleNodeEventImpl(ctx, newNode, newPhase, message, q)
				}
			}
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			n, ok := evt.Object.(*v1.Node)
			if !ok || !filter(n) {
				return
			}
			r.handleNodeEventImpl(ctx, n, NodeJobFailed, "The node is deleted", q)
		},
	}
}

func (r *AddonJobReconciler) handleNodeEventImpl(ctx context.Context,
	n *v1.Node, phase NodeJobPhase, message string, q v1.RequestWorkQueue) {
	jobId := v1.GetJobId(n)
	backoff.Retry(func() error {
		job := &v1.Job{}
		if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := r.updateJobConditionByNode(ctx, job, phase, n.Name, message); err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)

	if phase == NodeJobSucceeded {
		if fault, _ := getFault(ctx, r.Client, n.Name, commonconfig.GetAddonFaultId()); fault != nil {
			r.Delete(ctx, fault)
		}
	}
	q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: jobId}})
}

func (r *AddonJobReconciler) updateJobConditionByNode(ctx context.Context,
	job *v1.Job, phase NodeJobPhase, nodeName, message string) error {
	r.setNodeJobPhase(job.Name, nodeName, phase)
	status := metav1.ConditionTrue
	reason := "NodeAddonSucceed"
	if phase == NodeJobFailed {
		status = metav1.ConditionFalse
		reason = "NodeJobFailed"
	}
	cond := &metav1.Condition{
		Type:               nodeName,
		Status:             status,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
	}
	if err := updateJobCondition(ctx, r.Client, job, cond); err != nil {
		return err
	}
	klog.Infof("update job %s condition %v", job.Name, cond)
	return nil
}

func (r *AddonJobReconciler) handleWorkloadEvent() handler.EventHandler {
	enqueue := func(q v1.RequestWorkQueue, clusterId string) {
		labelSelector := labels.SelectorFromSet(map[string]string{
			v1.JobTypeLabel: string(v1.JobAddonType), v1.ClusterIdLabel: clusterId})
		jobList := &v1.JobList{}
		if r.List(context.Background(), jobList, &client.ListOptions{LabelSelector: labelSelector}) != nil {
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
				enqueue(q, v1.GetClusterId(newWorkload))
			}
		},
	}
}

func (r *AddonJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	filter := func(_ context.Context, job *v1.Job) bool {
		return !r.isConcernedJob(string(job.Spec.Type))
	}
	clearFuncs := []ClearFunc{r.removeJobLabelOfNodes, r.removeJob}
	return doReconcile(ctx, r.Client, req, filter, r.observe, nil, r.handle, clearFuncs...)
}

// Observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *AddonJobReconciler) observe(ctx context.Context, job *v1.Job) (bool, error) {
	phase, message := r.getJobPhase(job.Name)
	switch phase {
	case v1.JobPending, "":
		return false, nil
	case v1.JobRunning:
		nodes := r.getNodesToProcess(job)
		return len(nodes) == 0, nil
	case v1.JobFailed, v1.JobSucceeded:
		reason := JobFailed
		if phase == v1.JobSucceeded {
			reason = JobSucceed
		}
		if err := setJobCompleted(ctx, r.Client, job, phase, reason, message); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *AddonJobReconciler) getNodesToProcess(job *v1.Job) []string {
	r.RLock()
	defer r.RUnlock()
	addonJob, ok := r.allJobs[job.Name]
	if !ok {
		return nil
	}
	runningCount := 0
	var allPendingNodes []string
	for key, val := range addonJob.nodePhases {
		if val == NodeJobRunning {
			runningCount++
			if runningCount >= addonJob.batchCount {
				return nil
			}
		} else if val == NodeJobPending {
			allPendingNodes = append(allPendingNodes, key)
		}
	}
	sort.Strings(allPendingNodes)
	return slice.Copy(allPendingNodes, addonJob.batchCount-runningCount)
}

func (r *AddonJobReconciler) removeJobLabelOfNodes(ctx context.Context, job *v1.Job) error {
	addonJob, ok := r.allJobs[job.Name]
	if !ok {
		return nil
	}
	for nodeName := range addonJob.nodePhases {
		adminNode, err := getAdminNode(ctx, r.Client, nodeName)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if adminNode == nil || v1.GetJobId(adminNode) != job.Name {
			continue
		}
		patch := client.MergeFrom(adminNode.DeepCopy())
		nodesLabelAction := commonnodes.BuildAction(v1.NodeActionRemove, v1.JobIdLabel, v1.JobTypeLabel)
		nodesAnnotationAction := commonnodes.BuildAction(v1.NodeActionRemove, v1.NodeJobInputAnnotation)
		metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeLabelAction, nodesLabelAction)
		metav1.SetMetaDataAnnotation(&adminNode.ObjectMeta, v1.NodeAnnotationAction, nodesAnnotationAction)
		if err = r.Patch(ctx, adminNode, patch); err != nil {
			klog.ErrorS(err, "failed to patch node")
			return err
		}
	}
	return nil
}

func (r *AddonJobReconciler) handle(ctx context.Context, job *v1.Job) (ctrlruntime.Result, error) {
	if !r.hasJob(job.Name) {
		inputNodes, err := r.getInputNodes(ctx, job)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		if err = r.addJob(job, inputNodes); err != nil {
			err = setJobCompleted(ctx, r.Client, job, v1.JobFailed, InternalError, err.Error())
			return ctrlruntime.Result{}, err
		}
	}

	if job.IsPending() {
		patch := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = v1.JobRunning
		result := ctrlruntime.Result{}
		if err := r.Status().Patch(context.Background(), job, patch); err != nil {
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

func (r *AddonJobReconciler) handleImpl(ctx context.Context, job *v1.Job) (ctrlruntime.Result, error) {
	targetNodes := r.getNodesToProcess(job)
	if len(targetNodes) == 0 {
		return ctrlruntime.Result{}, nil
	}
	nodeJobInput, err := r.buildNodeJobInput(ctx, job)
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
		nodeJob := NodeJob{nodeName: n, isNodeInUse: allUsingNodes.Has(n), jobInput: nodeJobInput}
		if result, err := r.handleNode(ctx, job, nodeJob); err != nil && result.RequeueAfter > 0 {
			return result, err
		}
	}
	return ctrlruntime.Result{}, nil
}

type NodeJob struct {
	nodeName    string
	isNodeInUse bool
	jobInput    *commonjob.NodeJobInput
}

func (r *AddonJobReconciler) handleNode(ctx context.Context, job *v1.Job, nodeJob NodeJob) (ctrlruntime.Result, error) {
	adminNode, err := getAdminNode(ctx, r.Client, nodeJob.nodeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.setNodeJobPhase(job.Name, nodeJob.nodeName, NodeJobFailed)
		}
		return ctrlruntime.Result{}, err
	}
	if v1.GetJobId(adminNode) == job.Name {
		return ctrlruntime.Result{}, nil
	} else if v1.GetJobId(adminNode) != "" {
		klog.Errorf("another job(%s) is running, try later", v1.GetJobId(adminNode))
		return ctrlruntime.Result{RequeueAfter: time.Second * 10}, nil
	}

	if _, err = getFault(ctx, r.Client, adminNode.Name, commonconfig.GetAddonFaultId()); apierrors.IsNotFound(err) {
		fault := r.generateAddonFault(job, adminNode)
		if err = r.Create(context.Background(), fault); err != nil {
			return ctrlruntime.Result{}, err
		}
	}

	// This node is currently being used by another workload. Please retry later, but first apply a taint.
	if nodeJob.isNodeInUse {
		return ctrlruntime.Result{}, nil
	}

	patch := client.MergeFrom(adminNode.DeepCopy())
	v1.SetLabel(adminNode, v1.JobIdLabel, job.Name)
	v1.SetLabel(adminNode, v1.JobTypeLabel, string(job.Spec.Type))
	nodeLabelAction := commonnodes.BuildAction(v1.NodeActionAdd, v1.JobIdLabel, v1.JobTypeLabel)
	v1.SetAnnotation(adminNode, v1.NodeLabelAction, nodeLabelAction)
	v1.SetAnnotation(adminNode,
		v1.NodeJobInputAnnotation, string(jsonutils.MarshalSilently(*nodeJob.jobInput)))
	nodeAnnotationAction := commonnodes.BuildAction(v1.NodeActionAdd, v1.NodeJobInputAnnotation)
	v1.SetAnnotation(adminNode, v1.NodeAnnotationAction, nodeAnnotationAction)
	if err = r.Patch(context.Background(), adminNode, patch); err != nil {
		return ctrlruntime.Result{}, err
	}
	r.setNodeJobPhase(job.Name, adminNode.Name, NodeJobRunning)
	return ctrlruntime.Result{}, nil
}

func (r *AddonJobReconciler) addJob(job *v1.Job, inputNodes []*v1.Node) error {
	if len(inputNodes) == 0 {
		return fmt.Errorf("no nodes are found")
	}
	nodePhases := make(map[string]NodeJobPhase)
	for _, n := range inputNodes {
		nodePhases[n.Name] = NodeJobPending
	}
	addonJob := AddonJob{
		nodePhases: nodePhases,
	}
	failRatio := 1 - commonconfig.GetJobAvailableRatio()
	if addonJob.maxFailCount = int(float64(len(nodePhases)) * failRatio); addonJob.maxFailCount <= 0 {
		addonJob.maxFailCount = 1
	}
	if addonJob.batchCount = v1.GetJobBatchCount(job); addonJob.batchCount == 0 {
		addonJob.batchCount = addonJob.maxFailCount
	}
	r.Lock()
	defer r.Unlock()
	r.allJobs[job.Name] = &addonJob
	return nil
}

func (r *AddonJobReconciler) removeJob(_ context.Context, job *v1.Job) error {
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

func (r *AddonJobReconciler) setNodeJobPhase(jobId, nodeName string, phase NodeJobPhase) {
	r.Lock()
	defer r.Unlock()
	addonJob, ok := r.allJobs[jobId]
	if !ok {
		return
	}
	addonJob.nodePhases[nodeName] = phase
}

func (r *AddonJobReconciler) getJobPhase(jobId string) (v1.JobPhase, string) {
	r.RLock()
	defer r.RUnlock()
	job, ok := r.allJobs[jobId]
	if !ok {
		return v1.JobPending, ""
	}
	totalFailCount := 0
	totalSuccessCount := 0
	for _, p := range job.nodePhases {
		if p == NodeJobFailed {
			totalFailCount++
		} else if p == NodeJobSucceeded {
			totalSuccessCount++
		}
	}
	if totalFailCount >= job.maxFailCount {
		return v1.JobFailed, fmt.Sprintf("The number of failures has reached the threshold(%d)", job.maxFailCount)
	} else if totalFailCount+totalSuccessCount >= len(job.nodePhases) {
		return v1.JobSucceeded, fmt.Sprintf("success: %d, fail: %d", totalSuccessCount, totalFailCount)
	}
	return v1.JobRunning, ""
}

func (r *AddonJobReconciler) getInputNodes(ctx context.Context, job *v1.Job) ([]*v1.Node, error) {
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
	if err := r.List(context.Background(), nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	for i := range nodeList.Items {
		results = append(results, &nodeList.Items[i])
	}
	return results, nil
}

func (r *AddonJobReconciler) buildNodeJobInput(ctx context.Context, job *v1.Job) (*commonjob.NodeJobInput, error) {
	params := job.GetParameters(v1.ParameterAddonTemplate)
	nodeJob := &commonjob.NodeJobInput{
		DispatchTime: time.Now().Unix(),
	}
	for i := range params {
		addonTemplate := &v1.AddonTemplate{}
		err := r.Get(ctx, client.ObjectKey{Name: params[i].Value}, addonTemplate)
		if err != nil {
			return nil, err
		}
		cmd := commonjob.NodeJobCommand{
			Addon:   params[i].Value,
			Action:  addonTemplate.Spec.Extensions[v1.AddOnAction],
			Observe: addonTemplate.Spec.Extensions[v1.AddOnObserve],
			Chip:    addonTemplate.Spec.Chip,
		}
		if addonTemplate.Spec.Type == v1.AddonTemplateSystemd {
			cmd.IsSystemd = true
		}
		nodeJob.Commands = append(nodeJob.Commands, cmd)
	}
	return nodeJob, nil
}

func (r *AddonJobReconciler) generateAddonFault(job *v1.Job, adminNode *v1.Node) *v1.Fault {
	return &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonfaults.GenerateFaultName(adminNode.Name, r.addonFaultConfig.Id),
			Labels: map[string]string{
				v1.ClusterIdLabel: v1.GetClusterId(job),
				v1.NodeIdLabel:    adminNode.Name,
				v1.JobIdLabel:     job.Name,
			},
			Annotations: map[string]string{
				v1.JobUserAnnotation: v1.GetUserName(job),
			},
		},
		Spec: v1.FaultSpec{
			Id:                  r.addonFaultConfig.Id,
			Message:             "upgrade Addon",
			Action:              string(r.addonFaultConfig.Action),
			IsAutoRepairEnabled: r.addonFaultConfig.IsAutoRepairEnabled(),
			Node: &v1.FaultNode{
				ClusterName: v1.GetClusterId(job),
				AdminName:   adminNode.Name,
				K8sName:     adminNode.GetK8sNodeName(),
			},
		},
	}
}

func getNodeJobPhase(node *v1.Node) (NodeJobPhase, string) {
	jobId := v1.GetJobId(node)
	nodeJobInput := commonjob.GetNodeJobInput(node)
	if nodeJobInput == nil || nodeJobInput.DispatchTime == 0 {
		return NodeJobPending, ""
	}
	nodeJobCond := findCondition(node.Status.Conditions, v1.NodeJob, jobId)
	if nodeJobCond == nil || nodeJobInput.DispatchTime > nodeJobCond.LastTransitionTime.Unix() {
		return NodeJobRunning, ""
	}

	if nodeJobCond.Status == corev1.ConditionTrue {
		lastTransitionTime := nodeJobCond.LastTransitionTime.UTC().Format(timeutil.TimeRFC3339Short)
		klog.Infof("the addon job of node %s is successfully processed, time: %s, jobid: %s",
			node.Name, lastTransitionTime, jobId)
		return NodeJobSucceeded, fmt.Sprintf("The Addon is processed at %s", lastTransitionTime)
	} else {
		return NodeJobFailed, "Failed to process Addon, message: " + nodeJobCond.Message
	}
}

func isNodeJobEnd(phase NodeJobPhase) bool {
	if phase == NodeJobSucceeded || phase == NodeJobFailed {
		return true
	}
	return false
}
