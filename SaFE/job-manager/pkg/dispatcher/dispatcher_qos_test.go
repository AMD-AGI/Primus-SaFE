/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 */

package dispatcher

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func runningResources(gpu string) string {
	raw, err := json.Marshal([]v1.WorkloadResource{{
		Replica: 1, CPU: "1", Memory: "1Gi", GPU: gpu, GPUName: common.AmdGpu,
	}})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func scaleWorkload(name, gpu string, at time.Time, priority int) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(at),
			Labels: map[string]string{
				v1.WorkspaceIdLabel: "c1",
				v1.UserIdLabel:      "alice",
			},
			Annotations: map[string]string{
				v1.WorkloadScheduledAnnotation:   "true",
				v1.QosRunningResourcesAnnotation: runningResources("1"),
			},
		},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.DeploymentKind},
			Priority:         priority,
			Resources: []v1.WorkloadResource{{
				Replica: 1, CPU: "1", Memory: "1Gi", GPU: gpu, GPUName: common.AmdGpu,
			}},
		},
	}
}

func TestDispatcherRevertsTheLaterScale(t *testing.T) {
	scheme, err := genMockScheme()
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "10"}},
	}
	earlier := scaleWorkload("earlier", "8", base, 1)
	later := scaleWorkload("later", "8", base.Add(time.Minute), 100)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(qos, earlier, later).Build()
	r := &DispatcherReconciler{Client: cl}
	ctx := context.Background()

	clear, err := r.prepareScaleQuota(ctx, earlier)
	if err != nil || !clear || earlier.Spec.Resources[0].GPU != "8" {
		t.Fatalf("the earlier scale is applied, clear %v gpu %s err %v", clear, earlier.Spec.Resources[0].GPU, err)
	}
	clear, err = r.prepareScaleQuota(ctx, later)
	if err != nil || clear || later.Spec.Resources[0].GPU != "1" {
		t.Fatalf("the later scale is restored, clear %v gpu %s err %v", clear, later.Spec.Resources[0].GPU, err)
	}
	if v1.GetAnnotation(later, v1.QosRunningResourcesAnnotation) != "" {
		t.Fatal("the restored scale drops the running-size annotation")
	}
}

func TestDispatcherScaleSameTimeKeepsHigherPriorityThenName(t *testing.T) {
	scheme, err := genMockScheme()
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "10"}},
	}
	high := scaleWorkload("z-high", "8", base, 10)
	low := scaleWorkload("a-low", "8", base, 1)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(qos, high, low).Build()
	r := &DispatcherReconciler{Client: cl}
	ctx := context.Background()
	if _, err = r.prepareScaleQuota(ctx, low); err != nil || low.Spec.Resources[0].GPU != "1" {
		t.Fatalf("the lower priority is restored, gpu %s err %v", low.Spec.Resources[0].GPU, err)
	}
	if _, err = r.prepareScaleQuota(ctx, high); err != nil || high.Spec.Resources[0].GPU != "8" {
		t.Fatalf("the higher priority is applied, gpu %s err %v", high.Spec.Resources[0].GPU, err)
	}

	left := scaleWorkload("m-left", "8", base, 5)
	right := scaleWorkload("m-right", "8", base, 5)
	cl = ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(qos, left, right).Build()
	r = &DispatcherReconciler{Client: cl}
	if _, err = r.prepareScaleQuota(ctx, right); err != nil || right.Spec.Resources[0].GPU != "1" {
		t.Fatalf("the later name is restored, gpu %s err %v", right.Spec.Resources[0].GPU, err)
	}
	if _, err = r.prepareScaleQuota(ctx, left); err != nil || left.Spec.Resources[0].GPU != "8" {
		t.Fatalf("the earlier name is applied, gpu %s err %v", left.Spec.Resources[0].GPU, err)
	}
}

func TestDispatcherClearsTheAtomRootAnnotation(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	cs := monkeyDispatchClientSets()
	patches.ApplyFunc(syncer.GetClusterClientSets,
		func(*commonutils.ObjectManager, string) (*syncer.ClusterClientSets, error) { return cs, nil })
	patches.ApplyFunc(commonworkload.GetResourceTemplate,
		func(context.Context, ctrlclient.Client, *v1.Workload) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	reflectType := reflect.TypeOf(&DispatcherReconciler{})
	patches.ApplyPrivateMethod(reflectType, "markAsDispatched",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload) error { return nil })
	patches.ApplyPrivateMethod(reflectType, "syncWorkloadToObject",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload, _ *syncer.ClusterClientSets, _ *unstructured.Unstructured) error {
			return nil
		})
	patches.ApplyPrivateMethod(reflectType, "updateService",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload, _ *syncer.ClusterClientSets, _ *unstructured.Unstructured) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})
	patches.ApplyPrivateMethod(reflectType, "updateIngress",
		func(_ *DispatcherReconciler, _ context.Context, _ *v1.Workload, _ *syncer.ClusterClientSets, _ *unstructured.Unstructured) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})

	scheme, err := genMockScheme()
	if err != nil {
		t.Fatal(err)
	}
	root := atomRootWorkload()
	raw, err := json.Marshal(root.Spec.Resources)
	if err != nil {
		t.Fatal(err)
	}
	v1.SetLabel(root, v1.ClusterIdLabel, "c1")
	v1.SetLabel(root, v1.UserIdLabel, "alice")
	v1.SetAnnotation(root, v1.WorkloadScheduledAnnotation, "true")
	v1.SetAnnotation(root, v1.WorkloadDispatchedAnnotation, "true")
	v1.SetAnnotation(root, v1.QosRunningResourcesAnnotation, string(raw))
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(root).WithStatusSubresource(root).Build()
	r := &DispatcherReconciler{Client: cl}
	ctx := context.Background()

	if _, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: ctrlclient.ObjectKey{Name: root.Name}}); err != nil {
		t.Fatalf("reconcile the atom root: %v", err)
	}
	stored := &v1.Workload{}
	if err = cl.Get(ctx, ctrlclient.ObjectKey{Name: root.Name}, stored); err != nil {
		t.Fatal(err)
	}
	if stored.IsEnd() {
		t.Fatalf("the atom root stays running, phase %s", stored.Status.Phase)
	}
	if v1.GetAnnotation(stored, v1.QosRunningResourcesAnnotation) != "" {
		t.Fatal("the running-size annotation is cleared on the atom root")
	}
}

func TestDispatcherScaleCountsOlderPendingWork(t *testing.T) {
	scheme, err := genMockScheme()
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qos := &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxResources: &v1.ResourceCap{GPU: "8"}},
	}
	queued := scaleWorkload("queued", "8", base, 1)
	delete(queued.Annotations, v1.WorkloadScheduledAnnotation)
	delete(queued.Annotations, v1.QosRunningResourcesAnnotation)
	queued.Spec.Resources[0].GPU = "8"
	running := scaleWorkload("running", "4", base.Add(time.Minute), 1)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(qos, queued, running).Build()
	r := &DispatcherReconciler{Client: cl}

	clear, err := r.prepareScaleQuota(context.Background(), running)
	if err != nil || clear || running.Spec.Resources[0].GPU != "1" {
		t.Fatalf("an older queued workload holds the quota, clear %v gpu %s err %v", clear, running.Spec.Resources[0].GPU, err)
	}
}
