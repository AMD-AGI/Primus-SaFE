/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const cicdProxyOwnerIndex = "cicdProxyOwnerUID"

func desiredCICDProxy(source *v1.Workload) (map[string]interface{}, error) {
	config, err := commonworkload.ParseCICDProxy(source.Spec.Env)
	if err != nil || config == nil {
		return nil, err
	}
	proxy := make(map[string]interface{})
	for _, scheme := range []string{"http", "https"} {
		endpoint := map[string]interface{}{"url": config.URL}
		if config.CredentialSecret != "" {
			endpoint["credentialSecretRef"] = config.CredentialSecret
		}
		proxy[scheme] = endpoint
	}
	if len(config.NoProxy) > 0 {
		entries := make([]interface{}, len(config.NoProxy))
		for i, entry := range config.NoProxy {
			entries[i] = entry
		}
		proxy["noProxy"] = entries
	}
	return proxy, nil
}

func isCICDProxyChanged(source *v1.Workload, obj *unstructured.Unstructured) (bool, error) {
	if !commonworkload.IsCICDProxyManaged(source) {
		return false, nil
	}
	desired, err := desiredCICDProxy(source)
	if err != nil {
		return false, err
	}
	owned := v1.GetAnnotation(obj, v1.CICDProxyManagedAnnotation) == v1.TrueStr
	if desired == nil && !owned {
		return false, nil
	}
	current, found, err := unstructured.NestedMap(obj.Object, "spec", "proxy")
	if err != nil {
		return false, fmt.Errorf("spec.proxy: unable to read ARC proxy configuration")
	}
	return (desired != nil) != owned || (desired == nil && found) || !reflect.DeepEqual(current, desired), nil
}

func updateCICDProxy(obj *unstructured.Unstructured, source *v1.Workload) error {
	if !commonworkload.IsCICDProxyManaged(source) {
		return nil
	}
	desired, err := desiredCICDProxy(source)
	if err != nil {
		return err
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	if desired == nil {
		if v1.GetAnnotation(obj, v1.CICDProxyManagedAnnotation) == v1.TrueStr {
			unstructured.RemoveNestedField(obj.Object, "spec", "proxy")
			delete(annotations, v1.CICDProxyManagedAnnotation)
			obj.SetAnnotations(annotations)
		}
		return nil
	}
	if err = unstructured.SetNestedMap(obj.Object, desired, "spec", "proxy"); err != nil {
		return err
	}
	annotations[v1.CICDProxyManagedAnnotation] = v1.TrueStr
	obj.SetAnnotations(annotations)
	return nil
}

func cicdProxyWorkload(workload, source *v1.Workload, obj *unstructured.Unstructured) *v1.Workload {
	if !commonworkload.IsCICDProxyManaged(source) {
		return workload
	}
	derived := workload.DeepCopy()
	if derived.Spec.Env == nil {
		derived.Spec.Env = make(map[string]string)
	}
	removed := v1.GetEnvToBeRemoved(workload)
	for _, key := range commonworkload.CICDProxyEnvKeys() {
		value, exists := source.Spec.Env[key]
		if exists {
			derived.Spec.Env[key] = value
		} else {
			delete(derived.Spec.Env, key)
			if cicdProxyContainerEnvPresent(obj, key) {
				removed = append(removed, key)
			}
		}
	}
	v1.SetAnnotation(derived, v1.EnvToBeRemovedAnnotation, string(jsonutils.MarshalSilently(removed)))
	return derived
}

func cicdProxyContainerEnvPresent(obj *unstructured.Unstructured, key string) bool {
	path := []string{"spec", "template", "spec", "containers"}
	if obj.GetKind() == common.CICDEphemeralRunnerKind {
		path = []string{"spec", "spec", "containers"}
	}
	containers, _, _ := unstructured.NestedSlice(obj.Object, path...)
	for _, entry := range containers {
		container, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		env, _, _ := unstructured.NestedSlice(container, "env")
		for _, entry := range env {
			if variable, ok := entry.(map[string]interface{}); ok && variable["name"] == key {
				return true
			}
		}
	}
	return false
}

func updateCICDProxyContainerEnvs(obj *unstructured.Unstructured, workload, source *v1.Workload, rt *v1.ResourceTemplate) error {
	if !commonworkload.IsCICDProxyManaged(source) {
		return nil
	}
	env, err := commonworkload.CICDProxyEnv(source.Spec.Env)
	if err != nil {
		return err
	}
	var removed []string
	for _, key := range commonworkload.CICDProxyEnvKeys() {
		if _, present := env[key]; !present {
			removed = append(removed, key)
		}
	}
	for _, spec := range rt.Spec.ResourceSpecs {
		containers, path, err := getContainers(workload, obj, spec)
		if err != nil {
			return err
		}
		for _, entry := range containers {
			container, ok := entry.(map[string]interface{})
			if !ok {
				return fmt.Errorf("spec.template: expected a container object")
			}
			updateContainerEnv(env, container, removed)
		}
		if err = jobutils.SetNestedField(obj.Object, containers, path); err != nil {
			return err
		}
	}
	return nil
}

func (r *DispatcherReconciler) syncCICDEphemeralRunnerProxy(ctx context.Context, workload, source *v1.Workload,
	clientSets *syncer.ClusterClientSets, obj *unstructured.Unstructured, rt *v1.ResourceTemplate) error {
	if !commonworkload.IsCICDProxyManaged(source) {
		return nil
	}
	changed, err := isCICDProxyChanged(source, obj)
	if err != nil {
		return err
	}
	desired := obj.DeepCopy()
	derived := cicdProxyWorkload(workload, source, obj)
	if err = updateCICDProxy(desired, source); err != nil {
		return err
	}
	if err = updateCICDProxyContainerEnvs(desired, derived, source, rt); err != nil {
		return err
	}
	if !changed && reflect.DeepEqual(obj.Object, desired.Object) {
		return nil
	}
	if err = jobutils.UpdateObject(ctx, clientSets.ClientFactory(), desired); err != nil {
		return err
	}
	return r.clearCICDEnvRemoval(ctx, workload)
}

func (r *DispatcherReconciler) clearCICDEnvRemoval(ctx context.Context, workload *v1.Workload) error {
	if !v1.HasAnnotation(workload, v1.EnvToBeRemovedAnnotation) {
		return nil
	}
	patch := client.MergeFromWithOptions(workload.DeepCopy(), client.MergeFromWithOptimisticLock{})
	v1.RemoveAnnotation(workload, v1.EnvToBeRemovedAnnotation)
	return r.Patch(ctx, workload, patch)
}

func cicdProxyOwnerUID(obj client.Object) []string {
	workload, ok := obj.(*v1.Workload)
	if !ok || !commonworkload.IsCICDEphemeralRunner(workload) {
		return nil
	}
	if owner := metav1.GetControllerOf(workload); owner != nil && owner.Kind == v1.WorkloadKind {
		return []string{string(owner.UID)}
	}
	return nil
}

func (r *DispatcherReconciler) enqueueCICDProxyChildren(ctx context.Context, obj client.Object) []reconcile.Request {
	parent, ok := obj.(*v1.Workload)
	if !ok || !commonworkload.IsCICDScalingRunnerSet(parent) {
		return nil
	}
	children := &v1.WorkloadList{}
	if err := r.List(ctx, children, client.MatchingFields{cicdProxyOwnerIndex: string(parent.UID)}); err != nil {
		klog.ErrorS(err, "failed to enqueue proxy changes for child runners")
		return nil
	}
	var requests []reconcile.Request
	for _, child := range children.Items {
		if !child.IsEnd() && child.DeletionTimestamp.IsZero() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&child)})
		}
	}
	return requests
}

func cicdProxyParentPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldWorkload, oldOK := e.ObjectOld.(*v1.Workload)
			newWorkload, newOK := e.ObjectNew.(*v1.Workload)
			return oldOK && newOK && commonworkload.IsCICDScalingRunnerSet(newWorkload) &&
				(commonworkload.CICDProxyEnvChanged(oldWorkload, newWorkload) ||
					v1.GetAnnotation(oldWorkload, v1.CICDProxyManagedAnnotation) != v1.GetAnnotation(newWorkload, v1.CICDProxyManagedAnnotation))
		},
	}
}
