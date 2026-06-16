/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func timeNow() time.Time { return time.Now() }

func newPrewarmFactory(cs *k8sfake.Clientset) *commonclient.ClientFactory {
	return commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
}

func TestPrewarmCreateDaemonSet(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	factory := newPrewarmFactory(cs)
	ds, err := r.createPrewarmDaemonSet(context.Background(), factory, "ds1", "img:1", "ws1")
	assert.NoError(t, err)
	assert.Equal(t, "ds1", ds.Name)
}

func TestPrewarmDaemonSetExists(t *testing.T) {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: common.PrimusSafeNamespace}}
	cs := k8sfake.NewSimpleClientset(ds)
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	factory := newPrewarmFactory(cs)
	exists, err := r.daemonSetExists(context.Background(), factory, "ds1")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestPrewarmDeleteDaemonSet(t *testing.T) {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: common.PrimusSafeNamespace}}
	cs := k8sfake.NewSimpleClientset(ds)
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	factory := newPrewarmFactory(cs)
	assert.NoError(t, r.deleteDaemonSet(context.Background(), factory, "ds1"))
}

func TestPrewarmGetFailedPodsInfo(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{"app": "ds1"},
		},
		Spec:   corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	cs := k8sfake.NewSimpleClientset(pod)
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	factory := newPrewarmFactory(cs)
	info := r.getFailedPodsInfo(context.Background(), factory, "ds1")
	assert.Contains(t, info, "node1")
}

func TestPrewarmGetNodesDetail(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{"app": "ds1"},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
		},
	}
	cs := k8sfake.NewSimpleClientset(pod)
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	factory := newPrewarmFactory(cs)
	detail := r.getNodesDetail(context.Background(), factory, "ds1")
	assert.Contains(t, detail, "node1")
	assert.Contains(t, detail, "Ready")
}

func TestPrewarmDoCreatesDaemonSet(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobPrewarmType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterImage, Value: "img:1"},
				{Name: v1.ParameterWorkspace, Value: "ws1"},
			},
		},
	}
	cs := k8sfake.NewSimpleClientset()
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}

	patches := gomonkey.ApplyFunc(rmutils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return newPrewarmFactory(cs), nil
		})
	defer patches.Reset()

	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	_, err = cs.AppsV1().DaemonSets(common.PrimusSafeNamespace).Get(context.Background(), "j1", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestPrewarmCheckAndUpdateJobStatusCompleted(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobPrewarmType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning, StartedAt: &metav1.Time{Time: timeNow()}},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Namespace: common.PrimusSafeNamespace},
		Status:     appsv1.DaemonSetStatus{NumberReady: 2, DesiredNumberScheduled: 2},
	}
	cs := k8sfake.NewSimpleClientset(ds)
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}

	patches := gomonkey.ApplyFunc(rmutils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return newPrewarmFactory(cs), nil
		})
	defer patches.Reset()

	_, err := r.checkAndUpdateJobStatus(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobSucceeded, job.Status.Phase)
}

func TestPrewarmCheckAndUpdateJobStatusTimeout(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobPrewarmType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning, StartedAt: &metav1.Time{Time: time.Now().Add(-24 * time.Hour)}},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Namespace: common.PrimusSafeNamespace},
		Status:     appsv1.DaemonSetStatus{NumberReady: 1, DesiredNumberScheduled: 3},
	}
	cs := k8sfake.NewSimpleClientset(ds)
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	patches := gomonkey.ApplyFunc(rmutils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return newPrewarmFactory(cs), nil
		})
	defer patches.Reset()
	// Elapsed far exceeds prewarm timeout -> failed completion.
	_, err := r.checkAndUpdateJobStatus(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobFailed, job.Status.Phase)
}

func TestPrewarmCheckAndUpdateJobStatusProgress(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobPrewarmType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning, StartedAt: &metav1.Time{Time: time.Now()}},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Namespace: common.PrimusSafeNamespace},
		Status:     appsv1.DaemonSetStatus{NumberReady: 1, DesiredNumberScheduled: 3},
	}
	cs := k8sfake.NewSimpleClientset(ds)
	r := &PrewarmJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	patches := gomonkey.ApplyFunc(rmutils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return newPrewarmFactory(cs), nil
		})
	defer patches.Reset()
	// In progress (ready<desired, not timed out) -> requeue, updates progress.
	res, err := r.checkAndUpdateJobStatus(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)
}

func TestSetOutputParam(t *testing.T) {
	out := setOutputParam(nil, "a", "1")
	assert.Len(t, out, 1)
	out = setOutputParam(out, "a", "2")
	assert.Len(t, out, 1)
	assert.Equal(t, "2", out[0].Value)
}
