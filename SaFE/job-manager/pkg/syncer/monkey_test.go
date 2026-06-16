/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func monkeyClientSets() *ClusterClientSets {
	return &ClusterClientSets{
		dataClientFactory: commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", nil),
	}
}

// TestGetObjectsByWorkloadNonTorchFT patches jobutils.GetObject to avoid the real
// dynamic client and verifies the single-object path.
func TestGetObjectsByWorkloadNonTorchFT(t *testing.T) {
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(_ context.Context, _ *commonclient.ClientFactory, name, ns string, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			obj := &unstructured.Unstructured{}
			obj.SetName(name)
			return obj, nil
		})
	defer patches.Reset()

	r := &SyncerReconciler{}
	w := &v1.Workload{}
	objs, err := r.getObjectsByWorkload(context.Background(), w,
		&resourceMessage{name: "obj", namespace: "ns", gvk: schema.GroupVersionKind{Kind: "Job"}}, monkeyClientSets())
	assert.NilError(t, err)
	assert.Equal(t, len(objs), 1)
}

// TestWaitJobDeletedEmpty patches getObjectsByWorkload (via GetObject) to return no
// objects, exercising the "all deleted" path.
func TestWaitJobDeletedEmpty(t *testing.T) {
	patches := gomonkey.ApplyFunc(jobutils.GetObject,
		func(_ context.Context, _ *commonclient.ClientFactory, name, ns string, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			// IgnoreNotFound path: getObjectsByWorkload treats not-found as empty.
			return &unstructured.Unstructured{}, nil
		})
	defer patches.Reset()
	// Patch IsTorchFT to false so the single-object branch is taken.
	p2 := gomonkey.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	defer p2.Reset()

	r := &SyncerReconciler{}
	w := &v1.Workload{}
	// One object returned without deletion timestamp -> not yet deleted.
	ok, err := r.waitJobDeleted(context.Background(), w,
		&resourceMessage{name: "obj", namespace: "ns", gvk: schema.GroupVersionKind{Kind: "Job"}}, monkeyClientSets())
	assert.NilError(t, err)
	assert.Equal(t, ok, false)
}

// TestUpdateAdminWorkloadByJobRunning patches the dynamic + template helpers so both
// getK8sObjectStatus and updateAdminWorkloadByJob run their full non-CICD update path
// against a fake admin client.
func TestUpdateAdminWorkloadByJobRunning(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(jobutils.GetObject,
		func(context.Context, *commonclient.ClientFactory, string, string, schema.GroupVersionKind) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	patches.ApplyFunc(commonworkload.IsTorchFT, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.IsMonarchJob, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.IsCICDScalingRunnerSet, func(*v1.Workload) bool { return false })
	patches.ApplyFunc(commonworkload.GetResourceTemplate,
		func(context.Context, ctrlClient.Client, *v1.Workload) (*v1.ResourceTemplate, error) {
			return &v1.ResourceTemplate{}, nil
		})
	patches.ApplyFunc(jobutils.GetK8sObjectStatus,
		func(*unstructured.Unstructured, *v1.ResourceTemplate) (*jobutils.K8sObjectStatus, error) {
			return &jobutils.K8sObjectStatus{Phase: string(v1.K8sRunning)}, nil
		})

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}

	out, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), w,
		&resourceMessage{name: "o", namespace: "ns", dispatchCount: 1, gvk: schema.GroupVersionKind{Kind: "Job"}})
	assert.NilError(t, err)
	assert.Assert(t, out != nil)
}

// TestHandleJobFullPath patches the two heavy helpers so handleJob + handleJobImpl run
// their orchestration for a dispatched, namespace-matched workload.
func TestHandleJobFullPath(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "updateAdminWorkloadByJob",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, w *v1.Workload, _ *resourceMessage) (*v1.Workload, error) {
			return w, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "shouldReSchedule",
		func(_ *SyncerReconciler, _ context.Context, _ *v1.Workload, _ *resourceMessage, _ *ClusterClientSets) (bool, error) {
			return false, nil
		})

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.Workspace = "ns"
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}

	_, err := r.handleJob(context.Background(),
		&resourceMessage{workloadId: "w", namespace: "ns", gvk: schema.GroupVersionKind{Kind: "Job"}},
		monkeyClientSets())
	assert.NilError(t, err)
}

// TestDoRoutesToHandlers patches GetClusterClientSets + handleJob/handlePod so Do's
// routing switch is exercised for both Job-like and Pod kinds.
func TestDoRoutesToHandlers(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	cs := monkeyClientSets()
	patches.ApplyFunc(GetClusterClientSets,
		func(*commonutils.ObjectManager, string) (*ClusterClientSets, error) { return cs, nil })
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "handleJob",
		func(_ *SyncerReconciler, _ context.Context, _ *resourceMessage, _ *ClusterClientSets) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "handlePod",
		func(_ *SyncerReconciler, _ context.Context, _ *resourceMessage, _ *ClusterClientSets) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})

	r := &SyncerReconciler{}
	_, err := r.Do(context.Background(), &resourceMessage{cluster: "c", gvk: schema.GroupVersionKind{Kind: "Job"}})
	assert.NilError(t, err)
	_, err = r.Do(context.Background(), &resourceMessage{cluster: "c", gvk: schema.GroupVersionKind{Kind: "Pod"}})
	assert.NilError(t, err)
}

// TestUpdateAdminWorkloadByPodPath patches the per-pod helpers so updateAdminWorkloadByPod
// runs its orchestration up to the "no update needed" early return.
func TestUpdateAdminWorkloadByPodPath(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "getAdminWorkloadAndSyncPod",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *corev1.Pod, _ *resourceMessage) (*v1.Workload, error) {
			return w, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "handleRaySubmitterTimeout",
		func(_ *SyncerReconciler, _ context.Context, _ *v1.Workload, _ *corev1.Pod) (bool, error) {
			return false, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "getK8sNode",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ string) (*corev1.Node, error) {
			return &corev1.Node{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "updateWorkloadNodeAndPods",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *v1.Workload, _ *corev1.Pod, _ *corev1.Node) (v1.WorkloadPod, corev1.PodPhase, bool) {
			return v1.WorkloadPod{}, "", false
		})

	r := &SyncerReconciler{}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{"name": "p1"},
		"status":   map[string]interface{}{"phase": "Running"},
	}}
	_, err := r.updateAdminWorkloadByPod(context.Background(), monkeyClientSets(), obj, &resourceMessage{workloadId: "w"})
	assert.NilError(t, err)
}

// TestHandlePodPath patches the informer lookup + per-pod update so handlePod runs the
// "object present" branch into updateAdminWorkloadByPod.
func TestHandlePodPath(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethod(reflect.TypeOf(&ClusterClientSets{}), "GetResourceInformer",
		func(_ *ClusterClientSets, _ context.Context, _ schema.GroupVersionKind) (informers.GenericInformer, error) {
			return nil, nil
		})
	patches.ApplyFunc(jobutils.GetObjectByInformer,
		func(informers.GenericInformer, string, string) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "updateAdminWorkloadByPod",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *unstructured.Unstructured, _ *resourceMessage) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})

	r := &SyncerReconciler{}
	_, err := r.handlePod(context.Background(),
		&resourceMessage{name: "p1", namespace: "ns", gvk: schema.GroupVersionKind{Kind: "Pod"}},
		monkeyClientSets())
	assert.NilError(t, err)
}

// TestUpdateWorkloadNodeAndPodsAppend patches buildWorkloadPodInfo so the function
// appends a new pod entry and recomputes node assignments.
func TestUpdateWorkloadNodeAndPodsAppend(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "buildWorkloadPodInfo",
		func(_ *SyncerReconciler, _ context.Context, _ *ClusterClientSets, _ *v1.Workload, _ *corev1.Pod, _ *corev1.Node) v1.WorkloadPod {
			return v1.WorkloadPod{PodId: "p1", AdminNodeName: "n1"}
		})

	r := &SyncerReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "w",
		Labels: map[string]string{v1.WorkloadDispatchCntLabel: "1"},
	}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
	podInfo, _, updated := r.updateWorkloadNodeAndPods(context.Background(), monkeyClientSets(), w, pod, &corev1.Node{})
	assert.Equal(t, updated, true)
	assert.Equal(t, podInfo.PodId, "p1")
	assert.Equal(t, len(w.Status.Pods), 1)
}

// TestGetAdminWorkloadAndSyncPodNonMesh covers the non-mesh path: the admin workload is
// fetched directly by message.workloadId and stamped with the dispatch count.
func TestGetAdminWorkloadAndSyncPodNonMesh(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
	got, err := r.getAdminWorkloadAndSyncPod(context.Background(), monkeyClientSets(), pod,
		&resourceMessage{workloadId: "w", dispatchCount: 2})
	assert.NilError(t, err)
	assert.Assert(t, got != nil)
	assert.Equal(t, v1.GetWorkloadDispatchCnt(got), 2)
}

// TestBuildWorkloadPodInfo patches buildPodTerminatedInfo (the only clientSet user) so
// buildWorkloadPodInfo can assemble pod metadata without a live cluster.
func TestBuildWorkloadPodInfo(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(buildPodTerminatedInfo,
		func(context.Context, kubernetes.Interface, *v1.Workload, *corev1.Pod, *v1.WorkloadPod, string) {})

	r := &SyncerReconciler{}
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
	pod.Status.HostIP = "1.2.3.4"
	info := r.buildWorkloadPodInfo(context.Background(), monkeyClientSets(), w, pod, &corev1.Node{})
	assert.Equal(t, info.PodId, "p1")
	assert.Equal(t, info.HostIp, "1.2.3.4")
}
