/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"context"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func monkeyDispatchClientSets() *syncer.ClusterClientSets {
	c := &syncer.ClusterClientSets{}
	c.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", nil))
	return c
}

// TestDispatch patches generateK8sObject + CreateObject so dispatch runs its full path;
// a workload without a Service short-circuits createService/createIngress.
func TestDispatch(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&DispatcherReconciler{}), "generateK8sObject",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload, _ *syncer.ClusterClientSets) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	patches.ApplyFunc(jobutils.CreateObject,
		func(context.Context, *commonclient.ClientFactory, *unstructured.Unstructured) error {
			return nil
		})

	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	// No Service -> createService/createIngress return early.
	_, err := r.dispatch(context.Background(), w, monkeyDispatchClientSets())
	assert.NilError(t, err)
}

// TestProcessWorkloadDispatchPath patches GetClusterClientSets + GetResourceTemplate +
// GetObject(NotFound) + dispatch + markAsDispatched so processWorkload runs the
// "object not yet created" path.
func TestProcessWorkloadDispatchPath(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	cs := monkeyDispatchClientSets()
	patches.ApplyFunc(syncer.GetClusterClientSets,
		func(*commonutils.ObjectManager, string) (*syncer.ClusterClientSets, error) { return cs, nil })
	patches.ApplyFunc(commonworkload.GetResourceTemplate,
		func(context.Context, ctrlClient.Client, *v1.Workload) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "jobs"}, "w")
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&DispatcherReconciler{}), "dispatch",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload, _ *syncer.ClusterClientSets) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&DispatcherReconciler{}), "markAsDispatched",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload) error { return nil })

	r := &DispatcherReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	_, err := r.processWorkload(context.Background(), w)
	assert.NilError(t, err)
}

// TestDispatcherReconcileToProcess drives Reconcile through generateJobPort into
// processWorkload (patched) for a dispatched, non-TorchFT workload.
func TestDispatcherReconcileToProcess(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&DispatcherReconciler{}), "processWorkload",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.Workspace = "ws"
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(w).Build()
	r := &DispatcherReconciler{Client: cl}

	_, rerr := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlClient.ObjectKey{Name: "w"},
	})
	assert.NilError(t, rerr)
}