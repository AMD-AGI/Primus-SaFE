/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	commonctrl "github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func TestDumpLogReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Finalizers: []string{v1.OpsJobFinalizer}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobDumpLogType},
	}
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestExportImageReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Finalizers: []string{v1.OpsJobFinalizer}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestPrewarmReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Finalizers: []string{v1.OpsJobFinalizer}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobPrewarmType},
	}
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestPrewarmCleanupDaemonSet(t *testing.T) {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "j1", Namespace: common.PrimusSafeNamespace}}
	cs := k8sfake.NewSimpleClientset(ds)
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}

	patches := gomonkey.ApplyFunc(rmutils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs), nil
		})
	defer patches.Reset()
	assert.NoError(t, r.cleanupDaemonSet(context.Background(), job))
}

func TestExportImageDoMissingWorkloadId(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
}

func exportJob(name, workloadId, image string) *v1.OpsJob {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobExportImageType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterWorkload, Value: workloadId},
				{Name: v1.ParameterImage, Value: image},
			},
		},
		Status: v1.OpsJobStatus{Phase: v1.OpsJobRunning},
	}
	return job
}

func TestExportImageDoWorkloadBranches(t *testing.T) {
	ctx := context.Background()

	// workload missing -> failed
	t.Run("workload missing", func(t *testing.T) {
		job := exportJob("e1", "wl-missing", "img:1")
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
		_, err := r.Do(ctx, "e1")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})

	// workload with no pods -> failed
	t.Run("workload no pods", func(t *testing.T) {
		job := exportJob("e2", "wl2", "img:1")
		wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl2"}}
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
		_, err := r.Do(ctx, "e2")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e2"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})

	// workload pod scheduled to no node -> failed
	t.Run("pod empty node", func(t *testing.T) {
		job := exportJob("e3", "wl3", "img:1")
		wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl3"}}
		wl.Status.Pods = []v1.WorkloadPod{{AdminNodeName: ""}}
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
		_, err := r.Do(ctx, "e3")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e3"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})

	// admin node missing -> failed
	t.Run("node missing", func(t *testing.T) {
		job := exportJob("e4", "wl4", "img:1")
		wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl4"}}
		wl.Status.Pods = []v1.WorkloadPod{{AdminNodeName: "n-missing"}}
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
		_, err := r.Do(ctx, "e4")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e4"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})
}

func TestQueueControllerStarts(t *testing.T) {
	ctx := context.Background()

	dl := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	dl.Controller = commonctrl.NewController[string](dl, 0)
	dl.start(ctx)

	ei := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	ei.Controller = commonctrl.NewController[string](ei, 0)
	ei.start(ctx)

	pw := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	pw.Controller = commonctrl.NewController[string](pw, 0)
	pw.start(ctx)
}

func TestPrewarmDoMissingParams(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobPrewarmType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning},
	}
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
}

func TestPrewarmUpdatePrewarmProgress(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	err := r.updatePrewarmProgress(context.Background(), "j1", 50, 1, 2)
	assert.NoError(t, err)
}

func TestAddonAddFailedNodeCondition(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	r := &AddonJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job), allJobs: map[string]*AddonJob{}}
	r.addFailedNodeCondition(context.Background(), "j1", "node1", "boom")
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "j1"}, updated))
}

func TestCleanupJobRelatedInfoControllers(t *testing.T) {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"}}
	base := newBaseWithObjs(t, job)
	// Each controller's cleanupJobRelatedInfo delegates to common cleanup which
	// lists/deletes related resources via the fake client; should not error.
	cd := &CDJobReconciler{OpsJobBaseReconciler: base}
	assert.NoError(t, cd.cleanupJobRelatedInfo(context.Background(), job))
	dl := &DownloadJobReconciler{OpsJobBaseReconciler: base}
	assert.NoError(t, dl.cleanupJobRelatedInfo(context.Background(), job))
	pf := &PreflightJobReconciler{OpsJobBaseReconciler: base}
	assert.NoError(t, pf.cleanupJobRelatedInfo(context.Background(), job))
	ev := &EvaluationJobReconciler{OpsJobBaseReconciler: base}
	assert.NoError(t, ev.cleanupJobRelatedInfo(context.Background(), job))
	ad := &AddonJobReconciler{OpsJobBaseReconciler: base, allJobs: map[string]*AddonJob{}}
	assert.NoError(t, ad.cleanupJobRelatedInfo(context.Background(), job))
}

func TestDatasetSaveTriedWorkspaces(t *testing.T) {
	r := &DatasetDownloadController{}
	ds := &dbclient.Dataset{DatasetId: "ds1"}
	r.saveTriedWorkspaces(context.Background(), ds, map[string][]string{"/wekafs": {"ws1"}})
	assert.Contains(t, ds.TriedWorkspaces, "ws1")
}
