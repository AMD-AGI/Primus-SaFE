/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	UnifiedJobInput  = "unified-job-input"
	UnifiedJobOutput = "unified-job-output"
	LightHousePort   = 29510
)

// DispatcherReconciler reconciles Workload objects and handles their dispatching to target clusters.
type DispatcherReconciler struct {
	client.Client
	clusterClientSets *commonutils.ObjectManager
}

// SetupDispatcherController initializes and registers the dispatcher controller with the manager.
func SetupDispatcherController(mgr manager.Manager) error {
	r := &DispatcherReconciler{
		Client:            mgr.GetClient(),
		clusterClientSets: commonutils.NewObjectManagerSingleton(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workload{}, builder.WithPredicates(relevantChangePredicate{})).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Dispatcher Controller successfully")
	return nil
}

type relevantChangePredicate struct {
	predicate.Funcs
}

// Create determines if a Create event should be processed for a Workload.
func (relevantChangePredicate) Create(e event.CreateEvent) bool {
	w, ok := e.Object.(*v1.Workload)
	if !ok {
		return false
	}
	if shouldDispatch(w) {
		return true
	}
	return false
}

// Update determines if an Update event should be processed for a Workload.
func (relevantChangePredicate) Update(e event.UpdateEvent) bool {
	oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
	newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
	if !ok1 || !ok2 {
		return false
	}
	if !shouldDispatch(oldWorkload) && shouldDispatch(newWorkload) {
		return true
	}
	if v1.IsWorkloadDispatched(newWorkload) {
		oldGroup, _ := commonworkload.GetReplicaGroup(oldWorkload, common.ReplicaGroup)
		newGroup, _ := commonworkload.GetReplicaGroup(newWorkload, common.ReplicaGroup)
		if oldGroup != newGroup {
			return true
		}

		if v1.GetGithubSecretId(oldWorkload) != v1.GetGithubSecretId(newWorkload) {
			return true
		}
		if !commonworkload.IsResourceEqual(oldWorkload, newWorkload) {
			return true
		}
		for i := range oldWorkload.Spec.Resources {
			if oldWorkload.Spec.Resources[i].SharedMemory != newWorkload.Spec.Resources[i].SharedMemory {
				return true
			}
		}
		if oldWorkload.Spec.Image != newWorkload.Spec.Image {
			return true
		}
		if oldWorkload.Spec.EntryPoint != newWorkload.Spec.EntryPoint {
			return true
		}
		if !maps.EqualIgnoreOrder(oldWorkload.Spec.Env, newWorkload.Spec.Env) || len(v1.GetEnvToBeRemoved(newWorkload)) > 0 {
			return true
		}
		if oldWorkload.Spec.Priority != newWorkload.Spec.Priority {
			return true
		}
		if !reflect.DeepEqual(oldWorkload.Spec.Service, newWorkload.Spec.Service) {
			return true
		}
	}
	return false
}

// Reconcile is the main control loop for Workload resources that triggers dispatching.
func (r *DispatcherReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	workload := new(v1.Workload)
	var err error
	if err = r.Get(ctx, req.NamespacedName, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !workload.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}
	// To prevent port conflicts when retrying, the port must be regenerated each time
	if err = r.generateUniquePorts(ctx, workload); err != nil {
		return ctrlruntime.Result{}, err
	}

	var result ctrlruntime.Result
	if commonworkload.IsTorchFT(workload) {
		result, err = r.processTorchFTWorkload(ctx, workload)
	} else {
		result, err = r.processWorkload(ctx, workload)
	}

	if err != nil {
		klog.ErrorS(err, "failed to dispatch workload", "name", workload.Name)
		if jobutils.IsUnrecoverableError(err) {
			err = jobutils.SetWorkloadFailed(ctx, r.Client, workload, err.Error())
		}
	}
	return result, err
}

// processTorchFTWorkload processes a TorchFT workload resource, handling both scale-up and scale-down.
func (r *DispatcherReconciler) processTorchFTWorkload(ctx context.Context, rootWorkload *v1.Workload) (ctrlruntime.Result, error) {
	if commonconfig.GetTorchFTLightHouse() == "" {
		return ctrlruntime.Result{}, commonerrors.NewInternalError("TorchFT LightHouse is not configured")
	}
	group, err := commonworkload.GetReplicaGroup(rootWorkload, common.ReplicaGroup)
	if err != nil {
		return ctrlruntime.Result{}, commonerrors.NewBadRequest("invalid replica process group")
	}

	// Handle scale-down: delete jobs that exceed the current group count
	if err = r.scaleDownTorchFTWorkers(ctx, rootWorkload, group); err != nil {
		return ctrlruntime.Result{}, err
	}

	lightHouseWorkload := r.generateLighthouse(ctx, rootWorkload)
	if result, err := r.processWorkload(ctx, lightHouseWorkload); err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	lightHouseAddr := lightHouseWorkload.Name + "." + rootWorkload.Spec.Workspace + ".svc.cluster.local"

	for i := 0; i < group; i++ {
		torchFTWorkload := r.generateTorchFTWorker(ctx, rootWorkload, i, group, lightHouseAddr)
		if result, err := r.processWorkload(ctx, torchFTWorkload); err != nil || result.RequeueAfter > 0 {
			return result, err
		}
	}
	if err = r.markAsDispatched(ctx, rootWorkload); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// scaleDownTorchFTWorkers handles scale-down by deleting TorchFT jobs that exceed the target group count.
// It parses the index from each object's name and deletes those with index > targetGroup.
func (r *DispatcherReconciler) scaleDownTorchFTWorkers(ctx context.Context, rootWorkload *v1.Workload, targetGroup int) error {
	clientSets, err := syncer.GetClusterClientSets(r.clusterClientSets, v1.GetClusterId(rootWorkload))
	if err != nil {
		return err
	}

	// List all TorchFT jobs owned by this root workload
	workloadGVKs := commonworkload.GetWorkloadGVK(rootWorkload)
	var gvk schema.GroupVersionKind
	for _, gvk = range workloadGVKs {
		if gvk.Kind == common.PytorchJobKind {
			break
		}
	}
	labelSelector := v1.WorkloadIdLabel + "=" + rootWorkload.Name
	unstructuredObjs, err := jobutils.ListObject(ctx,
		clientSets.ClientFactory(), labelSelector, rootWorkload.Spec.Workspace, gvk)
	if err != nil {
		return err
	}
	if len(unstructuredObjs) <= targetGroup {
		return nil
	}

	// Name format: {displayName}-{index}-{suffix}
	// Index 0 = lighthouse (ignore), Index 1 to totalGroups = TorchFT workers
	for _, obj := range unstructuredObjs {
		index, ok := jobutils.ParseTorchFTGroupIndex(obj.GetName())
		if !ok {
			continue
		}
		if index > targetGroup {
			klog.Infof("scaling down TorchFT: deleting sub-workload %s (index %d)", obj.GetName(), index)
			if err = jobutils.DeleteObject(ctx, clientSets.ClientFactory(), &obj); err != nil {
				klog.ErrorS(err, "failed to delete object", "name", obj.GetName())
				return err
			}
		}
	}
	return nil
}

// processWorkload processes a workload resource and updates its state.
func (r *DispatcherReconciler) processWorkload(ctx context.Context, adminWorkload *v1.Workload) (ctrlruntime.Result, error) {
	clientSets, err := syncer.GetClusterClientSets(r.clusterClientSets, v1.GetClusterId(adminWorkload))
	if err != nil {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	rt, err := commonworkload.GetResourceTemplate(ctx, r.Client, adminWorkload)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	obj, err := jobutils.GetObject(ctx,
		clientSets.ClientFactory(), adminWorkload.Name, adminWorkload.Spec.Workspace, rt.ToSchemaGVK())

	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrlruntime.Result{}, err
		}
		if result, err := r.dispatch(ctx, adminWorkload, clientSets); err != nil || result.RequeueAfter > 0 {
			return result, err
		}
		if err = r.markAsDispatched(ctx, adminWorkload); err != nil {
			return ctrlruntime.Result{}, err
		}
		klog.Infof("the workload is dispatched, name: %s, dispatch count: %d, max retry: %d",
			adminWorkload.Name, v1.GetWorkloadDispatchCnt(adminWorkload), adminWorkload.Spec.MaxRetry)
	} else {
		if err = r.markAsDispatched(ctx, adminWorkload); err != nil {
			return ctrlruntime.Result{}, err
		}
		if commonworkload.IsApplication(adminWorkload) || commonworkload.IsCICDScalingRunnerSet(adminWorkload) {
			// update the workload which is already dispatched
			if err = r.syncWorkloadToObject(ctx, adminWorkload, clientSets, obj); err != nil {
				return ctrlruntime.Result{}, err
			}
			// sync service according to latest spec
			if result, err := r.updateService(ctx, adminWorkload, clientSets, obj); err != nil || result.RequeueAfter > 0 {
				return result, err
			}
			// Sync corresponding ingress
			if result, err := r.updateIngress(ctx, adminWorkload, clientSets, obj); err != nil || result.RequeueAfter > 0 {
				return result, err
			}
			klog.Infof("the workload is updated, name: %s, dispatch count: %d, max retry: %d",
				adminWorkload.Name, v1.GetWorkloadDispatchCnt(adminWorkload), adminWorkload.Spec.MaxRetry)
		}
	}
	return ctrlruntime.Result{}, nil
}

// dispatch creates and deploys the Kubernetes object in the data plane for the workload.
func (r *DispatcherReconciler) dispatch(ctx context.Context,
	adminWorkload *v1.Workload, clientSets *syncer.ClusterClientSets) (ctrlruntime.Result, error) {
	k8sObject, err := r.generateK8sObject(ctx, adminWorkload, clientSets)
	if err != nil {
		klog.ErrorS(err, "failed to create k8s unstructured object. ",
			"name", adminWorkload.Name, "gvk", adminWorkload.Spec.GroupVersionKind)
		return ctrlruntime.Result{}, err
	}
	if err = jobutils.CreateObject(ctx, clientSets.ClientFactory(), k8sObject); err != nil {
		return ctrlruntime.Result{}, err
	}
	if result, err := r.createService(ctx, adminWorkload, clientSets, k8sObject); err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	// Ensure an ingress that points to the same-named Service
	return r.createIngress(ctx, adminWorkload, clientSets, k8sObject)
}

// generateUniquePorts generates unique job and SSH ports for the workload to avoid conflicts.
func (r *DispatcherReconciler) generateUniquePorts(ctx context.Context, workload *v1.Workload) error {
	if v1.IsWorkloadDispatched(workload) {
		return nil
	}
	rand.Seed(time.Now().UnixNano())
	ports := make(map[int]bool)

	workloadList := &v1.WorkloadList{}
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: v1.GetClusterId(workload)})
	// Record currently in-use ports to avoid reuse
	if r.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector}) == nil {
		for _, item := range workloadList.Items {
			ports[item.Spec.JobPort] = true
			ports[item.Spec.SSHPort] = true
		}
	}
	patch := client.MergeFrom(workload.DeepCopy())
	if workload.Spec.Service != nil {
		workload.Spec.JobPort = workload.Spec.Service.TargetPort
	} else {
		workload.Spec.JobPort = generateRandomPort(ports)
	}
	workload.Spec.SSHPort = generateRandomPort(ports)
	if workload.Spec.JobPort == 0 || workload.Spec.SSHPort == 0 {
		return commonerrors.NewInternalError("failed to generate job or SSH port")
	}
	if err := r.Patch(ctx, workload, patch); err != nil {
		return err
	}
	return nil
}

// generateK8sObject creates the unstructured Kubernetes object from the workload specification.
func (r *DispatcherReconciler) generateK8sObject(ctx context.Context,
	adminWorkload *v1.Workload, clientSets *syncer.ClusterClientSets) (*unstructured.Unstructured, error) {
	workspace, err := r.getWorkspace(ctx, adminWorkload)
	if err != nil {
		return nil, err
	}

	rt, err := commonworkload.GetResourceTemplate(ctx, r.Client, adminWorkload)
	if err != nil {
		klog.Error(err.Error())
		return nil, err
	}

	result, err := r.getWorkloadTemplate(ctx, adminWorkload)
	if err != nil {
		klog.Error(err.Error())
		return nil, commonerrors.NewInternalError(err.Error())
	}
	if err = applyWorkloadSpecToObject(ctx, clientSets, result, adminWorkload, workspace, rt); err != nil {
		return nil, commonerrors.NewInternalError(err.Error())
	}
	for i, t := range rt.Spec.ResourceSpecs {
		if i >= len(adminWorkload.Spec.Resources) {
			break
		}
		if err = initializeObject(result, adminWorkload, workspace, &t, i); err != nil {
			return nil, commonerrors.NewInternalError(err.Error())
		}
	}
	setK8sObjectMeta(result, adminWorkload)
	return result, nil
}

func setK8sObjectMeta(result *unstructured.Unstructured, adminWorkload *v1.Workload) {
	result.SetName(adminWorkload.Name)
	result.SetNamespace(adminWorkload.Spec.Workspace)

	targetLabels := result.GetLabels()
	if len(targetLabels) == 0 {
		targetLabels = make(map[string]string)
	}
	for key, val := range buildObjectLabels(adminWorkload) {
		if strValue, ok := val.(string); ok {
			targetLabels[key] = strValue
		}
	}
	result.SetLabels(targetLabels)

	targetAnnotations := result.GetAnnotations()
	if len(targetAnnotations) == 0 {
		targetAnnotations = make(map[string]string)
	}
	for key, val := range buildObjectAnnotations(adminWorkload) {
		if strValue, ok := val.(string); ok {
			targetAnnotations[key] = strValue
		}
	}
	result.SetAnnotations(targetAnnotations)
}

// getWorkloadTemplate retrieves the workload template configuration based on its version and kind.
func (r *DispatcherReconciler) getWorkloadTemplate(ctx context.Context, adminWorkload *v1.Workload) (*unstructured.Unstructured, error) {
	templateConfig, err := commonworkload.GetWorkloadTemplate(ctx, r.Client, adminWorkload.ToSchemaGVK())
	if err != nil {
		return nil, err
	}
	templateStr, ok := templateConfig.Data["template"]
	if !ok || templateStr == "" {
		return nil, fmt.Errorf("failed to find the template. name: %s", templateConfig.Name)
	}
	template, err := jsonutils.ParseYamlToJson(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %v", err.Error())
	}
	return template, nil
}

// markAsDispatched updates a workload's status to indicate it has been dispatched.
func (r *DispatcherReconciler) markAsDispatched(ctx context.Context, workload *v1.Workload) error {
	if v1.GetRootWorkloadId(workload) != "" {
		return nil
	}
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload) + 1)
	cond := jobutils.NewCondition(string(v1.AdminDispatched), "the workload is dispatched", reason)
	if jobutils.FindCondition(workload, cond) == nil {
		statusPatch := map[string]any{}
		if workload.Status.Phase == "" {
			statusPatch["phase"] = v1.WorkloadPending
		}
		statusPatch["conditions"] = append(workload.Status.Conditions, *cond)
		patchObj := map[string]any{
			"metadata": map[string]any{
				"resourceVersion": workload.ResourceVersion,
			},
			"status": statusPatch,
		}
		p := jsonutils.MarshalSilently(patchObj)
		if err := r.Status().Patch(ctx, workload, client.RawPatch(apitypes.MergePatchType, p)); err != nil {
			return err
		}
	}

	if !v1.IsWorkloadDispatched(workload) {
		patch := client.MergeFrom(workload.DeepCopy())
		v1.RemoveAnnotation(workload, v1.WorkloadPreemptedAnnotation)
		v1.RemoveAnnotation(workload, v1.EnvToBeRemovedAnnotation)
		v1.SetAnnotation(workload, v1.WorkloadDispatchedAnnotation, timeutil.FormatRFC3339(time.Now().UTC()))
		v1.SetLabel(workload, v1.WorkloadDispatchCntLabel, buildDispatchCount(workload))
		if err := r.Patch(ctx, workload, patch); err != nil {
			return err
		}
	}
	return nil
}

// syncWorkloadToObject synchronizes workload spec changes to the corresponding Kubernetes object.
// It checks for significant changes and updates the object in the data plane cluster if needed.
func (r *DispatcherReconciler) syncWorkloadToObject(ctx context.Context, adminWorkload *v1.Workload,
	clientSets *syncer.ClusterClientSets, obj *unstructured.Unstructured) error {
	rt, err := commonworkload.GetResourceTemplate(ctx, r.Client, adminWorkload)
	if err != nil {
		klog.ErrorS(err, "", "gvk", adminWorkload.Spec.GroupVersionKind)
		return err
	}
	if len(rt.Spec.ResourceSpecs) == 0 {
		return nil
	}

	functions := []func(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool{
		isResourceChanged, isImageChanged, isEntryPointChanged, isSharedMemoryChanged,
		isEnvChanged, isPriorityClassChanged, isGithubSecretChanged,
	}
	isChanged := false
	for _, f := range functions {
		if isChanged = f(adminWorkload, obj, rt); isChanged {
			break
		}
	}
	if !isChanged {
		return nil
	}

	workspace, err := r.getWorkspace(ctx, adminWorkload)
	if err != nil {
		return err
	}
	if err = applyWorkloadSpecToObject(ctx, clientSets, obj, adminWorkload, workspace, rt); err != nil {
		return commonerrors.NewBadRequest(err.Error())
	}

	if err = jobutils.UpdateObject(ctx, clientSets.ClientFactory(), obj); err != nil {
		klog.ErrorS(err, "failed to update k8s unstructured object")
		return err
	}
	patch := client.MergeFrom(adminWorkload.DeepCopy())
	v1.RemoveAnnotation(adminWorkload, v1.EnvToBeRemovedAnnotation)
	if err = r.Patch(ctx, adminWorkload, patch); err != nil {
		return err
	}
	return nil
}

// isResourceChanged checks if the resource requirements of the workload have changed.
func isResourceChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	gpuName := ""
	for _, res := range adminWorkload.Spec.Resources {
		if res.GPU != "" {
			gpuName = res.GPUName
			break
		}
	}

	replicaList, resourceList, err := jobutils.GetResources(obj, rt, v1.GetMainContainer(adminWorkload), gpuName)
	if err != nil {
		klog.ErrorS(err, "failed to get resource", "rt", rt.Name, "obj", obj.GetName())
		return false
	}
	if len(replicaList) == len(adminWorkload.Spec.Resources) {
		for i := range replicaList {
			if replicaList[i] != int64(adminWorkload.Spec.Resources[i].Replica) {
				return true
			}
		}
	}

	if len(resourceList) == len(adminWorkload.Spec.Resources) {
		for i := range resourceList {
			podResource, err := commonworkload.GetPodResourceList(&adminWorkload.Spec.Resources[i])
			if err != nil {
				klog.ErrorS(err, "failed to get pod resource", "resource", adminWorkload.Spec.Resources[i])
				return false
			}
			if !quantity.Equal(podResource, resourceList[i]) {
				return true
			}
		}
	}
	return false
}

// isImageChanged checks if the container image of the workload has changed.
func isImageChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	image, err := jobutils.GetImage(obj, rt, v1.GetMainContainer(adminWorkload))
	if err != nil {
		klog.ErrorS(err, "failed to get image", "obj", obj.GetName())
		return false
	}
	return adminWorkload.Spec.Image != image
}

// isEntryPointChanged checks if the entry point/command of the workload has changed.
func isEntryPointChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	commands, err := jobutils.GetCommand(obj, rt, v1.GetMainContainer(adminWorkload))
	if err != nil {
		klog.ErrorS(err, "failed to get command", "obj", obj.GetName())
		return false
	}
	if len(commands) == 0 {
		return false
	}
	cmd := buildEntryPoint(adminWorkload)
	return cmd != commands[len(commands)-1]
}

// isEnvChanged checks if the environment variables of the workload have changed.
func isEnvChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	if len(v1.GetEnvToBeRemoved(adminWorkload)) > 0 {
		return true
	}
	mainContainerName := v1.GetMainContainer(adminWorkload)
	currentEnvs, err := jobutils.GetEnv(obj, rt, mainContainerName)
	if err != nil {
		klog.ErrorS(err, "failed to get env", "obj", obj.GetName())
		return false
	}
	currentEnvsMap := convertEnvsToStringMap(currentEnvs)
	return !maps.Contain(currentEnvsMap, adminWorkload.Spec.Env)
}

// isSharedMemoryChanged checks if the shared memory configuration of the workload has changed.
func isSharedMemoryChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	memoryStorageSizes, err := jobutils.GetMemoryStorageSize(obj, rt)
	if err != nil {
		return true
	}
	if len(memoryStorageSizes) != len(adminWorkload.Spec.Resources) {
		return true
	}
	for i := range memoryStorageSizes {
		if memoryStorageSizes[i] != adminWorkload.Spec.Resources[i].SharedMemory {
			return true
		}
	}
	return false
}

// isPriorityClassChanged checks if the priority of the workload has changed.
func isPriorityClassChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	priorityClassName, err := jobutils.GetPriorityClassName(obj, rt)
	if err != nil {
		return true
	}
	return commonworkload.GeneratePriorityClass(adminWorkload) != priorityClassName
}

// isGithubSecretChanged checks if the GitHub secret of the workload has changed.
func isGithubSecretChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, _ *v1.ResourceTemplate) bool {
	if !commonworkload.IsCICDScalingRunnerSet(adminWorkload) {
		return false
	}
	secretId, err := jobutils.GetGithubConfigSecret(obj)
	if err != nil {
		return true
	}
	return v1.GetGithubSecretId(adminWorkload) != secretId
}

// applyWorkloadSpecToObject applies the workload specifications to the unstructured Kubernetes object.
// It handles different workload types and updates various object properties including replicas,
// network settings, containers, and volumes based on the workload specification.
func applyWorkloadSpecToObject(ctx context.Context, clientSets *syncer.ClusterClientSets,
	obj *unstructured.Unstructured, adminWorkload *v1.Workload, workspace *v1.Workspace, rt *v1.ResourceTemplate) error {
	if commonworkload.IsCICDScalingRunnerSet(adminWorkload) {
		if err := updateCICDScaleSet(obj, adminWorkload, workspace, rt); err != nil {
			return err
		}
	} else if commonworkload.IsCICDEphemeralRunner(adminWorkload) {
		if err := updateCICDEphemeralRunner(ctx, clientSets, obj, adminWorkload, rt); err != nil {
			return err
		}
	}

	for i, t := range rt.Spec.ResourceSpecs {
		if i >= len(adminWorkload.Spec.Resources) {
			unstructured.RemoveNestedField(obj.Object, t.PrePaths...)
			continue
		}
		if err := updateHostNetwork(adminWorkload, obj, t, i); err != nil {
			return fmt.Errorf("failed to update host network: %v", err.Error())
		}
		if err := updateReplica(adminWorkload, obj, t, i); err != nil {
			return fmt.Errorf("failed to update replica: %v", err.Error())
		}
		if err := updateMetadata(adminWorkload, obj, t, i); err != nil {
			return fmt.Errorf("failed to update main container: %v", err.Error())
		}
		if err := updateContainers(adminWorkload, obj, t, i); err != nil {
			return fmt.Errorf("failed to update main container: %v", err.Error())
		}
		if err := updateSharedMemory(adminWorkload, obj, t, i); err != nil {
			return fmt.Errorf("failed to update shared memory: %v", err.Error())
		}
		if err := updatePriorityClass(adminWorkload, obj, t); err != nil {
			return fmt.Errorf("failed to update priority: %v", err.Error())
		}
	}
	return nil
}

// createService creates a Kubernetes Service for the workload if specified.
func (r *DispatcherReconciler) createService(ctx context.Context, adminWorkload *v1.Workload,
	clientSets *syncer.ClusterClientSets, obj *unstructured.Unstructured) (ctrlruntime.Result, error) {
	if adminWorkload.Spec.Service == nil {
		return ctrlruntime.Result{}, nil
	}
	k8sClientSet := clientSets.ClientFactory().ClientSet()
	namespace := adminWorkload.Spec.Workspace
	var err error
	if _, err = k8sClientSet.CoreV1().Services(namespace).Get(ctx, adminWorkload.Name, metav1.GetOptions{}); err == nil {
		return ctrlruntime.Result{}, nil
	}
	specService := adminWorkload.Spec.Service
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      adminWorkload.Name,
			Namespace: namespace,
			Annotations: map[string]string{
				v1.UserNameAnnotation: v1.GetUserName(adminWorkload),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				v1.WorkloadIdLabel: adminWorkload.Name,
			},
			Ports: generateServicePorts(specService),
			Type:  specService.ServiceType,
		},
	}

	// Only set owner reference when the owner object has a valid UID.
	owner := obj
	if len(owner.GetUID()) == 0 {
		if fetched, getErr := jobutils.GetObject(ctx, clientSets.ClientFactory(),
			obj.GetName(), obj.GetNamespace(), obj.GroupVersionKind()); getErr != nil {
			return ctrlruntime.Result{}, getErr
		} else {
			if len(fetched.GetUID()) == 0 {
				return ctrlruntime.Result{RequeueAfter: time.Second}, nil
			}
			owner = fetched
		}
	}

	if err = controllerutil.SetControllerReference(owner, service, r.Client.Scheme()); err != nil {
		klog.ErrorS(err, "failed to SetControllerReference")
		return ctrlruntime.Result{}, err
	}
	if specService.ServiceType == corev1.ServiceTypeNodePort && specService.NodePort > 0 {
		service.Spec.Ports[0].NodePort = int32(specService.NodePort)
	}

	if service, err = k8sClientSet.CoreV1().Services(namespace).Create(ctx,
		service, metav1.CreateOptions{}); client.IgnoreAlreadyExists(err) != nil {
		klog.ErrorS(err, "failed to create service", "name", adminWorkload.Name)
		if specService.NodePort > 0 {
			// NodePort error occurred; cannot retry.
			return ctrlruntime.Result{}, commonerrors.NewInternalError(err.Error())
		}
		return ctrlruntime.Result{}, err
	}
	klog.Infof("service %s/%s created", namespace, adminWorkload.Name)
	return ctrlruntime.Result{}, nil
}

// updateService ensures the Service matches the latest workload spec.f
// - If workload.Spec.Service == nil, the Service will be deleted if it exists.
// - If Service does not exist and spec is set, it will be created.
// - Otherwise, it will be updated in-place to match protocol/ports/type/selector.
func (r *DispatcherReconciler) updateService(ctx context.Context, adminWorkload *v1.Workload,
	clientSets *syncer.ClusterClientSets, obj *unstructured.Unstructured) (ctrlruntime.Result, error) {
	k8sClientSet := clientSets.ClientFactory().ClientSet()
	namespace := adminWorkload.Spec.Workspace

	// Delete when no service desired
	if adminWorkload.Spec.Service == nil {
		err := k8sClientSet.CoreV1().Services(namespace).Delete(ctx, adminWorkload.Name, metav1.DeleteOptions{})
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	// Get or create
	existing, err := k8sClientSet.CoreV1().Services(namespace).Get(ctx, adminWorkload.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return r.createService(ctx, adminWorkload, clientSets, obj)
	}
	if err != nil {
		return ctrlruntime.Result{}, err
	}

	specService := adminWorkload.Spec.Service
	isChanged := false
	if existing.Spec.Type != specService.ServiceType {
		existing.Spec.Type = specService.ServiceType
		isChanged = true
	}
	newPorts := generateServicePorts(specService)
	if !reflect.DeepEqual(existing.Spec.Ports, newPorts) {
		existing.Spec.Ports = newPorts
		isChanged = true
	}
	if specService.ServiceType == corev1.ServiceTypeNodePort {
		if existing.Spec.Ports[0].NodePort != int32(specService.NodePort) {
			existing.Spec.Ports[0].NodePort = int32(specService.NodePort)
			isChanged = true
		}
	} else {
		// reset NodePort when not required
		if existing.Spec.Ports[0].NodePort != 0 {
			existing.Spec.Ports[0].NodePort = 0
			isChanged = true
		}
	}
	if !isChanged {
		return ctrlruntime.Result{}, nil
	}
	if _, err = k8sClientSet.CoreV1().Services(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		klog.ErrorS(err, "failed to update service", "name", adminWorkload.Name)
		if specService.NodePort > 0 {
			// NodePort related update errors are not retryable via generic update
			err = commonerrors.NewInternalError(err.Error())
		}
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// createIngress creates an Ingress in the same namespace that points to the same-named Service.
func (r *DispatcherReconciler) createIngress(ctx context.Context, adminWorkload *v1.Workload,
	clientSets *syncer.ClusterClientSets, obj *unstructured.Unstructured) (ctrlruntime.Result, error) {
	if adminWorkload.Spec.Service == nil || commonconfig.GetIngress() != common.HigressClassname {
		return ctrlruntime.Result{}, nil
	}
	k8sClientSet := clientSets.ClientFactory().ClientSet()
	namespace := adminWorkload.Spec.Workspace
	name := adminWorkload.Name
	if _, err := k8sClientSet.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		return ctrlruntime.Result{}, nil
	}
	specService := adminWorkload.Spec.Service
	pathType := networkingv1.PathTypePrefix
	path := "/" + namespace + "/" + name + "/(.*)"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				v1.UserNameAnnotation:       v1.GetUserName(adminWorkload),
				"higress.io/rewrite-target": "/$1",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: pointer.String(common.HigressClassname),
			Rules: []networkingv1.IngressRule{{
				Host: commonconfig.GetSystemHost(),
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     path,
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: name, Port: networkingv1.ServiceBackendPort{
										Number: int32(specService.Port),
									},
								},
							},
						}},
					},
				},
			}},
		},
	}
	// Only set owner reference when the owner object has a valid UID.
	owner := obj
	if len(owner.GetUID()) == 0 {
		if fetched, getErr := jobutils.GetObject(ctx, clientSets.ClientFactory(),
			obj.GetName(), obj.GetNamespace(), obj.GroupVersionKind()); getErr != nil {
			return ctrlruntime.Result{}, getErr
		} else {
			if len(fetched.GetUID()) == 0 {
				return ctrlruntime.Result{RequeueAfter: time.Second}, nil
			}
			owner = fetched
		}
	}
	if err := controllerutil.SetControllerReference(owner, ing, r.Client.Scheme()); err != nil {
		klog.ErrorS(err, "failed to SetControllerReference for ingress", "ingress", name)
		return ctrlruntime.Result{}, err
	}
	if _, err := k8sClientSet.NetworkingV1().Ingresses(namespace).Create(ctx, ing, metav1.CreateOptions{}); err != nil {
		return ctrlruntime.Result{}, client.IgnoreAlreadyExists(err)
	}
	klog.Infof("ingress %s/%s created", namespace, name)
	return ctrlruntime.Result{}, nil
}

// updateIngress makes sure the Ingress exists (or is removed) and points to the same-named Service and port.
func (r *DispatcherReconciler) updateIngress(ctx context.Context, adminWorkload *v1.Workload,
	clientSets *syncer.ClusterClientSets, obj *unstructured.Unstructured) (ctrlruntime.Result, error) {
	if commonconfig.GetIngress() != common.HigressClassname {
		return ctrlruntime.Result{}, nil
	}

	k8sClientSet := clientSets.ClientFactory().ClientSet()
	namespace := adminWorkload.Spec.Workspace
	name := adminWorkload.Name
	// Delete when service is not desired
	if adminWorkload.Spec.Service == nil {
		err := k8sClientSet.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	existing, err := k8sClientSet.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return r.createIngress(ctx, adminWorkload, clientSets, obj)
	}
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	specService := adminWorkload.Spec.Service
	if len(existing.Spec.Rules) > 0 && existing.Spec.Rules[0].HTTP != nil && len(existing.Spec.Rules[0].HTTP.Paths) > 0 {
		if existing.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number == int32(specService.Port) {
			return ctrlruntime.Result{}, nil
		}
		existing.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number = int32(specService.Port)
	} else {
		return ctrlruntime.Result{}, commonerrors.NewInternalError("no rules found in ingress")
	}
	if _, err = k8sClientSet.NetworkingV1().Ingresses(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	return ctrlruntime.Result{}, nil
}

func (r *DispatcherReconciler) getWorkspace(ctx context.Context, adminWorkload *v1.Workload) (*v1.Workspace, error) {
	workspace := &v1.Workspace{}
	if adminWorkload.Spec.Workspace != corev1.NamespaceDefault {
		err := r.Get(ctx, client.ObjectKey{Name: adminWorkload.Spec.Workspace}, workspace)
		if err != nil {
			return nil, err
		}
	}
	return workspace, nil
}

// generateRandomPort generates a random port number within the specified range.
// with a maximum of 200 retry attempts to avoid infinite loops.
func generateRandomPort(ports map[int]bool) int {
	maxRetries := 200
	for i := 0; i < maxRetries; i++ {
		port := rand.Intn(10000) + 20000
		_, ok := ports[port]
		if !ok {
			ports[port] = true
			return port
		}
	}
	klog.Errorf("Unable to generate unique port after %d attempts", maxRetries)
	return 0
}

// generateLighthouse generates a lighthouse workload for TorchFT, which is used for coordination and management of the worker process group
func (r *DispatcherReconciler) generateLighthouse(ctx context.Context, rootWorkload *v1.Workload) *v1.Workload {
	workload := rootWorkload.DeepCopy()
	displayName := v1.GetDisplayName(rootWorkload) + "-0"
	workload.Name = commonutils.GenerateName(displayName)
	v1.SetLabel(workload, v1.DisplayNameLabel, displayName)
	v1.SetLabel(workload, v1.RootWorkloadIdLabel, rootWorkload.Name)
	v1.SetAnnotation(workload, v1.ResourceIdAnnotation, "0")

	minGroup, _ := commonworkload.GetReplicaGroup(workload, common.MinReplicaGroup)
	entryPoint := stringutil.Base64Decode(commonconfig.GetTorchFTLightHouse())
	entryPoint = strings.TrimRight(entryPoint, "\n")
	entryPoint += fmt.Sprintf(" --min_replicas %d", minGroup)
	workload.Spec.EntryPoint = stringutil.Base64Encode(entryPoint)
	workload.Spec.Kind = common.DeploymentKind
	workload.Spec.Resources = []v1.WorkloadResource{rootWorkload.Spec.Resources[0]}
	workload.Spec.Service = &v1.Service{
		Protocol:    corev1.ProtocolTCP,
		Port:        LightHousePort,
		TargetPort:  LightHousePort,
		ServiceType: corev1.ServiceTypeClusterIP,
	}
	workload.Spec.Service.Extends = make(map[string]string)
	workload.Spec.Service.Extends["maxUnavailable"] = common.DefaultMaxUnavailable
	workload.Spec.Service.Extends["maxSurge"] = common.DefaultMaxMaxSurge

	commonworkload.GetWorkloadMainContainer(ctx, r.Client, workload)
	return workload
}

// generateTorchFTWorker generates a TorchFT worker. It uses PyTorchJob as the main entity and integrates with Lighthouse via environment variables.
func (r *DispatcherReconciler) generateTorchFTWorker(ctx context.Context,
	rootWorkload *v1.Workload, id, group int, lightHouseAddr string) *v1.Workload {
	workload := rootWorkload.DeepCopy()
	// The webhook has already validated the resources.
	nodePerGroup := rootWorkload.Spec.Resources[1].Replica / group
	displayName := v1.GetDisplayName(rootWorkload) + "-" + strconv.Itoa(id+1)
	workload.Name = commonutils.GenerateName(displayName)
	workload.Spec.Resources = []v1.WorkloadResource{rootWorkload.Spec.Resources[1]}
	workload.Spec.Resources[0].Replica = 1
	if nodePerGroup > 1 {
		workload.Spec.Resources = append(workload.Spec.Resources, rootWorkload.Spec.Resources[1])
		workload.Spec.Resources[1].Replica = nodePerGroup - 1
	}

	if workload.Spec.Env == nil {
		workload.Spec.Env = make(map[string]string)
	}
	workload.Spec.Env[common.TorchFTLightHouse] = lightHouseAddr

	entryPoint := stringutil.Base64Decode(workload.Spec.EntryPoint)
	entryPoint = strings.TrimRight(entryPoint, "\n")
	workload.Spec.EntryPoint = stringutil.Base64Encode(entryPoint + " --fault_tolerance.enable --fault_tolerance.replica_id=" +
		strconv.Itoa(id) + " --fault_tolerance.group_size=" + strconv.Itoa(group))
	workload.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	v1.SetLabel(workload, v1.DisplayNameLabel, displayName)
	v1.SetLabel(workload, v1.RootWorkloadIdLabel, rootWorkload.Name)
	v1.SetAnnotation(workload, v1.ResourceIdAnnotation, "1")
	commonworkload.GetWorkloadMainContainer(ctx, r.Client, workload)
	return workload
}

func generateServicePorts(specService *v1.Service) []corev1.ServicePort {
	return []corev1.ServicePort{{
		Protocol:   specService.Protocol,
		Port:       int32(specService.Port),
		TargetPort: intstr.IntOrString{IntVal: int32(specService.TargetPort)},
	}}
}

// shouldDispatch checks if a workload is ready to be dispatched.
func shouldDispatch(workload *v1.Workload) bool {
	if v1.IsWorkloadScheduled(workload) && !v1.IsWorkloadDispatched(workload) {
		return true
	}
	return false
}
