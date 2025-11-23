/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	UnifiedBuildInput  = "unified-build-input"
	UnifiedBuildOutput = "unified-build-output"
)

// DispatcherReconciler reconciles Workload objects and handles their dispatching to target clusters.
type DispatcherReconciler struct {
	client.Client
	clusterInformers *commonutils.ObjectManager
}

// SetupDispatcherController initializes and registers the dispatcher controller with the manager.
func SetupDispatcherController(mgr manager.Manager) error {
	r := &DispatcherReconciler{
		Client:           mgr.GetClient(),
		clusterInformers: commonutils.NewObjectManagerSingleton(),
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
	if isDispatchingJob(w) {
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
	if !isDispatchingJob(oldWorkload) && isDispatchingJob(newWorkload) {
		return true
	}
	if !commonworkload.IsResourceEqual(oldWorkload, newWorkload) ||
		oldWorkload.Spec.Resource.SharedMemory != newWorkload.Spec.Resource.SharedMemory {
		return true
	}
	if oldWorkload.Spec.Image != newWorkload.Spec.Image {
		return true
	}
	if oldWorkload.Spec.EntryPoint != newWorkload.Spec.EntryPoint {
		return true
	}
	if !maps.EqualIgnoreOrder(oldWorkload.Spec.Env, newWorkload.Spec.Env) {
		return true
	}
	return false
}

// Reconcile is the main control loop for Workload resources that triggers dispatching.
func (r *DispatcherReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	workload := new(v1.Workload)
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !workload.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}
	result, err := r.processWorkload(ctx, workload)
	if err != nil {
		klog.ErrorS(err, "failed to dispatch workload", "name", workload.Name)
		if jobutils.IsUnrecoverableError(err) {
			err = jobutils.SetWorkloadFailed(ctx, r.Client, workload, err.Error())
		}
	}
	return result, err
}

// processWorkload processes a workload resource and updates its state.
func (r *DispatcherReconciler) processWorkload(ctx context.Context, workload *v1.Workload) (ctrlruntime.Result, error) {
	clusterInformer, err := syncer.GetClusterInformer(r.clusterInformers, v1.GetClusterId(workload))
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	rt, err := jobutils.GetResourceTemplate(ctx, r.Client, workload.ToSchemaGVK())
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	resourceInformer, err := clusterInformer.GetResourceInformer(ctx, rt.ToSchemaGVK())
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	obj, err := jobutils.GetObject(resourceInformer, workload.Name, workload.Spec.Workspace)

	switch {
	case !v1.IsWorkloadDispatched(workload):
		if apierrors.IsNotFound(err) {
			if err = r.dispatch(ctx, workload, clusterInformer); err != nil {
				break
			}
		} else if err != nil {
			break
		}
		if err = r.createService(ctx, workload, clusterInformer); err != nil {
			break
		}
		if err = r.patchDispatched(ctx, workload); err != nil {
			break
		}
		klog.Infof("the workload is dispatched, name: %s, dispatch count: %d, max retry: %d",
			workload.Name, v1.GetWorkloadDispatchCnt(workload), workload.Spec.MaxRetry)
	case err == nil:
		// update the workload which is already dispatched
		err = r.updateK8sObject(ctx, workload, clusterInformer, obj)
	}
	return ctrlruntime.Result{}, err
}

// dispatch creates and deploys the Kubernetes object in the data plane for the workload.
func (r *DispatcherReconciler) dispatch(ctx context.Context,
	adminWorkload *v1.Workload, clusterInformer *syncer.ClusterInformer) error {
	// To prevent port conflicts when retrying, the port must be regenerated each time
	if err := r.generateUniquePorts(ctx, adminWorkload); err != nil {
		return err
	}
	k8sObject, err := r.generateK8sObject(ctx, adminWorkload)
	if err != nil {
		klog.ErrorS(err, "failed to create k8s unstructured object. ",
			"name", adminWorkload.Name, "gvk", adminWorkload.Spec.GroupVersionKind)
		return err
	}
	if err = jobutils.CreateObject(ctx, clusterInformer.ClientFactory(), k8sObject); err != nil {
		return err
	}
	return nil
}

// generateUniquePorts generates unique job and SSH ports for the workload to avoid conflicts.
func (r *DispatcherReconciler) generateUniquePorts(ctx context.Context, workload *v1.Workload) error {
	if workload.SpecKind() == common.CICDScaleSetKind {
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
	workload.Spec.JobPort = generateRandomPort(ports)
	workload.Spec.SSHPort = generateRandomPort(ports)
	if workload.Spec.JobPort == 0 || workload.Spec.SSHPort == 0 {
		return commonerrors.NewInternalError("failed to generate job or SSH port")
	}
	if err := r.Update(ctx, workload); err != nil {
		return err
	}
	return nil
}

// generateK8sObject creates the unstructured Kubernetes object from the workload specification.
func (r *DispatcherReconciler) generateK8sObject(ctx context.Context,
	adminWorkload *v1.Workload) (*unstructured.Unstructured, error) {
	workspace, err := r.getWorkspace(ctx, adminWorkload)
	if err != nil {
		return nil, err
	}

	rt, err := jobutils.GetResourceTemplate(ctx, r.Client, adminWorkload.ToSchemaGVK())
	if err != nil {
		klog.Error(err.Error())
		return nil, err
	}

	result, err := r.getWorkloadTemplate(ctx, adminWorkload)
	if err != nil {
		klog.Error(err.Error())
		return nil, err
	}
	if err = updateUnstructuredObject(result, adminWorkload, workspace, rt); err != nil {
		return nil, commonerrors.NewInternalError(err.Error())
	}
	for _, t := range rt.Spec.ResourceSpecs {
		if err = modifyObjectOnCreation(result, adminWorkload, workspace, &t); err != nil {
			return nil, commonerrors.NewInternalError(err.Error())
		}
	}
	setK8sObjectMeta(result, adminWorkload)
	return result, nil
}

func setK8sObjectMeta(result *unstructured.Unstructured, adminWorkload *v1.Workload) {
	result.SetName(adminWorkload.Name)
	result.SetNamespace(adminWorkload.Spec.Workspace)
	labels := result.GetLabels()
	if len(labels) == 0 {
		labels = make(map[string]string)
	}

	newLabels := buildLabels(adminWorkload)
	for key, val := range newLabels {
		if strValue, ok := val.(string); ok {
			labels[key] = strValue
		}
	}
	annotations := result.GetAnnotations()
	if len(annotations) == 0 {
		annotations = make(map[string]string)
	}
	if v1.GetUserName(adminWorkload) != "" {
		annotations[v1.UserNameAnnotation] = v1.GetUserName(adminWorkload)
	}
	if len(labels) > 0 {
		result.SetLabels(labels)
	}
	if len(annotations) > 0 {
		result.SetAnnotations(annotations)
	}
}

// getWorkloadTemplate retrieves the workload template configuration based on its version and kind.
func (r *DispatcherReconciler) getWorkloadTemplate(ctx context.Context, adminWorkload *v1.Workload) (*unstructured.Unstructured, error) {
	templateConfig, err := commonworkload.GetWorkloadTemplate(ctx, r.Client, adminWorkload)
	if err != nil {
		return nil, err
	}
	templateStr, ok := templateConfig.Data["template"]
	if !ok || templateStr == "" {
		return nil, commonerrors.NewInternalError(
			fmt.Sprintf("failed to find the template. name: %s", templateConfig.Name))
	}
	template, err := jsonutils.ParseYamlToJson(templateStr)
	if err != nil {
		return nil, commonerrors.NewInternalError(
			fmt.Sprintf("failed to parse template: %v", err.Error()))
	}
	return template, nil
}

// patchDispatched updates a workload's status to indicate it has been dispatched.
func (r *DispatcherReconciler) patchDispatched(ctx context.Context, workload *v1.Workload) error {
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload) + 1)
	cond := jobutils.NewCondition(string(v1.AdminDispatched), "the workload is dispatched", reason)
	if jobutils.FindCondition(workload, cond) == nil {
		workload.Status.Conditions = append(workload.Status.Conditions, *cond)
		if workload.Status.Phase == "" {
			workload.Status.Phase = v1.WorkloadPending
		}
		if err := r.Status().Update(ctx, workload); err != nil {
			klog.ErrorS(err, "failed to update workload", "name", workload.Name)
			return err
		}
	}

	if !v1.IsWorkloadDispatched(workload) {
		originalWorkload := client.MergeFrom(workload.DeepCopy())
		v1.SetAnnotation(workload, v1.WorkloadDispatchedAnnotation, timeutil.FormatRFC3339(time.Now().UTC()))
		v1.SetLabel(workload, v1.WorkloadDispatchCntLabel, buildDispatchCount(workload))
		v1.RemoveAnnotation(workload, v1.WorkloadPreemptedAnnotation)
		if err := r.Patch(ctx, workload, originalWorkload); err != nil {
			klog.ErrorS(err, "failed to patch workload", "name", workload.Name)
			return err
		}
	}
	return nil
}

// updateK8sObject updates the existing Kubernetes object when workload specs change.
func (r *DispatcherReconciler) updateK8sObject(ctx context.Context, adminWorkload *v1.Workload,
	clusterInformer *syncer.ClusterInformer, obj *unstructured.Unstructured) error {
	rt, err := jobutils.GetResourceTemplate(ctx, r.Client, adminWorkload.ToSchemaGVK())
	if err != nil {
		klog.ErrorS(err, "", "gvk", adminWorkload.Spec.GroupVersionKind)
		return err
	}
	if len(rt.Spec.ResourceSpecs) == 0 {
		return nil
	}

	functions := []func(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool{
		isResourceChanged, isImageChanged, isEntryPointChanged, isSharedMemoryChanged, isEnvChanged, isPriorityClassChanged,
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
	if err = updateUnstructuredObject(obj, adminWorkload, workspace, rt); err != nil {
		return commonerrors.NewBadRequest(err.Error())
	}

	if err = jobutils.UpdateObject(ctx, clusterInformer.ClientFactory(), obj); err != nil {
		klog.ErrorS(err, "failed to update k8s unstructured object")
		return err
	}
	return nil
}

// isResourceChanged checks if the resource requirements of the workload have changed.
func isResourceChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	replicaList, resourceList, err := jobutils.GetResources(obj, rt,
		v1.GetMainContainer(adminWorkload), adminWorkload.Spec.Resource.GPUName)
	if err != nil || len(resourceList) == 0 {
		klog.ErrorS(err, "failed to get resource", "rt", rt.Name, "obj", obj.GetName())
		return false
	}
	var totalReplica int64 = 0
	for _, n := range replicaList {
		totalReplica += n
	}
	if int(totalReplica) != adminWorkload.Spec.Resource.Replica {
		return true
	}

	podResource, err := commonworkload.GetPodResources(&adminWorkload.Spec.Resource)
	if err != nil {
		return false
	}
	if !quantity.Equal(podResource, resourceList[0]) {
		return true
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
	if !commonworkload.IsJob(adminWorkload) {
		return false
	}
	memoryStorageSize, err := jobutils.GetMemoryStorageSize(obj, rt)
	if err != nil {
		if adminWorkload.Spec.Resource.SharedMemory == "" {
			return false
		}
		return true
	}
	return memoryStorageSize != adminWorkload.Spec.Resource.SharedMemory
}

// isPriorityClassChanged checks if the priority of the workload has changed.
func isPriorityClassChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	priorityClassName, err := jobutils.GetPriorityClassName(obj, rt)
	if err != nil {
		return true
	}
	return commonworkload.GeneratePriorityClass(adminWorkload) != priorityClassName
}

// updateUnstructuredObject updates the unstructured object with workload specifications.
func updateUnstructuredObject(obj *unstructured.Unstructured,
	adminWorkload *v1.Workload, workspace *v1.Workspace, rt *v1.ResourceTemplate) error {
	if adminWorkload.SpecKind() == common.CICDScaleSetKind {
		return updateCICDScaleSet(obj, adminWorkload, workspace, rt)
	}

	var preAllocatedReplica int64 = 0
	for _, t := range rt.Spec.ResourceSpecs {
		preAllocatedReplica += t.Replica
	}
	for _, t := range rt.Spec.ResourceSpecs {
		replica := t.Replica
		// A webhook validation was previously to ensure that only one template could have replica=0
		if replica == 0 {
			replica = int64(adminWorkload.Spec.Resource.Replica) - preAllocatedReplica
		}
		if replica <= 0 {
			unstructured.RemoveNestedField(obj.Object, t.PrePaths...)
			continue
		}
		if err := updateHostNetwork(adminWorkload, obj, t); err != nil {
			return fmt.Errorf("failed to update host network: %v", err.Error())
		}
		if err := updateReplica(adminWorkload, obj, t, replica); err != nil {
			return fmt.Errorf("failed to update replica: %v", err.Error())
		}
		if err := updateMainContainer(adminWorkload, obj, t); err != nil {
			return fmt.Errorf("failed to update main container: %v", err.Error())
		}
		if err := updateSharedMemory(adminWorkload, obj, t); err != nil {
			return fmt.Errorf("failed to update shared memory: %v", err.Error())
		}
		if err := updatePriorityClass(adminWorkload, obj, t); err != nil {
			return fmt.Errorf("failed to update priority: %v", err.Error())
		}
	}
	return nil
}

// updateReplica updates the replica count in the unstructured object.
func updateReplica(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec, replica int64) error {
	path := resourceSpec.PrePaths
	path = append(path, resourceSpec.ReplicasPaths...)
	if err := unstructured.SetNestedField(obj.Object, replica, path...); err != nil {
		return err
	}
	if adminWorkload.SpecKind() == common.JobKind {
		end := len(path) - 1
		if end < 0 {
			end = 0
		}
		path = path[:end]
		path = append(path, "completions")
		if err := unstructured.SetNestedField(obj.Object, replica, path...); err != nil {
			return err
		}
	}
	return nil
}

// updateCICDScaleSet updates the CICD scale set configuration in the unstructured object.
// It first updates the GitHub configuration, then conditionally updates the environments for build
// or removes unnecessary containers based on whether CICD unified build is enabled.
// Returns an error if no resource templates are found or if any update operation fails.
func updateCICDScaleSet(obj *unstructured.Unstructured,
	adminWorkload *v1.Workload, workspace *v1.Workspace, rt *v1.ResourceTemplate) error {
	if len(rt.Spec.ResourceSpecs) == 0 {
		return fmt.Errorf("no resource template found")
	}
	if err := updateCICDGithub(adminWorkload, obj, rt); err != nil {
		return err
	}
	if err := updateCICDEnvironments(obj, adminWorkload, workspace, rt.Spec.ResourceSpecs[0]); err != nil {
		return err
	}
	return nil
}

// updateCICDGithub updates the CICD scale set configuration in the unstructured object.
// It updates the GitHub configuration and then configures environment variables based on unified build settings.
// Returns an error if no resource templates are found or if any update operation fails.
func updateCICDGithub(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, rt *v1.ResourceTemplate) error {
	if len(rt.Spec.ResourceSpecs) == 0 {
		return fmt.Errorf("no resource template found")
	}

	specObject, ok, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("failed to find object with path: [spec]")
	}
	githubConfig := buildGithubConfig(adminWorkload)
	for key, val := range githubConfig {
		specObject[key] = val
	}
	if err = unstructured.SetNestedMap(obj.Object, specObject, "spec"); err != nil {
		return err
	}
	return nil
}

// updateCICDEnvironments configures environment variables for CICD workloads based on unified build settings.
// When unified build is enabled, it updates all containers with NFS paths and environment variables,
// with additional resource variables for the main container.
// When unified build is disabled, it keeps only the main container with resource variables.
func updateCICDEnvironments(obj *unstructured.Unstructured,
	adminWorkload *v1.Workload, workspace *v1.Workspace, resourceSpec v1.ResourceSpec) error {
	containers, path, err := getContainers(obj, resourceSpec)
	if err != nil {
		return err
	}
	envs := maps.Copy(adminWorkload.Spec.Env)
	mainContainerName := v1.GetMainContainer(adminWorkload)

	if v1.IsCICDUnifiedBuildEnable(adminWorkload) {
		pfsPath := ""
		for _, vol := range workspace.Spec.Volumes {
			if vol.Type == v1.PFS {
				pfsPath = vol.MountPath
				break
			}
		}
		envs[jobutils.NfsPathEnv] = pfsPath
		envs[jobutils.NfsInputEnv] = UnifiedBuildInput
		envs[jobutils.NfsOutputEnv] = UnifiedBuildOutput
		envs[jobutils.WorkloadEnv] = adminWorkload.Name
		envs[jobutils.WorkspaceEnv] = adminWorkload.Spec.Workspace
		envs[jobutils.UserEnv] = v1.GetUserId(adminWorkload)

		// When unified build is enabled, update all containers with envs
		// and add resource variables to main container
		for i := range containers {
			container := containers[i].(map[string]interface{})
			name := jobutils.GetUnstructuredString(container, []string{"name"})
			if name == mainContainerName {
				// For main container, also add resource variables
				newEnvs := maps.Copy(envs)
				newEnvs[jobutils.ResourcesEnv] = string(jsonutils.MarshalSilently(adminWorkload.Spec.Resource))
				newEnvs[jobutils.ImageEnv] = adminWorkload.Spec.Image
				newEnvs[jobutils.EntrypointEnv] = buildEntryPoint(adminWorkload)
				updateContainerEnv(newEnvs, container)
			} else {
				updateContainerEnv(envs, container)
			}
		}
		if err = unstructured.SetNestedField(obj.Object, containers, path...); err != nil {
			return err
		}
	} else {
		// When unified build is disabled, keep only main container with resource variables
		for i := range containers {
			container := containers[i].(map[string]interface{})
			name := jobutils.GetUnstructuredString(container, []string{"name"})
			if name == mainContainerName {
				// Only update main container with resource variables
				envs[jobutils.ResourcesEnv] = string(jsonutils.MarshalSilently(adminWorkload.Spec.Resource))
				envs[jobutils.ImageEnv] = adminWorkload.Spec.Image
				envs[jobutils.EntrypointEnv] = buildEntryPoint(adminWorkload)
				updateContainerEnv(envs, container)
				// Keep only the main container
				newContainers := []interface{}{container}
				return unstructured.SetNestedField(obj.Object, newContainers, path...)
			}
		}
		return fmt.Errorf("no main container found")
	}
	return nil
}

// updateMainContainer updates the main container configuration in the unstructured object.
func updateMainContainer(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec) error {
	containers, path, err := getContainers(obj, resourceSpec)
	if err != nil {
		return err
	}

	mainContainer, err := getMainContainer(containers, v1.GetMainContainer(adminWorkload))
	if err != nil {
		return err
	}
	resources := buildResources(adminWorkload)
	mainContainer["resources"] = map[string]interface{}{
		"limits":   resources,
		"requests": resources,
	}
	mainContainer["image"] = adminWorkload.Spec.Image
	mainContainer["command"] = buildCommands(adminWorkload)
	if len(adminWorkload.Spec.Env) > 0 {
		updateContainerEnv(adminWorkload.Spec.Env, mainContainer)
	}
	if err = unstructured.SetNestedField(obj.Object, containers, path...); err != nil {
		return err
	}
	return nil
}

// getContainers retrieves the containers slice and its path from the unstructured object based on the resource specification.
// Returns the containers slice, the path to the containers field, and an error if the operation fails or no containers are found.
func getContainers(obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec) ([]interface{}, []string, error) {
	templatePath := resourceSpec.GetTemplatePath()
	path := append(templatePath, "spec", "containers")
	containers, found, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return nil, nil, err
	}
	if !found || len(containers) == 0 {
		return nil, nil, fmt.Errorf("failed to find container with path: %v", path)
	}
	return containers, path, nil
}

// updateContainerEnv updates environment variables in the container.
func updateContainerEnv(envs map[string]string, mainContainer map[string]interface{}) {
	var currentEnv []interface{}
	envObjs, ok := mainContainer["env"]
	if ok {
		currentEnv = envObjs.([]interface{})
	}

	newEnv := make([]interface{}, 0, len(currentEnv))
	currentEnvSet := sets.NewSet()
	isChanged := false

	for i, e := range currentEnv {
		env, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := env["name"]
		if !ok {
			continue
		}
		nameStr := name.(string)
		currentEnvSet.Insert(nameStr)
		value, ok := env["value"]
		if !ok {
			newEnv = append(newEnv, currentEnv[i])
			continue
		}
		specValue, ok := envs[nameStr]
		if ok && specValue != value.(string) {
			isChanged = true
			// An empty value means the field should be deleted.
			if specValue == "" {
				continue
			}
			currentEnv[i] = map[string]interface{}{
				"name":  nameStr,
				"value": specValue,
			}
		}
		newEnv = append(newEnv, currentEnv[i])
	}

	for key, val := range envs {
		if val == "" {
			continue
		}
		if !currentEnvSet.Has(key) {
			isChanged = true
			newEnv = append(newEnv, map[string]interface{}{
				"name":  key,
				"value": val,
			})
		}
	}
	if !isChanged {
		return
	}
	mainContainer["env"] = newEnv
}

// updateSharedMemory updates the shared memory volume configuration.
func updateSharedMemory(adminWorkload *v1.Workload, obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec) error {
	if !commonworkload.IsJob(adminWorkload) {
		return nil
	}
	path := resourceSpec.PrePaths
	path = append(path, resourceSpec.TemplatePaths...)
	path = append(path, "spec", "volumes")
	volumes, found, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	if !found {
		sharedMemoryVolume := buildSharedMemoryVolume(adminWorkload.Spec.Resource.SharedMemory)
		volumes = []interface{}{sharedMemoryVolume}
		if err = unstructured.SetNestedSlice(obj.Object, volumes, path...); err != nil {
			return err
		}
		return nil
	}

	sharedMemory := jobutils.GetMemoryStorageVolume(volumes)
	if sharedMemory != nil {
		sharedMemory["sizeLimit"] = adminWorkload.Spec.Resource.SharedMemory
		if err = unstructured.SetNestedField(obj.Object, volumes, path...); err != nil {
			return err
		}
	} else {
		volumes = append(volumes, buildSharedMemoryVolume(adminWorkload.Spec.Resource.SharedMemory))
		if err = unstructured.SetNestedSlice(obj.Object, volumes, path...); err != nil {
			return err
		}
	}
	return nil
}

// updateHostNetwork updates the host network configuration.
func updateHostNetwork(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec) error {
	templatePath := resourceSpec.GetTemplatePath()
	path := append(templatePath, "spec", "hostNetwork")
	return modifyHostNetwork(obj, adminWorkload, path)
}

// updatePriorityClass updates the priority class configuration.
func updatePriorityClass(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, resourceSpec v1.ResourceSpec) error {
	templatePath := resourceSpec.GetTemplatePath()
	path := append(templatePath, "spec", "priorityClassName")
	return modifyPriorityClass(obj, adminWorkload, path)
}

// createService creates a Kubernetes Service for the workload if specified.
func (r *DispatcherReconciler) createService(ctx context.Context,
	adminWorkload *v1.Workload, clusterInformer *syncer.ClusterInformer) error {
	if adminWorkload.Spec.Service == nil {
		return nil
	}
	k8sClientSet := clusterInformer.ClientFactory().ClientSet()
	namespace := adminWorkload.Spec.Workspace
	var err error
	if _, err = k8sClientSet.CoreV1().Services(namespace).Get(ctx, adminWorkload.Name, metav1.GetOptions{}); err == nil {
		return nil
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
			Ports: []corev1.ServicePort{{
				Protocol:   specService.Protocol,
				Port:       int32(specService.Port),
				TargetPort: intstr.IntOrString{IntVal: int32(specService.TargetPort)},
			}},
			Type: specService.ServiceType,
		},
	}
	if err = controllerutil.SetControllerReference(adminWorkload, service, r.Client.Scheme()); err != nil {
		klog.ErrorS(err, "failed to SetControllerReference")
		return err
	}
	if specService.ServiceType == corev1.ServiceTypeNodePort && specService.NodePort > 0 {
		service.Spec.Ports[0].NodePort = int32(specService.NodePort)
	}

	if service, err = k8sClientSet.CoreV1().Services(namespace).Create(ctx,
		service, metav1.CreateOptions{}); client.IgnoreAlreadyExists(err) != nil {
		klog.ErrorS(err, "failed to create service", "name", adminWorkload.Name)
		if specService.NodePort > 0 {
			// NodePort error occurred; skipping retry.
			return commonerrors.NewBadRequest(err.Error())
		}
		return err
	}
	return nil
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

// buildDispatchCount generates the dispatch count as a string.
func buildDispatchCount(w *v1.Workload) string {
	// The count for the first dispatch is 1, so it needs to be incremented by 1 here.
	return strconv.Itoa(v1.GetWorkloadDispatchCnt(w) + 1)
}

// isDispatchingJob checks if a workload is ready to be dispatched.
func isDispatchingJob(w *v1.Workload) bool {
	if v1.IsWorkloadScheduled(w) && !v1.IsWorkloadDispatched(w) {
		return true
	}
	return false
}
