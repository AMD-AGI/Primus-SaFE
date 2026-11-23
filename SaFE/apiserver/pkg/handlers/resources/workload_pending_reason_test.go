/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockdb "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestPendingReasonActionFor(t *testing.T) {
	tests := []struct {
		name    string
		phase   v1.WorkloadPhase
		app     bool
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
		{name: "not ready application collects pods", phase: v1.WorkloadNotReady, app: true, message: "x", want: pendingReasonCollect},
		{name: "not ready non application", phase: v1.WorkloadNotReady, message: "x", want: pendingReasonNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pendingReasonActionFor(tt.phase, tt.app, tt.queue, tt.message))
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

// A job-manager log line alone is evidence for the summary, never the user-visible reason.
func TestPendingReasonWithoutFactsIsEmpty(t *testing.T) {
	const workloadID = "wl-quiet"
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
	orig := fetchPodLogs
	fetchPodLogs = func(context.Context, kubernetes.Interface, string, string, int64) ([]byte, error) {
		return []byte("failed to dispatch " + workloadID + ": quota exceeded\n"), nil
	}
	t.Cleanup(func() { fetchPodLogs = orig })

	h := &Handler{clientSet: k8sfake.NewSimpleClientset(deploy, jmPod)}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: workloadID},
		Spec:       v1.WorkloadSpec{Workspace: "ws-a"},
		Status:     v1.WorkloadStatus{Phase: v1.WorkloadPending},
	}

	evidence, fallback := h.collectPendingEvidence(context.Background(), workload)
	assert.Contains(t, evidence, "quota exceeded")
	assert.Empty(t, fallback)
	assert.Empty(t, h.resolvePendingReason(context.Background(), workload, "", ""))
}

// Pending pods are selected by the server across every namespace, since the ARC listener pod
// runs in the controller's namespace and not in the workspace.
func TestPendingPodEvidenceListsAllNamespaces(t *testing.T) {
	const (
		workloadID = "wl-arc"
		clusterID  = "cluster-a"
	)
	listener := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadID + "-listener",
			Namespace: "arc-systems",
			Labels:    map[string]string{v1.WorkloadIdLabel: workloadID},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "listener",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
			}},
		},
	}
	data := k8sfake.NewSimpleClientset(listener)
	var listNamespace, listFields string
	data.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listAction := action.(k8stesting.ListAction)
		listNamespace = listAction.GetNamespace()
		listFields = listAction.GetListRestrictions().Fields.String()
		return false, nil, nil
	})
	manager := commonutils.NewObjectManager()
	require.NoError(t, manager.Add(clusterID, k8sclient.NewClientFactoryWithOnlyClient(context.Background(), clusterID, data)))

	h := &Handler{clientManager: manager}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:   workloadID,
			Labels: map[string]string{v1.ClusterIdLabel: clusterID},
		},
		Spec: v1.WorkloadSpec{Workspace: "ws-a"},
	}

	lines, _ := h.pendingPodEvidence(context.Background(), workload)
	assert.Equal(t, metav1.NamespaceAll, listNamespace)
	assert.Equal(t, "status.phase=Pending", listFields)
	assert.Contains(t, strings.Join(lines, "\n"), "ImagePullBackOff")
}

func TestPendingPodEvidenceNotReadyApplication(t *testing.T) {
	const clusterID, workloadID = "c1", "wl-app"
	pod := func(name string, phase corev1.PodPhase, ready corev1.ConditionStatus, reason string) *corev1.Pod {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ws-a", Labels: map[string]string{v1.WorkloadIdLabel: workloadID}},
			Status: corev1.PodStatus{
				Phase:      phase,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: ready}},
			},
		}
		if reason != "" {
			p.Status.ContainerStatuses = []corev1.ContainerStatus{{
				Name:  "main",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}},
			}}
		}
		return p
	}
	data := k8sfake.NewSimpleClientset(
		pod("crashing", corev1.PodRunning, corev1.ConditionFalse, "CrashLoopBackOff"),
		pod("healthy", corev1.PodRunning, corev1.ConditionTrue, "StaleReason"),
		pod("unknown-ready", corev1.PodRunning, corev1.ConditionUnknown, "UnknownReady"),
		pod("done", corev1.PodSucceeded, corev1.ConditionFalse, "SucceededReason"),
	)
	var listFields string
	data.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listFields = action.(k8stesting.ListAction).GetListRestrictions().Fields.String()
		return false, nil, nil
	})
	manager := commonutils.NewObjectManager()
	require.NoError(t, manager.Add(clusterID, k8sclient.NewClientFactoryWithOnlyClient(context.Background(), clusterID, data)))
	h := &Handler{clientManager: manager}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: workloadID, Labels: map[string]string{v1.ClusterIdLabel: clusterID}},
		Status:     v1.WorkloadStatus{Phase: v1.WorkloadNotReady},
	}

	lines, _ := h.pendingPodEvidence(context.Background(), workload)
	text := strings.Join(lines, "\n")
	assert.Empty(t, listFields)
	assert.Contains(t, text, "CrashLoopBackOff")
	assert.Contains(t, text, "UnknownReady")
	assert.NotContains(t, text, "StaleReason")
	assert.NotContains(t, text, "SucceededReason")

	// A Pending workload keeps to Pending pods.
	workload.Status.Phase = v1.WorkloadPending
	lines, _ = h.pendingPodEvidence(context.Background(), workload)
	assert.Empty(t, lines)
	assert.Equal(t, "status.phase=Pending", listFields)
}

func TestFilterWorkloadLogLines(t *testing.T) {
	raw := "other line\n\nworker wl-1 started\nwl-1 again\n"
	got := filterWorkloadLogLines(raw, "wl-1", 1)
	assert.Equal(t, []string{"worker wl-1 started"}, got)
	assert.Empty(t, filterWorkloadLogLines(raw, "", 5))
}

type fakePendingSummarizer struct {
	reply string
	err   error
	calls int
	email string
	ntid  string
	text  string
}

func (f *fakePendingSummarizer) SummarizePendingReason(_ context.Context, email, ntid, evidence string) (string, error) {
	f.calls++
	f.email, f.ntid, f.text = email, ntid, evidence
	return f.reply, f.err
}

func pendingWorkloadForReason(name string, phase v1.WorkloadPhase, queue int, message string) *v1.Workload {
	wl := newWorkloadForLog(name, "c1", "ws-1")
	wl.Status = v1.WorkloadStatus{Phase: phase, QueuePosition: queue, Message: message}
	return wl
}

func TestGetWorkloadPendingReasonHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		workload *v1.Workload
		request  string
		wantCode int
		wantBody string
	}{
		{
			name:     "queued pending returns the scheduler message",
			workload: pendingWorkloadForReason("wl-q", v1.WorkloadPending, 2, "In queue"),
			request:  "wl-q",
			wantCode: http.StatusOK,
			wantBody: `{"reason":"In queue"}`,
		},
		{
			name:     "running workload returns an empty reason",
			workload: pendingWorkloadForReason("wl-r", v1.WorkloadRunning, 0, ""),
			request:  "wl-r",
			wantCode: http.StatusOK,
			wantBody: `{"reason":""}`,
		},
		{
			name:     "missing workload is not found",
			workload: pendingWorkloadForReason("wl-x", v1.WorkloadPending, 1, "x"),
			request:  "wl-missing",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "empty name is a bad request",
			workload: pendingWorkloadForReason("wl-y", v1.WorkloadPending, 1, "x"),
			request:  "",
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, user := newAdminHandlerWithObjects(tt.workload)
			h.SetPendingReasonSummarizer(&fakePendingSummarizer{})
			rsp := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rsp)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Set(common.UserId, user.Name)
			c.Set(common.Name, tt.request)
			h.GetWorkloadPendingReason(c)
			assert.Equal(t, tt.wantCode, rsp.Code)
			if tt.wantBody != "" {
				assert.JSONEq(t, tt.wantBody, rsp.Body.String())
			}
		})
	}
}

// TestGetWorkloadPendingReasonAfterObjectDeleted covers a workload whose Kubernetes object is
// gone (for example removed by its TTL) while the database still holds the record.
func TestGetWorkloadPendingReasonAfterObjectDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("db.enable", "true")
	t.Cleanup(func() { commonconfig.SetValue("db.enable", "") })

	h, user := newAdminHandlerWithObjects()
	ctrl := gomock.NewController(t)
	mockDB := mockdb.NewMockInterface(ctrl)
	mockDB.EXPECT().GetWorkload(gomock.Any(), "wl-ttl").Return(&dbclient.Workload{
		WorkloadId: "wl-ttl",
		Workspace:  "ws",
		Cluster:    "c1",
		GVK:        `{"group":"amd.com","version":"v1","kind":"Workload"}`,
	}, nil)
	h.dbClient = mockDB
	h.SetPendingReasonSummarizer(&fakePendingSummarizer{})

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "wl-ttl")
	h.GetWorkloadPendingReason(c)

	assert.Equal(t, http.StatusOK, rsp.Code)
	assert.JSONEq(t, `{"reason":""}`, rsp.Body.String())
}

func TestSetPendingReasonSummarizerNilHandler(t *testing.T) {
	var h *Handler
	assert.NotPanics(t, func() { h.SetPendingReasonSummarizer(&fakePendingSummarizer{}) })
}

func TestPendingReasonCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)
	named := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "u-named",
			Labels: map[string]string{v1.UserIdLabel: "u-named"},
			Annotations: map[string]string{
				v1.UserNameAnnotation:          "named",
				v1.UserEmailAnnotation:         " named@amd.com ",
				v1.UserPreferredNameAnnotation: "ntid9@amd.com",
			},
		},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}
	// The shared access controller is a process singleton bound to its first client, so this
	// test builds its own to see the users defined here.
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(named, genMockUser()).Build()
	h := &Handler{accessController: &authority.AccessController{Client: fakeClient}}
	tests := []struct {
		name      string
		userId    string
		userName  string
		wantEmail string
		wantNtid  string
	}{
		{name: "email and ntid come from the user", userId: "u-named", wantEmail: "named@amd.com", wantNtid: "ntid9"},
		{name: "user name stands in for a missing email", userId: "test-user", wantEmail: "test-user"},
		{name: "unknown user keeps the request user name", userId: "missing", userName: "fallback-name", wantEmail: "fallback-name"},
		{name: "no user id uses the request user name", userName: "only-name", wantEmail: "only-name"},
		{name: "nothing known yields an empty identity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.userId != "" {
				c.Set(common.UserId, tt.userId)
			}
			if tt.userName != "" {
				c.Set(common.UserName, tt.userName)
			}
			email, ntid := h.pendingReasonCaller(c)
			assert.Equal(t, tt.wantEmail, email)
			assert.Equal(t, tt.wantNtid, ntid)
		})
	}
}

func TestResolvePendingReason(t *testing.T) {
	provisioning := func() *v1.Workload {
		return &v1.Workload{
			ObjectMeta: metav1.ObjectMeta{Name: "wl-p"},
			Status: v1.WorkloadStatus{
				Phase: v1.WorkloadPending,
				ExternalExecution: &v1.WorkloadExternalExecution{
					ProvisioningCondition: "CapacityRevoked",
				},
			},
		}
	}
	ctx := context.Background()

	assert.Empty(t, (&Handler{}).resolvePendingReason(ctx, nil, "u@amd.com", "n"))

	t.Run("summary is used and bills the caller", func(t *testing.T) {
		s := &fakePendingSummarizer{reply: "Capacity was revoked."}
		h := &Handler{pendingSummarizer: s}
		assert.Equal(t, "Capacity was revoked.", h.resolvePendingReason(ctx, provisioning(), "u@amd.com", "ntid1"))
		assert.Equal(t, 1, s.calls)
		assert.Equal(t, "u@amd.com", s.email)
		assert.Equal(t, "ntid1", s.ntid)
		assert.Contains(t, s.text, "CapacityRevoked")
	})
	t.Run("blank email skips the summarizer", func(t *testing.T) {
		s := &fakePendingSummarizer{reply: "unused"}
		h := &Handler{pendingSummarizer: s}
		assert.Contains(t, h.resolvePendingReason(ctx, provisioning(), "  ", ""), "CapacityRevoked")
		assert.Zero(t, s.calls)
	})
	t.Run("missing summarizer keeps the strongest fact", func(t *testing.T) {
		h := &Handler{}
		assert.Contains(t, h.resolvePendingReason(ctx, provisioning(), "u@amd.com", ""), "CapacityRevoked")
	})
	t.Run("summarizer failure keeps the strongest fact", func(t *testing.T) {
		s := &fakePendingSummarizer{err: errors.New("llm down")}
		h := &Handler{pendingSummarizer: s}
		assert.Contains(t, h.resolvePendingReason(ctx, provisioning(), "u@amd.com", ""), "CapacityRevoked")
		assert.Equal(t, 1, s.calls)
	})
}

func TestPendingEvidenceHelpers(t *testing.T) {
	now := time.Now()
	t.Run("eventLastTime prefers the newest recorded field", func(t *testing.T) {
		assert.True(t, eventLastTime(nil).IsZero())
		last := metav1.NewTime(now.Add(-1 * time.Minute))
		micro := metav1.NewMicroTime(now.Add(-2 * time.Minute))
		first := metav1.NewTime(now.Add(-3 * time.Minute))
		created := metav1.NewTime(now.Add(-4 * time.Minute))
		ev := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{CreationTimestamp: created},
			FirstTimestamp: first,
			EventTime:      micro,
			LastTimestamp:  last,
		}
		assert.True(t, eventLastTime(ev).Equal(last.Time))
		ev.LastTimestamp = metav1.Time{}
		assert.True(t, eventLastTime(ev).Equal(micro.Time))
		ev.EventTime = metav1.MicroTime{}
		assert.True(t, eventLastTime(ev).Equal(first.Time))
		ev.FirstTimestamp = metav1.Time{}
		assert.True(t, eventLastTime(ev).Equal(created.Time))
	})

	t.Run("isPendingSignalEvent", func(t *testing.T) {
		assert.False(t, isPendingSignalEvent(nil))
		assert.True(t, isPendingSignalEvent(&corev1.Event{Type: corev1.EventTypeWarning, Reason: "Anything"}))
		assert.True(t, isPendingSignalEvent(&corev1.Event{Type: corev1.EventTypeNormal, Reason: "Pulling", Message: "Pulling image x"}))
		assert.True(t, isPendingSignalEvent(&corev1.Event{Type: corev1.EventTypeNormal, Reason: "TriggeredScaleUp"}))
		assert.False(t, isPendingSignalEvent(&corev1.Event{Type: corev1.EventTypeNormal, Reason: "Started", Message: "Started container"}))
	})

	t.Run("joinReason", func(t *testing.T) {
		assert.Equal(t, "p r: m", joinReason("p", "r", "m"))
		assert.Equal(t, "p r", joinReason("p", "r", ""))
		assert.Equal(t, "p: m", joinReason("p", "", "m"))
		assert.Equal(t, "p", joinReason("p", "", ""))
	})

	t.Run("evidenceRank orders specific facts first", func(t *testing.T) {
		assert.Equal(t, 4, evidenceRank("pod/a container c waiting X"))
		assert.Equal(t, 3, evidenceRank("pod/a unschedulable X"))
		assert.Equal(t, 2, evidenceRank("event X"))
		assert.Equal(t, 1, evidenceRank("provisioning X"))
		assert.Equal(t, 0, evidenceRank("other"))
	})

	t.Run("clipText", func(t *testing.T) {
		assert.Equal(t, "", clipText("abc", 0))
		assert.Equal(t, "abc", clipText("abc", 5))
		assert.Equal(t, "ab", clipText("abc", 2))
	})

	t.Run("provisioningLine", func(t *testing.T) {
		assert.Empty(t, provisioningLine(nil))
		assert.Empty(t, provisioningLine(&v1.Workload{}))
		blank := &v1.Workload{Status: v1.WorkloadStatus{ExternalExecution: &v1.WorkloadExternalExecution{ProvisioningCondition: "  "}}}
		assert.Empty(t, provisioningLine(blank))
	})
}

func TestJobManagerLogLinesSkipsUnusableSources(t *testing.T) {
	ctx := context.Background()
	assert.Nil(t, (&Handler{}).jobManagerLogLines(ctx, "wl-1"))
	assert.Nil(t, (&Handler{clientSet: k8sfake.NewSimpleClientset()}).jobManagerLogLines(ctx, ""))

	labelled := func(name string, selector *metav1.LabelSelector) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: common.PrimusSafeNamespace,
				Labels:    map[string]string{jobManagerAppLabelKey: jobManagerAppLabelValue},
			},
			Spec: appsv1.DeploymentSpec{Selector: selector},
		}
	}
	jmPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-manager-0",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{"app": "jm"},
		},
	}
	noSelector := labelled("no-selector", nil)
	withSelector := labelled("with-selector", &metav1.LabelSelector{MatchLabels: map[string]string{"app": "jm"}})

	orig := fetchPodLogs
	t.Cleanup(func() { fetchPodLogs = orig })
	fetchPodLogs = func(context.Context, kubernetes.Interface, string, string, int64) ([]byte, error) {
		return nil, errors.New("log read failed")
	}
	h := &Handler{clientSet: k8sfake.NewSimpleClientset(noSelector, withSelector, jmPod)}
	assert.Empty(t, h.jobManagerLogLines(ctx, "wl-1"))
}
