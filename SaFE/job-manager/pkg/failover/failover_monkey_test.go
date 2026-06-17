/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package failover

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

func failoverClientSets() *syncer.ClusterClientSets {
	c := &syncer.ClusterClientSets{}
	c.SetClientFactory(commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c", nil))
	return c
}

// TestHandleFailover patches GetClusterClientSets + DeleteObjectsByWorkload so handle
// runs its full path (delete data-plane objects, add failover condition).
func TestHandleFailover(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	cs := failoverClientSets()
	patches.ApplyFunc(syncer.GetClusterClientSets,
		func(*commonutils.ObjectManager, string) (*syncer.ClusterClientSets, error) { return cs, nil })
	patches.ApplyFunc(jobutils.DeleteObjectsByWorkload,
		func(context.Context, ctrlClient.Client, *commonclient.ClientFactory, *v1.Workload) (bool, error) {
			return false, nil
		})

	w := dispatchedWorkload("w")
	cl := ctrlfake.NewClientBuilder().
		WithScheme(failoverScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &FailoverReconciler{Client: cl, clusterClientSets: commonutils.NewObjectManager()}

	_, err := r.handle(context.Background(), w)
	assert.NilError(t, err)
}

// TestReconcileFailoverToHandle drives Reconcile through to handle for a normal,
// dispatched, non-TorchFT workload.
func TestReconcileFailoverToHandle(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	cs := failoverClientSets()
	patches.ApplyFunc(syncer.GetClusterClientSets,
		func(*commonutils.ObjectManager, string) (*syncer.ClusterClientSets, error) { return cs, nil })
	patches.ApplyFunc(jobutils.DeleteObjectsByWorkload,
		func(context.Context, ctrlClient.Client, *commonclient.ClientFactory, *v1.Workload) (bool, error) {
			return false, nil
		})

	w := dispatchedWorkload("w")
	cl := ctrlfake.NewClientBuilder().
		WithScheme(failoverScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &FailoverReconciler{Client: cl, clusterClientSets: commonutils.NewObjectManager()}

	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlClient.ObjectKey{Name: "w"},
	})
	assert.NilError(t, err)
}

// TestHandleFaultEventImpl patches the cluster/client + workload lookups so the fault
// handler drives a workload through addFailoverCondition and enqueues it.
func TestHandleFaultEventImpl(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	cs := failoverClientSets()
	patches.ApplyPrivateMethod(reflect.TypeOf(&FailoverReconciler{}), "getClusterClientSets",
		func(_ *FailoverReconciler, _ string) *syncer.ClusterClientSets { return cs })
	patches.ApplyPrivateMethod(reflect.TypeOf(&FailoverReconciler{}), "getWorkloadsOnFaultNode",
		func(_ *FailoverReconciler, _ context.Context, _ *syncer.ClusterClientSets, _ *v1.Fault) ([]string, error) {
			return []string{"w"}, nil
		})

	w := dispatchedWorkload("w")
	w.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour))
	cl := ctrlfake.NewClientBuilder().
		WithScheme(failoverScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &FailoverReconciler{Client: cl, clusterClientSets: commonutils.NewObjectManager()}

	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f", CreationTimestamp: metav1.NewTime(time.Now())}}
	fault.Spec.Node = &v1.FaultNode{AdminName: "node-1", ClusterName: "c"}
	fault.Spec.MonitorId = "mon"

	q := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	r.handleFaultEventImpl(context.Background(), fault, q)
	// The workload should be enqueued for reconciliation.
	assert.Equal(t, q.Len(), 1)
}