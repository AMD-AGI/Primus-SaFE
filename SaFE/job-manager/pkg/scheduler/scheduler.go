/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	defaultRetryCount = 3
	defaultRetryDelay = 100 * time.Millisecond
)

type SchedulerReconciler struct {
	client.Client
	clusterInformers *commonutils.ObjectManager
	// cronManager manages all cron jobs for workload scheduling
	cronManager *CronJobManager
	*controller.Controller[*SchedulerMessage]
}

type SchedulerMessage struct {
	WorkspaceId string
	ClusterId   string
}

// SetupSchedulerController initializes and registers the SchedulerReconciler with the controller manager.
func SetupSchedulerController(ctx context.Context, mgr manager.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v1.Workload{}, "spec.dependencies", func(object client.Object) []string {
		workload := object.(*v1.Workload)
		if len(workload.Spec.Dependencies) == 0 {
			return nil
		}
		return workload.Spec.Dependencies
	}); err != nil {
		return fmt.Errorf("failed to setup field indexer for workload dependencies: %v", err)
	}

	r := &SchedulerReconciler{
		Client:           mgr.GetClient(),
		clusterInformers: commonutils.NewObjectManagerSingleton(),
		cronManager:      newCronJobManager(mgr),
	}
	r.Controller = controller.NewController[*SchedulerMessage](r, 1)
	r.start(ctx)

	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workload{}, builder.WithPredicates(predicate.Or(
			r.relevantChangePredicate(), predicate.GenerationChangedPredicate{}))).
		Watches(&v1.Workspace{}, r.handleWorkspaceEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Scheduler Controller successfully")
	return nil
}

// relevantChangePredicate defines which Workload changes should trigger scheduling reconciliation.
func (r *SchedulerReconciler) relevantChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			workload, ok := e.Object.(*v1.Workload)
			if !ok {
				return false
			}
			if len(workload.Spec.CronJobs) > 0 {
				r.cronManager.addOrReplace(workload)
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 {
				return false
			}
			if !reflect.DeepEqual(oldWorkload.Spec.CronJobs, newWorkload.Spec.CronJobs) {
				r.cronManager.addOrReplace(newWorkload)
			}
			if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
				return true
			}
			if v1.GetCronjobTimestamp(oldWorkload) != v1.GetCronjobTimestamp(newWorkload) {
				return true
			}
			if v1.IsWorkloadScheduled(oldWorkload) != v1.IsWorkloadScheduled(newWorkload) {
				return true
			}
			if !oldWorkload.IsDependenciesFinish() && newWorkload.IsDependenciesFinish() {
				return true
			}
			return false
		},
	}
}

// handleWorkspaceEvent creates an event handler that watches Workspace resource events.
func (r *SchedulerReconciler) handleWorkspaceEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkspace, ok1 := evt.ObjectOld.(*v1.Workspace)
			newWorkspace, ok2 := evt.ObjectNew.(*v1.Workspace)
			if !ok1 || !ok2 {
				return
			}
			// Since workspace resource updates may impact scheduling decisions, a rescheduling reconciliation is triggered.
			if !quantity.Equal(oldWorkspace.Status.AvailableResources, newWorkspace.Status.AvailableResources) ||
				oldWorkspace.Spec.QueuePolicy != newWorkspace.Spec.QueuePolicy {
				r.Add(&SchedulerMessage{
					ClusterId:   newWorkspace.Spec.Cluster,
					WorkspaceId: newWorkspace.Name,
				})
			}
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {},
	}
}

// Reconcile is the main control loop for Workload resources that triggers scheduling.
func (r *SchedulerReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	workload := new(v1.Workload)
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !workload.GetDeletionTimestamp().IsZero() {
		if result, err := r.delete(ctx, workload); err != nil || result.RequeueAfter > 0 {
			return result, err
		}
	}
	if workload.Spec.Workspace == corev1.NamespaceDefault {
		return ctrlruntime.Result{}, nil
	}

	if err := r.updateDependentsPhase(ctx, workload); err != nil {
		return ctrlruntime.Result{}, err
	}

	msg := &SchedulerMessage{
		ClusterId:   v1.GetClusterId(workload),
		WorkspaceId: workload.Spec.Workspace,
	}
	r.Add(msg)
	return ctrlruntime.Result{}, nil
}

// delete handles the deletion of a workload and its associated resources.
func (r *SchedulerReconciler) delete(ctx context.Context, adminWorkload *v1.Workload) (ctrlruntime.Result, error) {
	if len(adminWorkload.Spec.CronJobs) > 0 {
		r.cronManager.remove(adminWorkload.Name)
	}
	clusterInformer, err := syncer.GetClusterInformer(r.clusterInformers, v1.GetClusterId(adminWorkload))
	if err != nil {
		klog.Errorf("failed to get cluster informer, clusterId: %s, workspaceId: %s, workloadId: %s",
			v1.GetClusterId(adminWorkload), adminWorkload.Spec.Workspace, adminWorkload.Name)
		return ctrlruntime.Result{}, err
	}
	// generate the related resource reference
	obj, err := jobutils.GenObjectReference(ctx, r.Client, adminWorkload)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	// delete the related resource in data plane
	if err = jobutils.DeleteObject(ctx, clusterInformer.ClientFactory(), obj); err != nil {
		klog.ErrorS(err, "failed to delete k8s object")
		return ctrlruntime.Result{}, err
	}
	if controllerutil.RemoveFinalizer(adminWorkload, v1.WorkloadFinalizer) {
		if err = r.Update(ctx, adminWorkload); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	klog.Infof("delete workload, name: %s", adminWorkload.Name)
	return ctrlruntime.Result{}, nil
}

// Start implements Runnable interface in controller runtime package.
func (r *SchedulerReconciler) start(ctx context.Context) {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
}

// Do processes a scheduling message by calling the main scheduling logic.
// It is the interface of the custom controller.
func (r *SchedulerReconciler) Do(ctx context.Context, message *SchedulerMessage) (ctrlruntime.Result, error) {
	err := r.scheduleWorkloads(ctx, message)
	if utils.IsNonRetryableError(err) {
		err = nil
	}
	return ctrlruntime.Result{}, err
}

// scheduleWorkloads process all workloads that are currently queued,
// checking whether the available resources in the current workspace meet the requirements.
// Workloads that meet the criteria are passed to the next scheduling step,
// while those that do not remain in the queue and have their queue positions updated.
// Preemption of tasks is also supported.
func (r *SchedulerReconciler) scheduleWorkloads(ctx context.Context, message *SchedulerMessage) error {
	workspace, err := r.getWorkspace(ctx, message.WorkspaceId)
	if workspace == nil {
		return err
	}

	schedulingWorkloads, scheduledWorkloads, err := r.getUnfinishedWorkloads(ctx, workspace)
	if err != nil || len(schedulingWorkloads) == 0 {
		return err
	}
	leftAvailResources, leftTotalResources, err := r.getLeftTotalResources(ctx, workspace, scheduledWorkloads)
	if err != nil {
		return err
	}

	scheduledCount := 0
	unScheduledReasons := make(map[string]string)
	for i, w := range schedulingWorkloads {
		requestResources, _ := commonworkload.CvtToResourceList(w)
		var leftResources *corev1.ResourceList
		if w.Spec.IsTolerateAll {
			leftResources = &leftTotalResources
		} else {
			leftResources = &leftAvailResources
		}
		ok, reason, err := r.canScheduleWorkload(ctx, w, scheduledWorkloads, requestResources, *leftResources)
		if err != nil {
			return err
		}
		if !ok {
			unScheduledReasons[w.Name] = reason
			// Process scheduling workloads based on priority and policy
			// If the scheduling policy is FIFO, or the priority is higher than subsequent queued workloads
			// (excluding the workload which specified node), then break out of the queue directly and continue waiting.
			if reason == CronjobReason || reason == DependencyReason || w.IsEnd() {
				// CronJob or a job with dependencies that are not yet ready to start should be skipped
				continue
			} else if workspace.IsEnableFifo() {
				// In FIFO mode, if current workload cannot be scheduled, subsequent ones won't be either
				break
			} else if w.HasSpecifiedNodes() {
				// Workloads with specific node assignments should remain in queue
				continue
			} else if i < len(schedulingWorkloads)-1 && w.Spec.Priority > schedulingWorkloads[i+1].Spec.Priority {
				// If current workload has higher priority than next one, stop scheduling for now
				break
			} else {
				continue
			}
		}
		if err = r.updateScheduled(ctx, schedulingWorkloads[i]); err != nil {
			return err
		}
		klog.Infof("the workload is scheduled, name: %s, dispatch count: %d",
			w.Name, v1.GetWorkloadDispatchCnt(w)+1)
		leftAvailResources = quantity.SubResource(leftAvailResources, requestResources)
		leftTotalResources = quantity.SubResource(leftTotalResources, requestResources)
		scheduledWorkloads = append(scheduledWorkloads, schedulingWorkloads[i])
		scheduledCount++
	}
	if scheduledCount != len(schedulingWorkloads) {
		r.updateUnScheduled(ctx, schedulingWorkloads, unScheduledReasons)
	}
	return nil
}

// canScheduleWorkload checks if a workload can be scheduled based on resource availability.
func (r *SchedulerReconciler) canScheduleWorkload(ctx context.Context, requestWorkload *v1.Workload,
	scheduledWorkloads []*v1.Workload, requestResources, leftResources corev1.ResourceList) (bool, string, error) {
	for _, job := range requestWorkload.Spec.CronJobs {
		if job.Action == v1.CronStart {
			_, scheduleTime, err := timeutil.CvtTime3339ToCronStandard(job.Schedule)
			if err == nil && scheduleTime.After(time.Now().UTC()) {
				return false, CronjobReason, nil
			}
		}
	}
	hasEnoughQuota, key := quantity.IsSubResource(requestResources, leftResources)
	isDependencyReady, err := r.checkWorkloadDependencies(ctx, requestWorkload)
	if err != nil {
		return false, "", err
	}
	var reason string
	if !isDependencyReady {
		reason = DependencyReason
		klog.Infof("the workload(%s) is not scheduled, reason: %s", requestWorkload.Name, reason)
		return false, reason, nil
	}

	isPreemptable := false
	if !hasEnoughQuota {
		reason = fmt.Sprintf("Insufficient total %s resources", formatResourceName(key))
		isPreemptable, err = r.preempt(ctx, requestWorkload, scheduledWorkloads, leftResources)
	} else {
		hasEnoughQuota, reason, err = r.checkNodeResources(ctx, requestWorkload, scheduledWorkloads)
		if !hasEnoughQuota {
			isPreemptable = r.isPreemptable(requestWorkload, scheduledWorkloads)
		}
	}
	if err != nil {
		return false, "", err
	}
	if !hasEnoughQuota && !isPreemptable {
		klog.Infof("the workload(%s) is not scheduled, reason: %s, request.resource: %s, left.resource: %s",
			requestWorkload.Name, reason, string(jsonutils.MarshalSilently(requestResources)),
			string(jsonutils.MarshalSilently(leftResources)))
		return false, reason, nil
	}
	return true, "", nil
}

// checkWorkloadDependencies checks whether all dependencies of the workload are satisfied.
func (r *SchedulerReconciler) checkWorkloadDependencies(ctx context.Context, workload *v1.Workload) (bool, error) {
	isReady := true
	isChange := false
	for _, dep := range workload.Spec.Dependencies {
		phase, ok := workload.GetDependenciesPhase(dep)
		if !ok {
			depWorkload := &v1.Workload{}
			if err := r.Get(ctx, client.ObjectKey{Name: dep, Namespace: workload.Namespace}, depWorkload); err != nil {
				if apierrors.IsNotFound(err) {
					if err = jobutils.SetWorkloadFailed(ctx, r.Client, workload, fmt.Sprintf("dependency workload %s not found", dep)); err != nil {
						klog.Errorf("failed to set workload %s dependency failed", workload.Name)
						return true, err
					}
					return false, nil
				}
				return isReady, err
			}
			phase = depWorkload.Status.Phase
			if depWorkload.IsEnd() {
				workload.SetDependenciesPhase(dep, depWorkload.Status.Phase)
				isChange = true
			}
		}
		if phase != v1.WorkloadSucceeded {
			isReady = false
		}
	}
	if isChange {
		if err := r.Status().Update(ctx, workload); err != nil {
			return isReady, err
		}
	}

	return isReady, nil
}

// checkNodeResources check if each node's available resources satisfy the workload's resource requests.
// Return true if satisfied, false otherwise, along with the reason.
func (r *SchedulerReconciler) checkNodeResources(ctx context.Context,
	requestWorkload *v1.Workload, runningWorkloads []*v1.Workload) (bool, string, error) {
	nodes, err := getAvailableResourcesPerNode(ctx, r.Client, requestWorkload, runningWorkloads)
	if err != nil {
		return false, "", err
	}
	podResources, err := commonworkload.GetPodResources(&requestWorkload.Spec.Resource)
	if err != nil {
		return false, "", err
	}
	if len(nodes) == 0 {
		return false, buildReason(requestWorkload, podResources, nil), nil
	}
	// All nodes within the same workspace are of the same flavor
	nf := &v1.NodeFlavor{}
	if err = r.Get(ctx, client.ObjectKey{Name: v1.GetNodeFlavorId(nodes[0].node)}, nf); err != nil {
		return false, "", err
	}

	matchCount := 0
	totalCount := requestWorkload.Spec.Resource.Replica
	var unmatchedNodes []*NodeWrapper
	for i, n := range nodes {
		ok, _ := quantity.IsSubResource(podResources, n.resource)
		if ok {
			matchCount++
			if matchCount >= totalCount {
				break
			}
		} else {
			nodes[i].resourceScore = buildResourceWeight(requestWorkload, n.resource, nf)
			unmatchedNodes = append(unmatchedNodes, &nodes[i])
		}
	}
	if matchCount >= totalCount {
		return true, "", nil
	}
	return false, buildReason(requestWorkload, podResources, unmatchedNodes), nil
}

// getWorkspace retrieves a workspace by cluster ID and workspace ID.
func (r *SchedulerReconciler) getWorkspace(ctx context.Context, workspaceId string) (*v1.Workspace, error) {
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: workspaceId}, workspace); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return workspace, nil
}

// getAdminSecret retrieves a secret from the admin plane.
func (r *SchedulerReconciler) getAdminSecret(ctx context.Context, secretId string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretId, Namespace: common.PrimusSafeNamespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// getUnfinishedWorkloads Retrieve the list of unfinished workloads, sorted by priority and other criteria, including both queued and running ones.
func (r *SchedulerReconciler) getUnfinishedWorkloads(ctx context.Context, workspace *v1.Workspace) ([]*v1.Workload, []*v1.Workload, error) {
	filterFunc := func(w *v1.Workload) bool {
		return w.IsEnd()
	}
	workloads, err := commonworkload.GetWorkloadsOfWorkspace(ctx, r.Client,
		workspace.Spec.Cluster, []string{workspace.Name}, filterFunc)
	if err != nil {
		return nil, nil, err
	}
	var schedulingWorkloads, scheduledWorkloads []*v1.Workload
	for i, w := range workloads {
		if !v1.IsWorkloadScheduled(w) {
			schedulingWorkloads = append(schedulingWorkloads, workloads[i])
		} else {
			scheduledWorkloads = append(scheduledWorkloads, workloads[i].DeepCopy())
		}
	}
	if len(schedulingWorkloads) > 0 {
		sort.Sort(WorkloadList(schedulingWorkloads))
	}
	if len(scheduledWorkloads) > 0 {
		sort.Sort(WorkloadList(scheduledWorkloads))
	}
	return schedulingWorkloads, scheduledWorkloads, nil
}

// getLeftTotalResources Retrieve the total amount of left resources. The system usually reserves a certain amount of CPU, memory, and other resources.
func (r *SchedulerReconciler) getLeftTotalResources(ctx context.Context,
	workspace *v1.Workspace, workloads []*v1.Workload) (corev1.ResourceList, corev1.ResourceList, error) {
	filterFunc := func(nodeName string) bool {
		n := &v1.Node{}
		if err := r.Get(ctx, client.ObjectKey{Name: nodeName}, n); err != nil {
			return true
		}
		return !n.IsAvailable(false)
	}
	usedResource := make(corev1.ResourceList)
	for _, w := range workloads {
		var resourceList corev1.ResourceList
		var err error
		if w.IsRunning() {
			resourceList, _, err = commonworkload.GetActiveResources(w, filterFunc)
		} else {
			resourceList, err = commonworkload.CvtToResourceList(w)
		}
		if err != nil {
			return nil, nil, err
		}
		usedResource = quantity.AddResource(usedResource, resourceList)
	}

	availResource := workspace.Status.AvailableResources
	leftAvailResource := quantity.SubResource(availResource, usedResource)
	totalResource := quantity.GetAvailableResource(workspace.Status.TotalResources)
	leftTotalResource := quantity.SubResource(totalResource, usedResource)
	klog.Infof("total resource: %v, total used: %v, left total: %v, left avail: %v",
		totalResource, usedResource, leftTotalResource, leftAvailResource)
	return leftAvailResource, leftTotalResource, nil
}

// updateScheduled updates a workload's status to indicate it has been scheduled.
func (r *SchedulerReconciler) updateScheduled(ctx context.Context, workload *v1.Workload) error {
	name := workload.Name
	if err := backoff.ConflictRetry(func() error {
		if innerError := r.updateStatus(ctx, workload); innerError == nil {
			return nil
		} else {
			if apierrors.IsConflict(innerError) {
				r.Get(ctx, client.ObjectKey{Name: name}, workload)
				if workload == nil {
					return commonerrors.NewNotFoundWithMessage(fmt.Sprintf("The workload %s is not found", name))
				}
			}
			return innerError
		}
	}, defaultRetryCount, defaultRetryDelay); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", workload.Name)
		return err
	}

	annotations := workload.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[v1.WorkloadScheduledAnnotation] = timeutil.FormatRFC3339(time.Now().UTC())
	delete(annotations, v1.WorkloadReScheduledAnnotation)
	workload.SetAnnotations(annotations)
	if err := r.Update(ctx, workload); err != nil {
		klog.ErrorS(err, "failed to update workload", "name", workload.Name)
		return err
	}
	return nil
}

// updateStatus updates the workload status with scheduling information.
func (r *SchedulerReconciler) updateStatus(ctx context.Context, workload *v1.Workload) error {
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload) + 1)
	cond := jobutils.NewCondition(string(v1.AdminScheduled), "the workload is scheduled", reason)
	if jobutils.FindCondition(workload, cond) != nil {
		return nil
	}
	workload.Status.Conditions = append(workload.Status.Conditions, *cond)
	workload.Status.QueuePosition = 0
	if workload.Status.Phase == "" {
		workload.Status.Phase = v1.WorkloadPending
	}
	if err := r.Status().Update(ctx, workload); err != nil {
		return err
	}
	return nil
}

// updateUnScheduled updates the status of unscheduled workloads with ordering and reasons.
func (r *SchedulerReconciler) updateUnScheduled(ctx context.Context, workloads []*v1.Workload, unScheduledReasons map[string]string) {
	position := 1
	for i, w := range workloads {
		if v1.IsWorkloadScheduled(w) || w.IsEnd() {
			continue
		}
		isChanged := false
		if workloads[i].Status.QueuePosition != position {
			workloads[i].Status.QueuePosition = position
			isChanged = true
		}
		reason, _ := unScheduledReasons[w.Name]
		if reason == "" && position > 1 {
			reason = "There are high priority or pre-created tasks that have not been scheduled yet"
		}
		if reason != workloads[i].Status.Message {
			workloads[i].Status.Message = reason
			isChanged = true
		}
		if isChanged {
			patchObj := map[string]any{
				"metadata": map[string]any{
					"resourceVersion": workloads[i].ResourceVersion,
				},
				"status": map[string]any{
					"queuePosition": position,
					"message":       reason,
				},
			}
			p, err := json.Marshal(patchObj)
			if err != nil {
				klog.ErrorS(err, "failed to marshal patch object")
				continue
			}
			if err := r.Status().Patch(ctx, workloads[i], client.RawPatch(types.MergePatchType, p)); err != nil {
				klog.ErrorS(err, "failed to patch workload status", "name", workloads[i].Name)
			}
		}
		position++
	}
}

// updateDependentsPhase handles the phase of dependent workloads based on the status of their dependencies.
func (r *SchedulerReconciler) updateDependentsPhase(ctx context.Context, workload *v1.Workload) error {
	if !workload.IsEnd() {
		return nil
	}
	var dependents v1.WorkloadList
	if err := r.List(ctx, &dependents, client.MatchingFields{"spec.dependencies": workload.Name}); err != nil {
		klog.Errorf("failed to list dependencies for workload %s: %v", workload.Name, err)
		return err
	}
	for _, depWorkload := range dependents.Items {
		if err := r.setDependenciesPhase(ctx, workload, &depWorkload); err != nil {
			klog.Errorf("failed to set dependency phase for workload %s in dependent workload %s: %v", workload.Name, depWorkload.Name, err)
			return err
		}
	}

	return nil
}

// setDependenciesPhase sets the phase of a dependent workload based on the status of its dependency.
func (r *SchedulerReconciler) setDependenciesPhase(ctx context.Context, workload, depWorkload *v1.Workload) error {
	depWorkload.SetDependenciesPhase(workload.Name, workload.Status.Phase)
	if workload.Status.Phase != v1.WorkloadSucceeded {
		if err := jobutils.SetWorkloadFailed(ctx, r.Client, depWorkload, fmt.Sprintf("dependency workload %s failed", workload.Name)); err != nil {
			return err
		}
		return nil
	}
	return r.Status().Update(ctx, depWorkload)
}
