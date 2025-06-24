/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

type DispatcherReconciler struct {
	client.Client
	clusterInformers *commonutils.ObjectManager
}

func SetupDispatcherController(mgr manager.Manager) error {
	r := &DispatcherReconciler{
		Client:           mgr.GetClient(),
		clusterInformers: commonutils.NewObjectManagerSingleton(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workload{}, builder.WithPredicates(caredChangePredicate{})).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Dispatcher Controller successfully")
	return nil
}

type caredChangePredicate struct {
	predicate.Funcs
}

func (caredChangePredicate) Create(e event.CreateEvent) bool {
	w, ok := e.Object.(*v1.Workload)
	if !ok {
		return false
	}
	if isDispatchingJob(w) {
		return true
	}
	return false
}

func (caredChangePredicate) Update(e event.UpdateEvent) bool {
	oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
	newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
	if !ok1 || !ok2 {
		return false
	}
	if !isDispatchingJob(oldWorkload) && isDispatchingJob(newWorkload) {
		return true
	}
	if !commonworkload.IsResourceEqual(oldWorkload, newWorkload) ||
		oldWorkload.Spec.Resource.ShareMemory != newWorkload.Spec.Resource.ShareMemory {
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

func (r *DispatcherReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	workload := new(v1.Workload)
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !workload.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}
	result, err := r.handle(ctx, workload)
	if err != nil {
		klog.ErrorS(err, "failed to dispatch workload", "name", workload.Name)
		if jobutils.IsNonRetryableError(err) {
			err = jobutils.SetWorkloadFailed(ctx, r.Client, workload, err.Error())
		}
	}
	return result, err
}

func (r *DispatcherReconciler) handle(ctx context.Context, workload *v1.Workload) (ctrlruntime.Result, error) {
	clusterInformer, err := syncer.GetClusterInformer(r.clusterInformers, v1.GetClusterId(workload))
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	resourceInformer, err := clusterInformer.GetResourceInformer(ctx, workload.ToSchemaGVK())
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	obj, err := jobutils.GetObject(resourceInformer, workload.Name, workload.Spec.Workspace)
	switch {
	case !v1.IsWorkloadDispatched(workload):
		if apierrors.IsNotFound(err) {
			err = r.dispatch(ctx, workload, clusterInformer)
		}
		if err != nil {
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

func (r *DispatcherReconciler) dispatch(ctx context.Context,
	adminWorkload *v1.Workload, clusterInformer *syncer.ClusterInformer) error {
	// To prevent port conflicts when retrying, the port must be regenerated each time
	if err := r.buildPort(ctx, adminWorkload); err != nil {
		return err
	}

	k8sObject, err := r.createK8sObject(ctx, adminWorkload)
	if err != nil {
		klog.ErrorS(err, "failed to create k8s unstructured object. ",
			"name", adminWorkload.Name, "gvk", adminWorkload.Spec.GroupVersionKind)
		return err
	}
	if err = jobutils.CreateObject(ctx, clusterInformer.ClientFactory().DynamicClient(),
		clusterInformer.ClientFactory().Mapper(), k8sObject); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			klog.ErrorS(err, "failed to create k8s unstructured object")
		} else {
			err = nil
		}
		return err
	}
	return nil
}

func (r *DispatcherReconciler) buildPort(ctx context.Context, workload *v1.Workload) error {
	rand.Seed(time.Now().UnixNano())
	ports := make(map[int]bool)

	workloadList := &v1.WorkloadList{}
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: v1.GetClusterId(workload)})
	// Record currently in-use ports to avoid reuse
	if r.List(ctx, workloadList, &client.ListOptions{LabelSelector: labelSelector}) == nil {
		for _, item := range workloadList.Items {
			if !v1.IsEnableHostNetwork(&item) {
				continue
			}
			ports[item.Spec.Resource.JobPort] = true
		}
	}
	workload.Spec.Resource.JobPort = buildRandPort(ports)
	if err := r.Update(ctx, workload); err != nil {
		return err
	}
	return nil
}

func (r *DispatcherReconciler) createK8sObject(ctx context.Context,
	adminWorkload *v1.Workload) (*unstructured.Unstructured, error) {
	workspace := &v1.Workspace{}
	err := r.Get(ctx, client.ObjectKey{Name: adminWorkload.Spec.Workspace}, workspace)
	if err != nil {
		return nil, err
	}
	rt, err := jobutils.GetResourceTemplate(ctx, r.Client, adminWorkload.ToSchemaGVK())
	if err != nil {
		klog.ErrorS(err, "", "gvk", adminWorkload.Spec.GroupVersionKind)
		return nil, err
	}

	result, err := r.getWorkloadTemplate(ctx, adminWorkload)
	if err != nil {
		return nil, err
	}
	if err = updateUnstructuredObj(result, adminWorkload, rt); err != nil {
		return nil, commonerrors.NewInternalError(err.Error())
	}
	for _, t := range rt.Spec.Templates {
		if err = modifyObjectOnCreation(result, adminWorkload, workspace, &t); err != nil {
			return nil, commonerrors.NewInternalError(err.Error())
		}
	}
	result.SetName(adminWorkload.Name)
	result.SetNamespace(adminWorkload.Spec.Workspace)
	result.SetLabels(convertToStringMap(buildLabels(adminWorkload)))
	if v1.GetUserName(adminWorkload) != "" {
		result.SetAnnotations(map[string]string{
			v1.UserNameAnnotation: v1.GetUserName(adminWorkload),
		})
	}
	return result, nil
}

func (r *DispatcherReconciler) getWorkloadTemplate(ctx context.Context, adminWorkload *v1.Workload) (*unstructured.Unstructured, error) {
	templateConfig, err := commonworkload.GetWorkloadTemplate(ctx, r.Client,
		adminWorkload.Spec.GroupVersionKind, adminWorkload.Spec.Resource.GPUName)
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
		return nil, commonerrors.NewInternalError(err.Error())
	}
	return template, nil
}

func (r *DispatcherReconciler) patchDispatched(ctx context.Context, workload *v1.Workload) error {
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload) + 1)
	cond := jobutils.NewCondition(string(v1.AdminDispatched), "the workload is dispatched", reason)
	if jobutils.FindCondition(workload, cond) == nil {
		workload.Status.Conditions = append(workload.Status.Conditions, *cond)
		if err := r.Status().Update(ctx, workload); err != nil {
			klog.ErrorS(err, "failed to update workload", "name", workload.Name)
			return err
		}
	}

	if !v1.IsWorkloadDispatched(workload) {
		patch := client.MergeFrom(workload.DeepCopy())
		v1.SetAnnotation(workload, v1.WorkloadDispatchedAnnotation, time.Now().UTC().Format(time.RFC3339))
		v1.SetLabel(workload, v1.WorkloadDispatchCntLabel, buildDispatchCount(workload))
		v1.RemoveAnnotation(workload, v1.WorkloadPreemptedAnnotation)
		if err := r.Patch(ctx, workload, patch); err != nil {
			klog.ErrorS(err, "failed to patch workload", "name", workload.Name)
			return err
		}
	}
	return nil
}

func (r *DispatcherReconciler) updateK8sObject(ctx context.Context, adminWorkload *v1.Workload,
	clusterInformer *syncer.ClusterInformer, obj *unstructured.Unstructured) error {
	rt, err := jobutils.GetResourceTemplate(ctx, r.Client, adminWorkload.ToSchemaGVK())
	if err != nil {
		klog.ErrorS(err, "", "gvk", adminWorkload.Spec.GroupVersionKind)
		return err
	}
	if len(rt.Spec.Templates) == 0 {
		return nil
	}

	functions := []func(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool{
		isResourceChanged, isImageChanged, isEntryPointChanged, isShareMemoryChanged, isEnvChanged, isPriorityClassChanged,
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

	if err = updateUnstructuredObj(obj, adminWorkload, rt); err != nil {
		return commonerrors.NewBadRequest(err.Error())
	}

	if err = jobutils.UpdateObject(ctx, clusterInformer.ClientFactory().DynamicClient(),
		clusterInformer.ClientFactory().Mapper(), obj); err != nil {
		klog.ErrorS(err, "failed to update k8s unstructured object")
		return err
	}
	return nil
}

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

	podResource, err := commonworkload.GetPodResources(adminWorkload)
	if err != nil {
		return false
	}
	if !quantity.Equal(podResource, resourceList[0]) {
		return true
	}
	return false
}

func isImageChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	image, err := jobutils.GetImage(obj, rt, v1.GetMainContainer(adminWorkload))
	if err != nil {
		klog.ErrorS(err, "failed to get image", "obj", obj.GetName())
		return false
	}
	return adminWorkload.Spec.Image != image
}

func isEntryPointChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	commands, err := jobutils.GetCommand(obj, rt, v1.GetMainContainer(adminWorkload))
	if err != nil {
		klog.ErrorS(err, "failed to get command", "obj", obj.GetName())
		return false
	}
	if len(commands) == 0 {
		return false
	}
	cmd := buildEntryPoint(adminWorkload.Spec.EntryPoint)
	return cmd != commands[len(commands)-1]
}

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

func isShareMemoryChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	shareMemory, err := jobutils.GetShareMemorySize(obj, rt)
	if err != nil {
		if adminWorkload.Spec.Resource.ShareMemory == "" {
			return false
		}
		return true
	}
	return shareMemory != adminWorkload.Spec.Resource.ShareMemory
}

func isPriorityClassChanged(adminWorkload *v1.Workload, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) bool {
	priorityClassName, err := jobutils.GetPriorityClassName(obj, rt)
	if err != nil {
		return true
	}
	return commonworkload.GeneratePriorityClass(adminWorkload) != priorityClassName
}

func updateUnstructuredObj(obj *unstructured.Unstructured, adminWorkload *v1.Workload, rt *v1.ResourceTemplate) error {
	var preAllocatedReplica int64 = 0
	for _, t := range rt.Spec.Templates {
		preAllocatedReplica += t.Replica
	}

	for _, t := range rt.Spec.Templates {
		replica := t.Replica
		// A webhook validation was previously to ensure that only one template could have replica=0
		if replica == 0 {
			replica = int64(adminWorkload.Spec.Resource.Replica) - preAllocatedReplica
		}
		if replica <= 0 {
			unstructured.RemoveNestedField(obj.Object, t.PrePaths...)
			continue
		}
		if err := udpateHostNetwork(adminWorkload, obj, t); err != nil {
			return err
		}
		if err := updateReplica(obj, t, replica); err != nil {
			return err
		}
		if err := updateMainContainer(adminWorkload, obj, t); err != nil {
			return err
		}
		if err := updateShareMemory(adminWorkload, obj, t); err != nil {
			return err
		}
		if err := udpatePriorityClass(adminWorkload, obj, t); err != nil {
			return err
		}
	}
	return nil
}

func updateReplica(obj *unstructured.Unstructured, template v1.Template, replica int64) error {
	path := template.PrePaths
	path = append(path, template.ReplicasPaths...)
	if err := unstructured.SetNestedField(obj.Object, replica, path...); err != nil {
		return err
	}
	return nil
}

func updateMainContainer(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, template v1.Template) error {
	templatePath := template.GetTemplatePath()
	path := append(templatePath, "spec", "containers")
	containers, found, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	if !found || len(containers) == 0 {
		return fmt.Errorf("failed to find container with path: %v", path)
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
	mainContainer["command"] = buildCommands(adminWorkload.Spec.EntryPoint)
	if len(adminWorkload.Spec.Env) > 0 {
		updateContainerEnv(adminWorkload, mainContainer)
	}
	if err = unstructured.SetNestedField(obj.Object, containers, path...); err != nil {
		return err
	}
	return nil
}

func updateContainerEnv(adminWorkload *v1.Workload, mainContainer map[string]interface{}) {
	currentEnv := mainContainer["env"].([]interface{})
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
		specValue, ok := adminWorkload.Spec.Env[nameStr]
		if ok && specValue != value.(string) {
			isChanged = true
			// A empty value means the field should be deleted.
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

	for key, val := range adminWorkload.Spec.Env {
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

func updateShareMemory(adminWorkload *v1.Workload, obj *unstructured.Unstructured, template v1.Template) error {
	path := template.PrePaths
	path = append(path, template.TemplatePaths...)
	path = append(path, "spec", "volumes")
	volumes, found, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil {
		return err
	}
	if !found {
		shareMemoryVolume := buildShareMemory(adminWorkload.Spec.Resource.ShareMemory)
		volumes = []interface{}{shareMemoryVolume}
		if err = unstructured.SetNestedSlice(obj.Object, volumes, path...); err != nil {
			return err
		}
		return nil
	}

	shareMemory := jobutils.GetShareMemoryVolume(volumes)
	if shareMemory != nil {
		shareMemory["sizeLimit"] = adminWorkload.Spec.Resource.ShareMemory
		if err = unstructured.SetNestedField(obj.Object, volumes, path...); err != nil {
			return err
		}
	} else {
		volumes = append(volumes, buildShareMemory(adminWorkload.Spec.Resource.ShareMemory))
		if err = unstructured.SetNestedSlice(obj.Object, volumes, path...); err != nil {
			return err
		}
	}
	return nil
}

func udpateHostNetwork(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, template v1.Template) error {
	templatePath := template.GetTemplatePath()
	path := append(templatePath, "spec", "hostNetwork")
	return modifyHostNetWork(obj, adminWorkload, path)
}

func udpatePriorityClass(adminWorkload *v1.Workload,
	obj *unstructured.Unstructured, template v1.Template) error {
	templatePath := template.GetTemplatePath()
	path := append(templatePath, "spec", "priorityClassName")
	return modifyPriorityClass(obj, adminWorkload, path)
}

func (r *DispatcherReconciler) createService(ctx context.Context,
	adminWorkload *v1.Workload, clusterInformer *syncer.ClusterInformer) error {
	if adminWorkload.Spec.Service == nil {
		return nil
	}
	clientSet := clusterInformer.ClientFactory().ClientSet().CoreV1()
	namespace := adminWorkload.Spec.Workspace
	var err error
	if _, err = clientSet.Services(namespace).Get(ctx, adminWorkload.Name, metav1.GetOptions{}); err == nil {
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

	if service, err = clientSet.Services(namespace).Create(ctx,
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

func buildRandPort(ports map[int]bool) int {
	for {
		port := rand.Intn(10000) + 20000
		_, ok := ports[port]
		if !ok {
			ports[port] = true
			return port
		}
	}
}

func buildDispatchCount(w *v1.Workload) string {
	return strconv.Itoa(v1.GetWorkloadDispatchCnt(w) + 1)
}

func isDispatchingJob(w *v1.Workload) bool {
	if v1.IsWorkloadScheduled(w) && !v1.IsWorkloadDispatched(w) {
		return true
	}
	return false
}
