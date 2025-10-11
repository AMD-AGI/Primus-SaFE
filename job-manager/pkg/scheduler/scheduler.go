/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

type SchedulerReconciler struct {
	client.Client
	clusterInformers *commonutils.ObjectManager
	*controller.Controller[*SchedulerMessage]
}

type SchedulerMessage struct {
	WorkspaceId string
	ClusterId   string
	WorkloadId  string
}

func SetupSchedulerController(ctx context.Context, mgr manager.Manager) error {
	r := &SchedulerReconciler{
		Client:           mgr.GetClient(),
		clusterInformers: commonutils.NewObjectManagerSingleton(),
	}
	r.Controller = controller.NewController[*SchedulerMessage](r, 1)
	r.start(ctx)

	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workload{}, builder.WithPredicates(predicate.Or(
			r.caredChangePredicate(), predicate.GenerationChangedPredicate{}))).
		Watches(&v1.Workspace{}, r.handleByWorkspace()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Scheduler Controller successfully")
	return nil
}

func (r *SchedulerReconciler) caredChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 {
				return false
			}
			if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
				return true
			}
			if v1.IsWorkloadScheduled(oldWorkload) != v1.IsWorkloadScheduled(newWorkload) {
				return true
			}
			return false
		},
	}
}

func (r *SchedulerReconciler) handleByWorkspace() handler.EventHandler {
	waitTime := time.Millisecond * 200
	maxWaitTime := waitTime * 10
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			workspace, ok := evt.Object.(*v1.Workspace)
			if !ok {
				return
			}
			op := func() error {
				return r.createDataPlaneResources(ctx, workspace)
			}
			if err := backoff.Retry(op, maxWaitTime, waitTime); err != nil {
				klog.Error(err.Error())
			}
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkspace, ok1 := evt.ObjectOld.(*v1.Workspace)
			newWorkspace, ok2 := evt.ObjectNew.(*v1.Workspace)
			if !ok1 || !ok2 {
				return
			}
			op := func() error {
				return r.updateDataPlaneResources(ctx, oldWorkspace, newWorkspace)
			}
			if err := backoff.Retry(op, maxWaitTime, waitTime); err != nil {
				klog.Error(err.Error())
			}
			// Since workspace resource updates may impact scheduling decisions, a rescheduling reconciliation is triggered.
			if !quantity.Equal(oldWorkspace.Status.AvailableResources, newWorkspace.Status.AvailableResources) {
				r.Add(&SchedulerMessage{
					ClusterId:   newWorkspace.Spec.Cluster,
					WorkspaceId: newWorkspace.Name,
				})
			}
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			workspace, ok := evt.Object.(*v1.Workspace)
			if !ok {
				return
			}
			op := func() error {
				return r.deleteDataPlaneResources(ctx, workspace)
			}
			if err := backoff.Retry(op, maxWaitTime, waitTime); err != nil {
				klog.Error(err.Error())
			}
		},
	}
}

func (r *SchedulerReconciler) createDataPlaneResources(ctx context.Context, workspace *v1.Workspace) error {
	clusterInformer, err := syncer.GetClusterInformer(r.clusterInformers, workspace.Spec.Cluster)
	if err != nil {
		return err
	}
	clientSet := clusterInformer.ClientFactory().ClientSet()
	// create namespace for data plane
	if err = jobutils.CreateNamespace(ctx, workspace.Name, clientSet); err != nil {
		return err
	}
	// copy image secret from admin plane to data plane
	for _, s := range workspace.Spec.ImageSecrets {
		secret, err := r.getAdminSecret(ctx, s.Name)
		if err != nil {
			continue
		}
		if err = jobutils.CopySecret(ctx, clientSet, secret, workspace.Name); err != nil {
			return err
		}
	}
	// create pvc for data plane
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		pvc, err := r.generatePVC(&vol, workspace)
		if err != nil {
			klog.Error(err.Error())
			continue
		}
		if err = jobutils.CreatePVC(ctx, pvc, clientSet); err != nil {
			return err
		}
	}
	return nil
}

func (r *SchedulerReconciler) updateDataPlaneResources(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	if !reflect.DeepEqual(oldWorkspace.Spec.Volumes, newWorkspace.Spec.Volumes) {
		if err := r.updateDataPlanePvc(ctx, oldWorkspace, newWorkspace); err != nil {
			return err
		}
	}

	if !reflect.DeepEqual(oldWorkspace.Spec.ImageSecrets, newWorkspace.Spec.ImageSecrets) {
		if err := r.updateDataPlaneSecrets(ctx, oldWorkspace, newWorkspace); err != nil {
			return err
		}
	}
	return nil
}

func (r *SchedulerReconciler) updateDataPlanePvc(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	informer, err := syncer.GetClusterInformer(r.clusterInformers, newWorkspace.Spec.Cluster)
	if err != nil {
		return err
	}

	oldPvcSets := sets.NewSet()
	for _, vol := range oldWorkspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		oldPvcSets.Insert(vol.GenFullVolumeId())
	}
	newPvcSets := sets.NewSet()
	clientSet := informer.ClientFactory().ClientSet()
	for _, vol := range newWorkspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		volumeId := vol.GenFullVolumeId()
		newPvcSets.Insert(volumeId)
		if oldPvcSets.Has(volumeId) {
			continue
		}
		pvc, err := r.generatePVC(&vol, newWorkspace)
		if err != nil {
			klog.Error(err.Error())
			continue
		}
		if err = jobutils.CreatePVC(ctx, pvc, clientSet); err != nil {
			return err
		}
	}
	for _, vol := range oldWorkspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		volumeId := vol.GenFullVolumeId()
		if newPvcSets.Has(volumeId) {
			continue
		}
		if err = jobutils.DeletePVC(ctx, volumeId, newWorkspace.Name, clientSet); err != nil {
			return err
		}
	}
	return nil
}

func (r *SchedulerReconciler) updateDataPlaneSecrets(ctx context.Context, oldWorkspace, newWorkspace *v1.Workspace) error {
	informer, err := syncer.GetClusterInformer(r.clusterInformers, newWorkspace.Spec.Cluster)
	if err != nil {
		return err
	}
	clientSet := informer.ClientFactory().ClientSet()

	oldSecretMap := make(map[string]string)
	for _, s := range oldWorkspace.Spec.ImageSecrets {
		oldSecretMap[s.Name] = s.ResourceVersion
	}
	newSecretSet := sets.NewSet()
	for _, s := range newWorkspace.Spec.ImageSecrets {
		newSecretSet.Insert(s.Name)
		secret, err := r.getAdminSecret(ctx, s.Name)
		if err != nil {
			continue
		}
		oldSecretVersion, ok := oldSecretMap[s.Name]
		if ok {
			if oldSecretVersion == s.ResourceVersion {
				continue
			}
			if err = jobutils.UpdateSecret(ctx, clientSet, secret, newWorkspace.Name); err != nil {
				return err
			}
		} else {
			if err = jobutils.CopySecret(ctx, clientSet, secret, newWorkspace.Name); err != nil {
				return err
			}
		}
	}
	for _, s := range oldWorkspace.Spec.ImageSecrets {
		if newSecretSet.Has(s.Name) {
			continue
		}
		if err = jobutils.DeleteSecret(ctx, clientSet, s.Name, newWorkspace.Name); err != nil {
			return err
		}
	}
	return nil
}

func (r *SchedulerReconciler) deleteDataPlaneResources(ctx context.Context, workspace *v1.Workspace) error {
	informer, err := syncer.GetClusterInformer(r.clusterInformers, workspace.Spec.Cluster)
	if err != nil {
		return err
	}
	clientSet := informer.ClientFactory().ClientSet()
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.HOSTPATH {
			continue
		}
		if err = jobutils.DeletePVC(ctx, vol.GenFullVolumeId(), workspace.Name, clientSet); err != nil {
			return err
		}
	}
	for _, s := range workspace.Spec.ImageSecrets {
		if err = jobutils.DeleteSecret(ctx, clientSet, s.Name, workspace.Name); err != nil {
			return err
		}
	}
	if err = jobutils.DeleteNamespace(ctx, workspace.Name, clientSet); err != nil {
		return err
	}
	return nil
}

func (r *SchedulerReconciler) deleteDataPlaneNamespace(ctx context.Context, targetNamespace, clusterId string) error {
	informer, err := syncer.GetClusterInformer(r.clusterInformers, clusterId)
	if err != nil {
		return err
	}
	if err = jobutils.DeleteNamespace(ctx, targetNamespace, informer.ClientFactory().ClientSet()); err != nil {
		return err
	}
	return nil
}

func (r *SchedulerReconciler) generatePVC(volume *v1.WorkspaceVolume,
	workspace *v1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.SetName(volume.GenFullVolumeId())
	pvc.SetNamespace(workspace.Name)
	if len(volume.Selector) > 0 {
		pvc.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: volume.Selector,
		}
	} else {
		pvc.Spec.StorageClassName = pointer.String(volume.StorageClass)
	}

	storeQuantity, err := resource.ParseQuantity(volume.Capacity)
	if err != nil {
		return nil, err
	}
	pvc.Spec.Resources = corev1.VolumeResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceStorage: storeQuantity,
		},
	}
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{volume.AccessMode}
	volumeMode := corev1.PersistentVolumeFilesystem
	pvc.Spec.VolumeMode = &volumeMode
	return pvc, nil
}

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
	msg := &SchedulerMessage{
		ClusterId:   v1.GetClusterId(workload),
		WorkspaceId: workload.Spec.Workspace,
		WorkloadId:  workload.Name,
	}
	r.Add(msg)
	return ctrlruntime.Result{}, nil
}

func (r *SchedulerReconciler) delete(ctx context.Context, adminWorkload *v1.Workload) (ctrlruntime.Result, error) {
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

// Start implement Runnable interface in controller runtime package.
func (r *SchedulerReconciler) start(ctx context.Context) {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
}

func (r *SchedulerReconciler) Do(ctx context.Context, message *SchedulerMessage) (ctrlruntime.Result, error) {
	err := r.do(ctx, message)
	if utils.IsNonRetryableError(err) {
		err = nil
	}
	return ctrlruntime.Result{}, err
}

func (r *SchedulerReconciler) do(ctx context.Context, message *SchedulerMessage) error {
	workspace, err := r.getWorkspace(ctx, message.ClusterId, message.WorkspaceId)
	if workspace == nil {
		return err
	}

	schedulingWorkloads, runningWorkloads, err := r.getUnfinishedWorkloads(ctx, workspace)
	if err != nil || len(schedulingWorkloads) == 0 {
		return err
	}
	leftAvailResources, leftTotalResources, err := r.getLeftTotalResources(ctx, workspace, runningWorkloads)
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
		ok, reason, err := r.isCanSchedule(ctx, w, runningWorkloads, requestResources, *leftResources)
		if err != nil {
			return err
		}
		if !ok {
			unScheduledReasons[w.Name] = reason
			// If the scheduling policy is FIFO, or the priority is higher than subsequent queued workloads,
			// then break out of the queue directly and continue waiting.
			if workspace.IsEnableFifo() ||
				(i < len(schedulingWorkloads)-1 && w.Spec.Priority > schedulingWorkloads[i+1].Spec.Priority) {
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
		runningWorkloads = append(runningWorkloads, schedulingWorkloads[i])
		scheduledCount++
	}
	if scheduledCount != len(schedulingWorkloads) {
		r.updateUnScheduled(ctx, schedulingWorkloads, unScheduledReasons)
	}
	return nil
}

func (r *SchedulerReconciler) isCanSchedule(ctx context.Context, requestWorkload *v1.Workload,
	runningWorkloads []*v1.Workload, requestResources, leftResources corev1.ResourceList) (bool, string, error) {
	hasEnoughQuota, key := quantity.IsSubResource(requestResources, leftResources)
	var reason string
	var err error
	isPreemptable := false
	if !hasEnoughQuota {
		reason = fmt.Sprintf("Insufficient total %s resources", formatResourceName(key))
		isPreemptable, err = r.preempt(ctx, requestWorkload, runningWorkloads, leftResources)
	} else {
		hasEnoughQuota, reason, err = r.checkNodeResources(ctx, requestWorkload, runningWorkloads)
		if !hasEnoughQuota {
			isPreemptable = r.isCanPreempt(requestWorkload, runningWorkloads)
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

// Check if each node's available resources satisfy the workload's resource requests.
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

func (r *SchedulerReconciler) getWorkspace(ctx context.Context, clusterId, workspaceId string) (*v1.Workspace, error) {
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: workspaceId}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			if err = r.deleteDataPlaneNamespace(ctx, workspaceId, clusterId); err != nil {
				klog.Error(err.Error())
			}
			err = nil
		}
		return nil, err
	}
	return workspace, nil
}

func (r *SchedulerReconciler) getAdminSecret(ctx context.Context, secretId string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretId, Namespace: common.PrimusSafeNamespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// Retrieve the list of unfinished workloads, sorted by priority and other criteria, including both queued and running ones
func (r *SchedulerReconciler) getUnfinishedWorkloads(ctx context.Context, workspace *v1.Workspace) ([]*v1.Workload, []*v1.Workload, error) {
	filterFunc := func(w *v1.Workload) bool {
		return w.IsEnd()
	}
	workloads, err := commonworkload.GetWorkloadsOfWorkspace(ctx, r.Client,
		workspace.Spec.Cluster, []string{workspace.Name}, filterFunc)
	if err != nil {
		return nil, nil, err
	}
	var schedulingWorkloads, runningWorkloads []*v1.Workload
	for i, w := range workloads {
		if !v1.IsWorkloadScheduled(w) {
			schedulingWorkloads = append(schedulingWorkloads, workloads[i])
		} else {
			runningWorkloads = append(runningWorkloads, workloads[i].DeepCopy())
		}
	}
	if len(schedulingWorkloads) > 0 {
		sort.Sort(WorkloadList(schedulingWorkloads))
	}
	if len(runningWorkloads) > 0 {
		sort.Sort(WorkloadList(runningWorkloads))
	}
	return schedulingWorkloads, runningWorkloads, nil
}

// Retrieve the total amount of left resources. The system usually reserves a certain amount of CPU, memory, and other resources.
func (r *SchedulerReconciler) getLeftTotalResources(ctx context.Context,
	workspace *v1.Workspace, runningWorkloads []*v1.Workload) (corev1.ResourceList, corev1.ResourceList, error) {
	filterFunc := func(nodeName string) bool {
		n := &v1.Node{}
		if err := r.Get(ctx, client.ObjectKey{Name: nodeName}, n); err != nil {
			return true
		}
		return !n.IsAvailable(false)
	}
	usedResource := make(corev1.ResourceList)
	for _, w := range runningWorkloads {
		var resourceList corev1.ResourceList
		var err error
		if w.IsRunning() {
			resourceList, err = commonworkload.GetActiveResources(w, filterFunc)
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
	return leftAvailResource, leftTotalResource, nil
}

func (r *SchedulerReconciler) updateScheduled(ctx context.Context, workload *v1.Workload) error {
	maxRetry := 3
	if err := backoff.ConflictRetry(func() error {
		err := r.updateStatus(ctx, workload)
		if err == nil {
			return nil
		}
		if apierrors.IsConflict(err) {
			r.Get(ctx, client.ObjectKey{Namespace: workload.Namespace, Name: workload.Name}, workload)
		}
		return err
	}, maxRetry, time.Millisecond*100); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", workload.Name)
		return err
	}

	patch := client.MergeFrom(workload.DeepCopy())
	annotations := workload.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[v1.WorkloadScheduledAnnotation] = time.Now().UTC().Format(time.RFC3339)
	delete(annotations, v1.WorkloadReScheduledAnnotation)
	workload.SetAnnotations(annotations)
	if err := r.Patch(ctx, workload, patch); err != nil {
		klog.ErrorS(err, "failed to patch workload", "name", workload.Name)
		return err
	}
	return nil
}

func (r *SchedulerReconciler) updateStatus(ctx context.Context, workload *v1.Workload) error {
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload) + 1)
	cond := jobutils.NewCondition(string(v1.AdminScheduled), "the workload is scheduled", reason)
	if jobutils.FindCondition(workload, cond) != nil {
		return nil
	}
	workload.Status.Conditions = append(workload.Status.Conditions, *cond)
	workload.Status.SchedulerOrder = 0
	if workload.Status.Phase == "" {
		workload.Status.Phase = v1.WorkloadPending
	}
	if err := r.Status().Update(ctx, workload); err != nil {
		return err
	}
	return nil
}

func (r *SchedulerReconciler) updateUnScheduled(ctx context.Context, workloads []*v1.Workload, unScheduledReasons map[string]string) {
	order := 1
	for i, w := range workloads {
		if v1.IsWorkloadScheduled(w) {
			continue
		}
		patch := client.MergeFrom(workloads[i].DeepCopy())
		isChanged := false
		if workloads[i].Status.SchedulerOrder != order {
			workloads[i].Status.SchedulerOrder = order
			isChanged = true
		}
		reason, _ := unScheduledReasons[w.Name]
		if reason == "" && order > 1 {
			reason = "There are high priority or pre-created tasks that have not been scheduled yet"
		}
		if reason != workloads[i].Status.Message {
			workloads[i].Status.Message = reason
			isChanged = true
		}
		if isChanged {
			if err := r.Status().Patch(ctx, workloads[i], patch); err != nil {
				klog.ErrorS(err, "failed to patch workload", "name", workloads[i].Name)
			}
		}
		order++
	}
}
