/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestPendingReasonActionFor(t *testing.T) {
	tests := []struct {
		name    string
		phase   v1.WorkloadPhase
		queue   int
		message string
		want    pendingReasonAction
	}{
		{name: "running", phase: v1.WorkloadRunning, queue: 0, message: "x", want: pendingReasonNone},
		{name: "empty phase", phase: "", queue: 1, message: "queued", want: pendingReasonNone},
		{name: "queue head", phase: v1.WorkloadPending, queue: 1, message: "insufficient gpu", want: pendingReasonMessage},
		{name: "behind others", phase: v1.WorkloadPending, queue: 3, message: " In queue ", want: pendingReasonMessage},
		{name: "queued without message", phase: v1.WorkloadPending, queue: 2, message: "  ", want: pendingReasonCollect},
		{name: "dequeued with message collects pods", phase: v1.WorkloadPending, queue: 0, message: "waiting for scale up", want: pendingReasonCollect},
		{name: "dequeued without message", phase: v1.WorkloadPending, queue: 0, message: "", want: pendingReasonCollect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pendingReasonActionFor(tt.phase, tt.queue, tt.message))
		})
	}
}

func TestApplyPendingReason(t *testing.T) {
	collect := func() (string, string) {
		return "pod/a container main waiting ImagePullBackOff: pull failed", "pod/a container main waiting ImagePullBackOff: pull failed"
	}
	tests := []struct {
		name      string
		action    pendingReasonAction
		message   string
		summarize func(string) (string, error)
		want      string
	}{
		{name: "none", action: pendingReasonNone, message: "x", want: ""},
		{name: "message", action: pendingReasonMessage, message: " In queue ", want: "In queue"},
		{
			name: "collect with message is summarized with pod evidence", action: pendingReasonCollect, message: "waiting for scale up",
			summarize: func(text string) (string, error) {
				assert.Contains(t, text, "scheduler message: waiting for scale up")
				assert.Contains(t, text, "ImagePullBackOff")
				return "The image pull failed after scale up.", nil
			},
			want: "The image pull failed after scale up.",
		},
		{
			name: "collect with message falls back to message", action: pendingReasonCollect, message: "waiting for scale up",
			summarize: func(string) (string, error) { return "", errors.New("down") },
			want:      "waiting for scale up",
		},
		{
			name: "collect summarized", action: pendingReasonCollect,
			summarize: func(text string) (string, error) {
				assert.Contains(t, text, "ImagePullBackOff")
				return "The image cannot be pulled.", nil
			},
			want: "The image cannot be pulled.",
		},
		{
			name: "collect falls back", action: pendingReasonCollect,
			summarize: func(string) (string, error) { return "", errPendingSummaryUnavailable },
			want:      "pod/a container main waiting ImagePullBackOff: pull failed",
		},
		{
			name: "nil summarizer keeps fallback", action: pendingReasonCollect, summarize: nil,
			want: "pod/a container main waiting ImagePullBackOff: pull failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyPendingReason(tt.action, tt.message, collect, tt.summarize)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollectPendingEvidence(t *testing.T) {
	const (
		workloadID = "wl-pending"
		workspace  = "ws-a"
		clusterID  = "cluster-a"
		podUID     = types.UID("pod-uid")
	)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wl-pending-0",
			Namespace: workspace,
			UID:       podUID,
			Labels:    map[string]string{v1.WorkloadIdLabel: workloadID},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "pytorch",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason:  "ImagePullBackOff",
					Message: "Back-off pulling image",
				}},
			}},
			Conditions: []corev1.PodCondition{{
				Type:    corev1.PodScheduled,
				Status:  corev1.ConditionFalse,
				Reason:  "Unschedulable",
				Message: "0/3 nodes are available",
			}},
		},
	}
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-pending-0.1", Namespace: workspace},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: pod.Name, Namespace: workspace, UID: podUID,
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "FailedScheduling",
		Message:       "0/3 nodes are available",
		LastTimestamp: metav1.NewTime(time.Now()),
	}
	stale := event.DeepCopy()
	stale.Name = "other"
	stale.InvolvedObject.UID = "other-uid"
	stale.Message = "stale event"

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "primus-safe-job-manager",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{jobManagerAppLabelKey: jobManagerAppLabelValue},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "primus-safe-job-manager"}},
		},
	}
	jmPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-manager-0",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{"app": "primus-safe-job-manager"},
		},
	}

	data := k8sfake.NewSimpleClientset(pod, event, stale)
	control := k8sfake.NewSimpleClientset(deploy, jmPod)
	manager := commonutils.NewObjectManager()
	require.NoError(t, manager.Add(clusterID, k8sclient.NewClientFactoryWithOnlyClient(context.Background(), clusterID, data)))

	orig := fetchPodLogs
	fetchPodLogs = func(_ context.Context, clientSet kubernetes.Interface, namespace, name string, _ int64) ([]byte, error) {
		assert.Equal(t, common.PrimusSafeNamespace, namespace)
		assert.Equal(t, jmPod.Name, name)
		assert.Equal(t, control, clientSet)
		return []byte("noise about something else\nfailed to dispatch " + workloadID + ": quota exceeded\n"), nil
	}
	t.Cleanup(func() { fetchPodLogs = orig })

	h := &Handler{clientSet: control, clientManager: manager}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:   workloadID,
			Labels: map[string]string{v1.ClusterIdLabel: clusterID},
		},
		Spec: v1.WorkloadSpec{Workspace: workspace},
		Status: v1.WorkloadStatus{
			Phase:         v1.WorkloadPending,
			QueuePosition: 0,
			ExternalExecution: &v1.WorkloadExternalExecution{
				ProvisioningCondition: "CapacityRevoked",
			},
		},
	}

	evidence, fallback := h.collectPendingEvidence(context.Background(), workload)
	assert.Contains(t, evidence, "ImagePullBackOff")
	assert.Contains(t, evidence, "FailedScheduling")
	assert.Contains(t, evidence, "quota exceeded")
	assert.Contains(t, evidence, "CapacityRevoked")
	assert.NotContains(t, evidence, "stale event")
	assert.NotContains(t, evidence, "noise about something else")
	assert.Contains(t, fallback, "ImagePullBackOff")
}

func TestFilterWorkloadLogLines(t *testing.T) {
	raw := "other line\n\nworker wl-1 started\nwl-1 again\n"
	got := filterWorkloadLogLines(raw, "wl-1", 1)
	assert.Equal(t, []string{"worker wl-1 started"}, got)
	assert.Empty(t, filterWorkloadLogLines(raw, "", 5))
}
