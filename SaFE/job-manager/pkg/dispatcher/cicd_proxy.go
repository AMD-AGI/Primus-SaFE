/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	cicdProxyOwnerIndex     = "cicdProxyOwnerUID"
	cicdProxyRelayContainer = "proxy-relay"
	cicdProxyCredentialVol  = "proxy-credential"
	cicdProxyHTTPEnv        = "http_proxy"
	cicdProxyHTTPSEnv       = "https_proxy"
)

// configureCICDProxyRelay activates the chart relay only for credentialed proxies and removes its
// pod fields otherwise, so a cluster-wide relay template remains valid for unproxied workloads.
func configureCICDProxyRelay(obj *unstructured.Unstructured, workload, source *v1.Workload,
	resourceSpec v1.ResourceSpec) (bool, error) {
	if !commonworkload.IsCICDProxyManaged(source) {
		return false, removeCICDProxyRelay(obj, workload, resourceSpec)
	}
	config, err := commonworkload.ParseCICDProxy(source.Spec.Env)
	if err != nil {
		return false, err
	}
	if config == nil || config.CredentialSecret == "" {
		return false, removeCICDProxyRelay(obj, workload, resourceSpec)
	}
	upstream, err := url.Parse(config.URL)
	if err != nil {
		return false, err
	}
	port := upstream.Port()
	if port == "" {
		port = "3128"
	}
	containers, path, err := getContainers(workload, obj, resourceSpec)
	if err != nil {
		return false, err
	}
	relay, err := findCICDProxyRelay(containers)
	if err != nil {
		return false, err
	}
	initContainersPath := podSpecPath(workload, &resourceSpec, "initContainers")
	initContainers, found, err := jobutils.NestedSlice(obj.Object, initContainersPath)
	if err != nil {
		return false, err
	}
	if relay == nil && found {
		relay, err = findCICDProxyRelay(initContainers)
		if err != nil {
			return false, err
		}
	}
	if relay == nil {
		return false, removeCICDProxyRelay(obj, workload, resourceSpec)
	}
	updateContainerEnv(map[string]string{
		"PROXY_UPSTREAM_HOST": upstream.Hostname(), "PROXY_UPSTREAM_PORT": port,
	}, relay, nil)
	for _, entry := range containers {
		container, ok := entry.(map[string]interface{})
		if !ok {
			return false, fmt.Errorf("spec.template: expected a container object")
		}
		if container["name"] == v1.GetMainContainer(workload) {
			endpoint := "http://127.0.0.1:" + strconv.Itoa(commonconfig.GetCICDProxyRelayPort())
			updateContainerEnv(map[string]string{
				cicdProxyHTTPEnv: endpoint, cicdProxyHTTPSEnv: endpoint,
			}, container, nil)
		}
	}
	if err = jobutils.SetNestedField(obj.Object, containers, path); err != nil {
		return false, err
	}
	if found {
		if err = jobutils.SetNestedField(obj.Object, initContainers, initContainersPath); err != nil {
			return false, err
		}
	}
	return true, bindCICDProxyCredential(obj, workload, resourceSpec, config.CredentialSecret)
}

func findCICDProxyRelay(containers []interface{}) (map[string]interface{}, error) {
	for _, entry := range containers {
		container, ok := entry.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("spec.template: expected a container object")
		}
		if container["name"] == cicdProxyRelayContainer {
			return container, nil
		}
	}
	return nil, nil
}

func removeCICDProxyRelay(obj *unstructured.Unstructured, workload *v1.Workload,
	resourceSpec v1.ResourceSpec) error {
	for _, field := range []string{"containers", "initContainers", "volumes"} {
		name := cicdProxyRelayContainer
		if field == "volumes" {
			name = cicdProxyCredentialVol
		}
		path := podSpecPath(workload, &resourceSpec, field)
		entries, found, err := jobutils.NestedSlice(obj.Object, path)
		if err != nil || !found {
			if err != nil {
				return err
			}
			continue
		}
		filtered := make([]interface{}, 0, len(entries))
		mainContainerUpdated := false
		for _, entry := range entries {
			item, ok := entry.(map[string]interface{})
			if !ok {
				return fmt.Errorf("%s: expected an object", strings.Join(path, "."))
			}
			if field == "containers" && item["name"] == v1.GetMainContainer(workload) {
				updateContainerEnv(nil, item, []string{cicdProxyHTTPEnv, cicdProxyHTTPSEnv})
				mainContainerUpdated = true
			}
			if item["name"] != name {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) != len(entries) || mainContainerUpdated {
			if err = jobutils.SetNestedField(obj.Object, filtered, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func bindCICDProxyCredential(obj *unstructured.Unstructured, workload *v1.Workload,
	resourceSpec v1.ResourceSpec, secretName string) error {
	path := podSpecPath(workload, &resourceSpec, "volumes")
	volumes, found, err := jobutils.NestedSlice(obj.Object, path)
	if err != nil || !found {
		return err
	}
	for _, entry := range volumes {
		volume, ok := entry.(map[string]interface{})
		if !ok || volume["name"] != cicdProxyCredentialVol {
			continue
		}
		secret, ok := volume["secret"].(map[string]interface{})
		if !ok {
			secret = map[string]interface{}{}
		}
		secret["secretName"] = secretName
		volume["secret"] = secret
		return jobutils.SetNestedField(obj.Object, volumes, path)
	}
	return nil
}

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
	noProxy := commonworkload.CICDProxyNoProxy(source, commonconfig.GetCICDNoProxy(), config.NoProxy)
	if len(noProxy) > 0 {
		entries := make([]interface{}, len(noProxy))
		for i, entry := range noProxy {
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
		if key == common.NoProxy && source.Spec.Env[common.ProxyUrl] != "" {
			value = strings.Join(commonworkload.CICDProxyNoProxy(source, commonconfig.GetCICDNoProxy(), strings.Split(value, ",")), ",")
			exists = true
		}
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
	env, err := commonworkload.CICDProxyEnv(workload.Spec.Env)
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
	relay, err := configureCICDProxyRelay(desired, workload, source, rt.Spec.ResourceSpecs[0])
	if err != nil {
		return err
	}
	if relay {
		unstructured.RemoveNestedField(desired.Object, "spec", "proxySecretRef")
	} else if scaleRunnerId := v1.GetLabel(workload, v1.CICDScaleRunnerIdLabel); scaleRunnerId != "" && clientSets != nil {
		owner, getErr := jobutils.GetObject(ctx,
			clientSets.ClientFactory(), scaleRunnerId, workload.Spec.Workspace, rt.ToSchemaGVK())
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				klog.V(4).InfoS("skipping runner proxy sync because owner scale runner is gone",
					"workload", workload.Name, "owner", scaleRunnerId)
				return nil
			}
			return fmt.Errorf("failed to get owner scale runner: %w", getErr)
		}
		if err = inheritCICDProxySecretRef(desired, owner); err != nil {
			return err
		}
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
