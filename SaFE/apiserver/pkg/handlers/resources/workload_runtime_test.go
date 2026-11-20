/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

const userManager = "manager-user"

var conditionReasonPattern = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`)

// newRuntimeFixture adds a workspace ws1 on cluster c1, whose Qos lets Train workloads run 3h,
// and a default user who manages ws1.
func newRuntimeFixture(t *testing.T) *accessFixture {
	t.Helper()
	f := newAccessFixture(t)
	manager := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: userManager, Labels: map[string]string{v1.UserIdLabel: userManager}},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.DefaultRole}},
	}
	commonuser.AddWorkspace(manager, "ws1")
	commonuser.AddManagedWorkspace(manager, "ws1")
	require.NoError(t, f.cli.Create(t.Context(), manager))
	require.NoError(t, f.cli.Create(t.Context(), &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	}))
	require.NoError(t, f.cli.Create(t.Context(), &v1.Qos{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1.QosSpec{MaxRuntime: map[string]int{string(v1.TrainScope): 10800}},
	}))
	return f
}

func runtimeWorkload(timeout int) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "w1", Labels: map[string]string{
			v1.ClusterIdLabel: "c1", v1.UserIdLabel: userDefault,
		}},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
			Workspace:        "ws1",
			Timeout:          pointer.Int(timeout),
		},
	}
}

func (f *accessFixture) roles(t *testing.T, userId string) (*v1.User, []*v1.Role) {
	t.Helper()
	requestUser := &v1.User{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: userId}, requestUser))
	return requestUser, f.h.accessController.GetRoles(t.Context(), requestUser)
}

func TestWorkloadTimeoutPatchAboveQosLimit(t *testing.T) {
	f := newRuntimeFixture(t)
	workload := runtimeWorkload(3600)
	update := func(userId string, timeout int) (*runtimeChange, error) {
		requestUser, roles := f.roles(t, userId)
		change, err := f.h.authWorkloadUpdate(f.ctx(userId, workload.Name, ""), workload, requestUser, roles,
			&view.PatchWorkloadRequest{Timeout: &timeout})
		if err == nil {
			change.apply(workload)
			workload.Spec.Timeout = pointer.Int(timeout)
		}
		return change, err
	}

	// The owner may shorten the timeout, but not lengthen it past the qos limit or drop it.
	_, err := update(userDefault, 1800)
	require.NoError(t, err)
	_, err = update(userDefault, 4*3600)
	assert.True(t, apierrors.IsForbidden(err), "%v", err)
	assert.ErrorContains(t, err, "timeout 4h is above the limit of 3h for Train workloads in workspace ws1")
	_, err = update(userDefault, 0)
	assert.True(t, apierrors.IsForbidden(err), "%v", err)
	assert.Empty(t, v1.GetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation))

	// A manager of the workspace may, and the workload is marked.
	change, err := update(userManager, 0)
	require.NoError(t, err)
	assert.Equal(t, userManager, v1.GetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation))
	require.NotNil(t, change.condition)
	assert.Equal(t, string(v1.RuntimeExtended), change.condition.Type)
	assert.Regexp(t, conditionReasonPattern, change.condition.Reason)
	assert.Equal(t, "runtime extended from 30m to unlimited by manager-user", change.condition.Message)

	// Back within the limit, by anyone allowed to update, removes the mark.
	change, err = update(userDefault, 7200)
	require.NoError(t, err)
	assert.Nil(t, change.condition)
	assert.Empty(t, v1.GetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation))

	// A change that is not a timeout leaves the mark alone.
	requestUser, roles := f.roles(t, userDefault)
	change, err = f.h.authWorkloadUpdate(f.ctx(userDefault, workload.Name, ""), workload, requestUser, roles,
		&view.PatchWorkloadRequest{})
	require.NoError(t, err)
	assert.Nil(t, change)
}

func TestWorkloadTimeoutCreateAboveQosLimit(t *testing.T) {
	f := newRuntimeFixture(t)
	create := func(userId string, timeout int) (*v1.Workload, *metav1.Condition, error) {
		requestUser, roles := f.roles(t, userId)
		workload := runtimeWorkload(timeout)
		// What the request carried is never kept.
		v1.SetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation, "someone")
		cond, err := f.h.authWorkloadCreateRuntime(f.ctx(userId, "", ""), workload, requestUser, roles)
		return workload, cond, err
	}

	workload, cond, err := create(userDefault, 3600)
	require.NoError(t, err)
	assert.Nil(t, cond)
	assert.Empty(t, v1.GetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation))

	// A clone or resume of a workload stored without a timeout sends 0; the webhook fills the ceiling.
	for _, timeout := range []int{0, -1} {
		workload, cond, err = create(userDefault, timeout)
		require.NoError(t, err)
		assert.Nil(t, cond)
		assert.Empty(t, v1.GetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation))
	}

	_, _, err = create(userDefault, 4*3600)
	assert.True(t, apierrors.IsForbidden(err), "%v", err)
	assert.ErrorContains(t, err, "above the limit of 3h")
	assert.ErrorContains(t, err, "workload/long-runtime")

	// A new workload has no cluster label yet; its workspace names the cluster.
	requestUser, roles := f.roles(t, userDefault)
	unlabeled := runtimeWorkload(4 * 3600)
	v1.RemoveLabel(unlabeled, v1.ClusterIdLabel)
	_, err = f.h.authWorkloadCreateRuntime(f.ctx(userDefault, "", ""), unlabeled, requestUser, roles)
	assert.True(t, apierrors.IsForbidden(err), "%v", err)

	workload, cond, err = create(userManager, 5*3600)
	require.NoError(t, err)
	assert.Equal(t, userManager, v1.GetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation))
	require.NotNil(t, cond)
	assert.Regexp(t, conditionReasonPattern, cond.Reason)
	assert.Equal(t, "runtime set to 5h by manager-user, above the qos limit 3h", cond.Message)

	workload, _, err = create(userAdmin, 6*3600)
	require.NoError(t, err)
	assert.Equal(t, userAdmin, v1.GetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation))
	workload, _, err = create(userAdmin, 0)
	require.NoError(t, err)
	assert.Empty(t, v1.GetAnnotation(workload, v1.WorkloadRuntimeExtendedByAnnotation))
}

func TestAppendWorkloadConditionKeepsEveryEntry(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	workload := runtimeWorkload(3600)
	workload.Status.Conditions = []metav1.Condition{{Type: string(v1.AdminScheduled), Status: metav1.ConditionTrue,
		Reason: "Scheduled", LastTransitionTime: metav1.Now()}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workload).WithStatusSubresource(&v1.Workload{}).Build()
	h := &Handler{Client: cli}

	stored := &v1.Workload{}
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: "w1"}, stored))
	require.NoError(t, h.appendWorkloadCondition(t.Context(), stored, newRuntimeExtendedCondition("first")))
	require.NoError(t, h.appendWorkloadCondition(t.Context(), stored, newRuntimeExtendedCondition("second")))

	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: "w1"}, stored))
	var messages []string
	for _, cond := range stored.Status.Conditions {
		messages = append(messages, cond.Type+"/"+cond.Message)
	}
	assert.Equal(t, []string{"AdminScheduled/", "RuntimeExtended/first", "RuntimeExtended/second"}, messages)
}

// TestRuntimeExtendedKeptAfterControllerPhase follows the create path when a controller sets the
// phase before the apiserver publishes Pending: the Pending patch is skipped, the entry is not.
func TestRuntimeExtendedKeptAfterControllerPhase(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(runtimeWorkload(5 * 3600)).
		WithStatusSubresource(&v1.Workload{}).Build()
	h := &Handler{Client: cli}

	created := &v1.Workload{}
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: "w1"}, created))
	scheduled := created.DeepCopy()
	scheduled.Status.Phase = v1.WorkloadRunning
	require.NoError(t, cli.Status().Update(t.Context(), scheduled))

	require.NoError(t, h.updateWorkloadPhase(t.Context(), created, v1.WorkloadPending, nil))
	require.NoError(t, h.appendWorkloadCondition(t.Context(), created, newRuntimeExtendedCondition("set")))

	stored := &v1.Workload{}
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: "w1"}, stored))
	assert.Equal(t, v1.WorkloadRunning, stored.Status.Phase)
	require.Len(t, stored.Status.Conditions, 1)
	assert.Equal(t, string(v1.RuntimeExtended), stored.Status.Conditions[0].Type)
}

func TestFormatRuntime(t *testing.T) {
	for seconds, want := range map[int]string{0: "unlimited", -1: "unlimited", 90: "1m30s", 1800: "30m",
		3600: "1h", 5400: "1h30m", 10800: "3h", 3661: "1h1m1s"} {
		assert.Equal(t, want, formatRuntime(seconds), "%d", seconds)
	}
}
