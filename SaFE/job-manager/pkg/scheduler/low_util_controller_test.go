/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

var lowUtilTestNow = time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

type fakeGPUWindow struct {
	windows map[string]robustclient.WorkloadGPUWindow
	// Results of one window, used instead of windows when present.
	byWindow map[string]map[string]robustclient.WorkloadGPUWindow
	err      error
	calls    []lowUtilQuery
}

func (f *fakeGPUWindow) GetWorkloadGPUWindow(_ context.Context, cluster string,
	uids []string, window, step string) ([]robustclient.WorkloadGPUWindow, error) {
	f.calls = append(f.calls, lowUtilQuery{cluster: cluster, window: window})
	if f.err != nil {
		return nil, f.err
	}
	if step != lowUtilStep {
		return nil, fmt.Errorf("unexpected step %s", step)
	}
	results := f.windows
	if byUID, ok := f.byWindow[window]; ok {
		results = byUID
	}
	var out []robustclient.WorkloadGPUWindow
	for _, uid := range uids {
		if w, ok := results[uid]; ok {
			out = append(out, w)
		}
	}
	return out, nil
}

func lowUtilPolicy(mode v1.LowUtilizationMode) v1.LowUtilizationPolicy {
	return v1.LowUtilizationPolicy{Threshold: 5, Window: "1h", Mode: mode, GracePeriod: "30m", NotifyBefore: "30m"}
}

func lowUtilTestWorkspace(policy v1.LowUtilizationPolicy) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: v1.WorkspaceSpec{
			LowUtilization: map[v1.WorkspaceScope]v1.LowUtilizationPolicy{v1.TrainScope: policy},
		},
	}
}

// lowUtilTestWorkload is a training workload that started two hours ago on the given nodes.
func lowUtilTestWorkload(name string, nodes ...string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		UID:    types.UID(name + "-uid"),
		Labels: map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	w.Spec.Workspace = "ws"
	w.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	w.Status.Phase = v1.WorkloadRunning
	w.Status.StartTime = &metav1.Time{Time: lowUtilTestNow.Add(-2 * time.Hour)}
	for _, n := range nodes {
		w.Status.NodeUsage = append(w.Status.NodeUsage, v1.NodePodUsage{Node: n})
	}
	return w
}

func gpuWindow(uid string, max float64, samples int, nodes ...robustclient.GPUWindowNode) robustclient.WorkloadGPUWindow {
	return robustclient.WorkloadGPUWindow{
		UID:     uid,
		Overall: robustclient.GPUWindowOverall{Max: max, Avg: max, Samples: samples, ExpectedSamples: 60},
		Nodes:   nodes,
	}
}

func newLowUtilTestController(t *testing.T, src gpuWindowSource,
	objs ...ctrlclient.Object) (*LowUtilizationController, ctrlclient.Client, *record.FakeRecorder) {
	t.Helper()
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	rec := record.NewFakeRecorder(10)
	r := &LowUtilizationController{
		Client: cl, source: src, recorder: rec,
		now: func() time.Time { return lowUtilTestNow },
	}
	return r, cl, rec
}

func getLowUtilWorkload(t *testing.T, cl ctrlclient.Client, name string) (*v1.Workload, error) {
	t.Helper()
	w := &v1.Workload{}
	err := cl.Get(context.Background(), ctrlclient.ObjectKey{Name: name}, w)
	return w, err
}

func TestDecideLowUtilAverage(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	mode, rule := policy.Mode, policy.EffectiveRules()[0]
	idle := gpuWindow("u", 3, 60)
	busy := gpuWindow("u", 40, 60)
	sparse := gpuWindow("u", 3, 53)
	noExpected := gpuWindow("u", 3, 0)
	noExpected.Overall.ExpectedSamples = 0
	edge := gpuWindow("u", 3, 54)

	d := decideLowUtil(mode, rule, &idle)
	assert.Equal(t, d.state, lowUtilIdle)
	assert.Equal(t, d.observed, 3.0)
	assert.Equal(t, decideLowUtil(mode, rule, &busy).state, lowUtilActive)
	assert.Equal(t, decideLowUtil(mode, rule, &sparse).state, lowUtilUnknown)
	assert.Equal(t, decideLowUtil(mode, rule, &noExpected).state, lowUtilUnknown)
	assert.Equal(t, decideLowUtil(mode, rule, &edge).state, lowUtilIdle)
	assert.Equal(t, decideLowUtil(mode, rule, nil).state, lowUtilUnknown)

	// The threshold is exclusive.
	at := gpuWindow("u", 5, 60)
	assert.Equal(t, decideLowUtil(mode, rule, &at).state, lowUtilActive)
}

func TestDecideLowUtilMin(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationMin)
	mode, rule := policy.Mode, policy.EffectiveRules()[0]
	n := func(name string, max float64, samples int) robustclient.GPUWindowNode {
		return robustclient.GPUWindowNode{Node: name, Max: max, Samples: samples}
	}

	oneIdle := gpuWindow("u", 80, 60, n("a", 80, 60), n("b", 2, 60))
	d := decideLowUtil(mode, rule, &oneIdle)
	assert.Equal(t, d.state, lowUtilIdle)
	assert.Equal(t, d.node, "b")
	assert.Equal(t, d.observed, 2.0)

	allBusy := gpuWindow("u", 80, 60, n("a", 80, 60), n("b", 60, 60))
	d = decideLowUtil(mode, rule, &allBusy)
	assert.Equal(t, d.state, lowUtilActive)
	assert.Equal(t, d.observed, 60.0)

	// One SaFE virtual node spread over several hosts: Robust reports each host key.
	spread := gpuWindow("u", 80, 60, n("host-1", 80, 60), n("host-2", 70, 60), n("host-3", 1, 60))
	d = decideLowUtil(mode, rule, &spread)
	assert.Equal(t, d.state, lowUtilIdle)
	assert.Equal(t, d.node, "host-3")

	// A host without an exporter is absent and cannot make the workload active.
	missingHost := gpuWindow("u", 80, 60, n("host-2", 2, 60))
	assert.Equal(t, decideLowUtil(mode, rule, &missingHost).state, lowUtilIdle)
	missingHostBusy := gpuWindow("u", 80, 60, n("host-1", 80, 60))
	assert.Equal(t, decideLowUtil(mode, rule, &missingHostBusy).state, lowUtilActive)

	// Nodes below the coverage threshold are ignored, whatever their value.
	sparseIdle := gpuWindow("u", 80, 60, n("a", 80, 60), n("b", 2, 40))
	d = decideLowUtil(mode, rule, &sparseIdle)
	assert.Equal(t, d.state, lowUtilActive)
	assert.Equal(t, d.observed, 80.0)
	sparseBusy := gpuWindow("u", 80, 60, n("a", 80, 40), n("b", 2, 60))
	assert.Equal(t, decideLowUtil(mode, rule, &sparseBusy).node, "b")

	// No covered node, or none reported at all.
	noneCovered := gpuWindow("u", 80, 60, n("a", 2, 40), n("b", 2, 10))
	assert.Equal(t, decideLowUtil(mode, rule, &noneCovered).state, lowUtilUnknown)
	noNodes := gpuWindow("u", 80, 60)
	assert.Equal(t, decideLowUtil(mode, rule, &noNodes).state, lowUtilUnknown)

	sparseOverall := gpuWindow("u", 80, 30, n("a", 80, 60), n("b", 2, 60))
	assert.Equal(t, decideLowUtil(mode, rule, &sparseOverall).state, lowUtilUnknown)
}

func TestLowUtilFirstDetectionMarksAndNotifies(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, rec := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w)

	r.runOnce(context.Background())

	assert.Equal(t, len(src.calls), 1)
	assert.Equal(t, src.calls[0], lowUtilQuery{cluster: "c1", window: "1h"})
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), lowUtilTestNow.Format(time.RFC3339))
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilEnforcedAtAnnotation), lowUtilTestNow.Format(time.RFC3339))
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilNotifiedAtAnnotation), lowUtilTestNow.Format(time.RFC3339))
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
	assert.Equal(t, len(rec.Events), 1)
	event := <-rec.Events
	assert.Assert(t, strings.Contains(event, lowUtilEventReason), event)
	assert.Assert(t, strings.Contains(event, lowUtilTestNow.Add(30*time.Minute).Format(time.RFC3339)), event)
}

// lowUtilEnforced marks w as detected and its stop countdown as started ago.
func lowUtilEnforced(w *v1.Workload, ago time.Duration) {
	at := lowUtilTestNow.Add(-ago).Format(time.RFC3339)
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, at)
	v1.SetAnnotation(w, v1.WorkloadLowUtilEnforcedAtAnnotation, at)
	v1.SetAnnotation(w, v1.WorkloadLowUtilNotifiedAtAnnotation, at)
}

func TestLowUtilStopsAfterNotifyDelay(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	lowUtilEnforced(w, 31*time.Minute)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, rec := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w)

	r.runOnce(context.Background())

	_, err := getLowUtilWorkload(t, cl, "w")
	assert.Assert(t, apierrors.IsNotFound(err), err)
	assert.Equal(t, len(rec.Events), 0)
}

func TestLowUtilWaitsForNotifyDelay(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	detectedAt := lowUtilTestNow.Add(-10 * time.Minute).Format(time.RFC3339)
	lowUtilEnforced(w, 10*time.Minute)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, rec := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w)

	r.runOnce(context.Background())

	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), detectedAt)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilNotifiedAtAnnotation), detectedAt)
	assert.Equal(t, len(rec.Events), 0)
}

// A workload detected under a dry-run policy gets a full countdown, and a notice, once the policy
// is enforced, instead of being stopped at once for its old detection.
func TestLowUtilDryRunToEnforcementStartsFreshCountdown(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.DryRun = true
	w := lowUtilTestWorkload("w", "a")
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), w)
	r.now = func() time.Time { return lowUtilTestNow.Add(-time.Hour) }
	r.runOnce(context.Background())
	r.now = func() time.Time { return lowUtilTestNow.Add(-20 * time.Minute) }
	r.runOnce(context.Background())
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilEnforcedAtAnnotation), "")
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilNotifiedAtAnnotation), "")

	ws := &v1.Workspace{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "ws"}, ws))
	policy.DryRun = false
	ws.Spec.LowUtilization[v1.TrainScope] = policy
	assert.NilError(t, cl.Update(context.Background(), ws))
	r.now = func() time.Time { return lowUtilTestNow }
	r.runOnce(context.Background())

	got, err = getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
	now := lowUtilTestNow.Format(time.RFC3339)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilEnforcedAtAnnotation), now)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilNotifiedAtAnnotation), now)
	timeline := lowUtilTimeline(t, cl, "w")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{"Detected_1", "DryRunWouldStop_1", "Enforced_1"})
	assert.Equal(t, timeline[2].Message, "the low utilization policy is enforced now; the workload will be stopped at "+
		lowUtilTestNow.Add(30*time.Minute).Format(time.RFC3339)+" if it stays idle")

	r.now = func() time.Time { return lowUtilTestNow.Add(31 * time.Minute) }
	r.runOnce(context.Background())
	_, err = getLowUtilWorkload(t, cl, "w")
	assert.Assert(t, apierrors.IsNotFound(err), err)
}

// Back to dry run, the countdown is dropped, so enforcing again starts a new one.
func TestLowUtilEnforcementToDryRunDropsCountdown(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.DryRun = true
	w := lowUtilTestWorkload("w", "a")
	lowUtilEnforced(w, 2*time.Hour)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), w)

	r.runOnce(context.Background())

	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilEnforcedAtAnnotation), "")
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation),
		lowUtilTestNow.Add(-2*time.Hour).Format(time.RFC3339))
}

func TestLowUtilStopsInTheSameRoundWithoutNotifyDelay(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.NotifyBefore = ""
	w := lowUtilTestWorkload("w", "a")
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), w)

	r.runOnce(context.Background())

	_, err := getLowUtilWorkload(t, cl, "w")
	assert.Assert(t, apierrors.IsNotFound(err), err)
}

func TestLowUtilMinModeStopsOnIdleNode(t *testing.T) {
	w := lowUtilTestWorkload("w", "a", "b")
	lowUtilEnforced(w, time.Hour)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{
		"w-uid": gpuWindow("w-uid", 90, 60,
			robustclient.GPUWindowNode{Node: "a", Max: 90, Samples: 60},
			robustclient.GPUWindowNode{Node: "b", Max: 0, Samples: 60}),
	}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationMin)), w)

	r.runOnce(context.Background())

	_, err := getLowUtilWorkload(t, cl, "w")
	assert.Assert(t, apierrors.IsNotFound(err), err)
}

func TestLowUtilDryRunKeepsWorkload(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.DryRun = true
	w := lowUtilTestWorkload("w", "a")
	detectedAt := lowUtilTestNow.Add(-2 * time.Hour).Format(time.RFC3339)
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, detectedAt)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), w)

	r.runOnce(context.Background())

	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), detectedAt)
}

func TestLowUtilActiveResetsDetection(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	lowUtilEnforced(w, time.Hour)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 50, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w)

	// Active for less than the cooldown, by default the 1h window: the detection is kept.
	r.runOnce(context.Background())
	r.now = func() time.Time { return lowUtilTestNow.Add(59 * time.Minute) }
	r.runOnce(context.Background())
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilActiveSinceAnnotation), lowUtilTestNow.Format(time.RFC3339))
	assert.Assert(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation) != "")

	r.now = func() time.Time { return lowUtilTestNow.Add(time.Hour) }
	r.runOnce(context.Background())
	got, err = getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	for _, key := range []string{v1.WorkloadLowUtilDetectedAtAnnotation, v1.WorkloadLowUtilEnforcedAtAnnotation,
		v1.WorkloadLowUtilActiveSinceAnnotation} {
		_, marked := got.Annotations[key]
		assert.Assert(t, !marked, key)
	}
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilNotifiedAtAnnotation),
		lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339))
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
}

// A workload hovering around the threshold keeps one detection, one notice and one countdown.
func TestLowUtilOscillationKeepsOneCountdown(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.Threshold = 10
	src := &fakeGPUWindow{}
	r, cl, rec := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), lowUtilTestWorkload("w", "a"))

	for i, util := range []float64{8, 12, 8, 12} {
		at := lowUtilTestNow.Add(time.Duration(i) * 10 * time.Minute)
		r.now = func() time.Time { return at }
		src.windows = map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", util, 60)}
		r.runOnce(context.Background())
	}
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	first := lowUtilTestNow.Format(time.RFC3339)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), first)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilEnforcedAtAnnotation), first)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilNotifiedAtAnnotation), first)
	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "w")), []string{"Detected_1"})
	assert.Equal(t, len(rec.Events), 1)

	r.now = func() time.Time { return lowUtilTestNow.Add(40 * time.Minute) }
	src.windows = map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 8, 60)}
	r.runOnce(context.Background())
	_, err = getLowUtilWorkload(t, cl, "w")
	assert.Assert(t, apierrors.IsNotFound(err), err)
}

func TestLowUtilNoticeRateLimited(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	notifiedAt := lowUtilTestNow.Add(-50 * time.Minute).Format(time.RFC3339)
	v1.SetAnnotation(w, v1.WorkloadLowUtilNotifiedAtAnnotation, notifiedAt)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w)

	r.runOnce(context.Background())

	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilEnforcedAtAnnotation), lowUtilTestNow.Format(time.RFC3339))
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilNotifiedAtAnnotation), notifiedAt)

	// Once the cooldown has passed since the last notice, the owner is told again.
	w2 := lowUtilTestWorkload("w2", "a")
	v1.SetAnnotation(w2, v1.WorkloadLowUtilNotifiedAtAnnotation, lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339))
	src = &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w2-uid": gpuWindow("w2-uid", 1, 60)}}
	r, cl, _ = newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w2)
	r.runOnce(context.Background())
	got, err = getLowUtilWorkload(t, cl, "w2")
	assert.NilError(t, err)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilNotifiedAtAnnotation), lowUtilTestNow.Format(time.RFC3339))
}

func TestLowUtilCooldown(t *testing.T) {
	assert.Equal(t, lowUtilCooldown(&v1.LowUtilizationPolicy{Threshold: 5, Window: "1h"}), time.Hour)
	rules := lowUtilRulesPolicy()
	assert.Equal(t, lowUtilCooldown(&rules), 30*time.Minute)
	rules.Cooldown = "2h"
	assert.Equal(t, lowUtilCooldown(&rules), 2*time.Hour)
	rules.Cooldown = "0s"
	assert.Equal(t, lowUtilCooldown(&rules), time.Duration(0))
}

func TestLowUtilUnknownKeepsState(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	detectedAt := lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339)
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, detectedAt)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 20)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w)

	r.runOnce(context.Background())

	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), detectedAt)
}

func TestLowUtilMinModeStopsSpreadNodeWithAHostMissing(t *testing.T) {
	w := lowUtilTestWorkload("w", "vk-spur-1")
	lowUtilEnforced(w, time.Hour)
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{
		"w-uid": gpuWindow("w-uid", 90, 60,
			robustclient.GPUWindowNode{Node: "host-1", Max: 90, Samples: 60},
			robustclient.GPUWindowNode{Node: "host-2", Max: 0, Samples: 60}),
	}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationMin)), w)

	r.runOnce(context.Background())

	_, err := getLowUtilWorkload(t, cl, "w")
	assert.Assert(t, apierrors.IsNotFound(err), err)
}

func TestLowUtilMinModeUnknownWithoutCoveredNodes(t *testing.T) {
	w := lowUtilTestWorkload("w", "a", "b")
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339))
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{
		"w-uid": gpuWindow("w-uid", 1, 60, robustclient.GPUWindowNode{Node: "b", Max: 0, Samples: 20}),
	}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationMin)), w)

	r.runOnce(context.Background())

	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
}

func TestLowUtilGracePeriodSkipsYoungWorkloads(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.GracePeriod = "3h"
	w := lowUtilTestWorkload("w", "a")
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), w)

	r.runOnce(context.Background())

	assert.Equal(t, len(src.calls), 0)
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), "")
}

func TestLowUtilSkipsExemptAndUnmatchedWorkloads(t *testing.T) {
	exempt := lowUtilTestWorkload("exempt", "a")
	v1.SetAnnotation(exempt, v1.WorkloadLowUtilExemptAnnotation, v1.TrueStr)
	v1.SetAnnotation(exempt, v1.WorkloadLowUtilDetectedAtAnnotation, lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339))
	infer := lowUtilTestWorkload("infer", "a")
	infer.Spec.GroupVersionKind.Kind = common.DeploymentKind
	pending := lowUtilTestWorkload("pending", "a")
	pending.Status.Phase = v1.WorkloadPending
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{
		"exempt-uid": gpuWindow("exempt-uid", 1, 60),
		"infer-uid":  gpuWindow("infer-uid", 1, 60),
	}}
	r, cl, _ := newLowUtilTestController(t, src,
		lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), exempt, infer, pending)

	r.runOnce(context.Background())

	assert.Equal(t, len(src.calls), 0)
	got, err := getLowUtilWorkload(t, cl, "exempt")
	assert.NilError(t, err)
	_, marked := got.Annotations[v1.WorkloadLowUtilDetectedAtAnnotation]
	assert.Assert(t, !marked)
}

// lowUtilExemptUser is a user exempt from the low utilization policy in the given workspaces.
func lowUtilExemptUser(name string, workspaces ...string) *v1.User {
	u := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: name}}
	u.Spec.LowUtilization = &v1.UserLowUtilization{Workspaces: map[string]v1.UserLowUtilizationScope{}}
	for _, ws := range workspaces {
		u.Spec.LowUtilization.Workspaces[ws] = v1.UserLowUtilizationScope{Exempt: true}
	}
	return u
}

func lowUtilOwnedWorkload(name, owner, workspace string) *v1.Workload {
	w := lowUtilTestWorkload(name, "a")
	w.Labels[v1.UserIdLabel] = owner
	w.Spec.Workspace = workspace
	return w
}

func TestLowUtilSkipsUserExemptOwnerOnlyInThatWorkspace(t *testing.T) {
	ws2 := lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage))
	ws2.Name = "ws2"
	notExempt := lowUtilExemptUser("bob")
	notExempt.Spec.LowUtilization.Workspaces["ws"] = v1.UserLowUtilizationScope{}
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{
		"exempt-uid":     gpuWindow("exempt-uid", 1, 60),
		"other-ws-uid":   gpuWindow("other-ws-uid", 1, 60),
		"other-user-uid": gpuWindow("other-user-uid", 1, 60),
	}}
	r, cl, _ := newLowUtilTestController(t, src,
		lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), ws2,
		lowUtilExemptUser("alice", "ws"), notExempt,
		lowUtilOwnedWorkload("exempt", "alice", "ws"),
		lowUtilOwnedWorkload("other-ws", "alice", "ws2"),
		lowUtilOwnedWorkload("other-user", "bob", "ws"))

	r.runOnce(context.Background())

	for name, detected := range map[string]bool{"exempt": false, "other-ws": true, "other-user": true} {
		got, err := getLowUtilWorkload(t, cl, name)
		assert.NilError(t, err)
		_, marked := got.Annotations[v1.WorkloadLowUtilDetectedAtAnnotation]
		assert.Equal(t, marked, detected, name)
	}
	assert.Equal(t, len(lowUtilTimeline(t, cl, "exempt")), 0)
}

func TestLowUtilUserExemptionClosesOpenEpisode(t *testing.T) {
	open := lowUtilOwnedWorkload("open", "alice", "ws")
	lowUtilEnforced(open, 10*time.Minute)
	open.Status.Conditions = []metav1.Condition{lowUtilDetectedCondition()}
	quiet := lowUtilOwnedWorkload("quiet", "alice", "ws")
	recovered := lowUtilOwnedWorkload("recovered", "alice", "ws")
	lowUtilEnforced(recovered, 10*time.Minute)
	recovered.Status.Conditions = []metav1.Condition{lowUtilDetectedCondition(), {
		Type: string(v1.AdminLowUtilization), Status: metav1.ConditionTrue, Reason: "Recovered_1",
		Message: "recovered", LastTransitionTime: metav1.NewTime(lowUtilTestNow.Add(-time.Hour)),
	}}
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"open-uid": gpuWindow("open-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)),
		lowUtilExemptUser("alice", "ws"), open, quiet, recovered)

	r.runOnce(context.Background())
	r.runOnce(context.Background())

	assert.Equal(t, len(src.calls), 0)
	timeline := lowUtilTimeline(t, cl, "open")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{"Detected_1", "Exempted_1"})
	assert.Equal(t, timeline[1].Message, "the owner alice is exempt from the low utilization policy in workspace ws; "+
		"the workload is no longer checked")
	got, err := getLowUtilWorkload(t, cl, "open")
	assert.NilError(t, err)
	for _, key := range []string{v1.WorkloadLowUtilDetectedAtAnnotation, v1.WorkloadLowUtilEnforcedAtAnnotation,
		v1.WorkloadLowUtilActiveSinceAnnotation} {
		_, marked := got.Annotations[key]
		assert.Assert(t, !marked, key)
	}
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)

	assert.Equal(t, len(lowUtilTimeline(t, cl, "quiet")), 0)
	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "recovered")), []string{"Detected_1", "Recovered_1"})
}

func TestLowUtilAnnotationAndUserExemptionBothSkip(t *testing.T) {
	annotated := lowUtilOwnedWorkload("annotated", "bob", "ws")
	v1.SetAnnotation(annotated, v1.WorkloadLowUtilExemptAnnotation, v1.TrueStr)
	both := lowUtilOwnedWorkload("both", "alice", "ws")
	v1.SetAnnotation(both, v1.WorkloadLowUtilExemptAnnotation, v1.TrueStr)
	owner := lowUtilOwnedWorkload("owner", "alice", "ws")
	for _, w := range []*v1.Workload{annotated, both, owner} {
		lowUtilEnforced(w, 10*time.Minute)
		w.Status.Conditions = []metav1.Condition{lowUtilDetectedCondition()}
	}
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{
		"annotated-uid": gpuWindow("annotated-uid", 1, 60),
		"both-uid":      gpuWindow("both-uid", 1, 60),
		"owner-uid":     gpuWindow("owner-uid", 1, 60),
	}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)),
		lowUtilExemptUser("alice", "ws"), annotated, both, owner)

	r.runOnce(context.Background())

	assert.Equal(t, len(src.calls), 0)
	for _, name := range []string{"annotated", "both", "owner"} {
		got, err := getLowUtilWorkload(t, cl, name)
		assert.NilError(t, err)
		_, marked := got.Annotations[v1.WorkloadLowUtilDetectedAtAnnotation]
		assert.Assert(t, !marked, name)
	}
	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "annotated")), []string{"Detected_1"})
	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "both")), []string{"Detected_1"})
	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "owner")), []string{"Detected_1", "Exempted_1"})
}

func TestLowUtilUserListErrorKeepsPolicy(t *testing.T) {
	w := lowUtilOwnedWorkload("w", "alice", "ws")
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), lowUtilExemptUser("alice", "ws"), w).
		WithStatusSubresource(&v1.Workload{}).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c ctrlclient.WithWatch, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
				if _, ok := list.(*v1.UserList); ok {
					return errors.New("users unavailable")
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r := &LowUtilizationController{
		Client: cl, source: src, recorder: record.NewFakeRecorder(10),
		now: func() time.Time { return lowUtilTestNow },
	}

	r.runOnce(context.Background())

	assert.Equal(t, len(src.calls), 1)
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), lowUtilTestNow.Format(time.RFC3339))
}

func TestLowUtilSkipsUnsupportedCluster(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339))
	src := &fakeGPUWindow{err: fmt.Errorf("%w: 404", robustclient.ErrNotSupported)}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w)

	r.runOnce(context.Background())

	assert.Equal(t, len(src.calls), 1)
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
}

func TestLowUtilGroupsRequestsByWindow(t *testing.T) {
	ws := lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage))
	inferPolicy := lowUtilPolicy(v1.LowUtilizationAverage)
	inferPolicy.Window = "2h"
	ws.Spec.LowUtilization[v1.InferScope] = inferPolicy
	train := lowUtilTestWorkload("train", "a")
	infer := lowUtilTestWorkload("infer", "a")
	infer.Spec.GroupVersionKind.Kind = common.DeploymentKind
	src := &fakeGPUWindow{}
	r, _, _ := newLowUtilTestController(t, src, ws, train, infer)

	r.runOnce(context.Background())

	assert.Equal(t, len(src.calls), 2)
	windows := map[string]bool{}
	for _, c := range src.calls {
		windows[c.window] = true
	}
	assert.DeepEqual(t, windows, map[string]bool{"1h": true, "2h": true})
}

func TestRobustGPUWindowSourceWithoutClient(t *testing.T) {
	src := robustGPUWindowSource{rc: robustclient.NewClient(robustclient.ClientConfig{})}
	_, err := src.GetWorkloadGPUWindow(context.Background(), "missing", []string{"u"}, "1h", lowUtilStep)
	assert.Assert(t, errors.Is(err, errLowUtilNoClient), err)
}

func TestParseLowUtilDuration(t *testing.T) {
	assert.Equal(t, parseLowUtilDuration(""), time.Duration(0))
	assert.Equal(t, parseLowUtilDuration("bad"), time.Duration(0))
	assert.Equal(t, parseLowUtilDuration("-5m"), time.Duration(0))
	assert.Equal(t, parseLowUtilDuration("1h30m"), 90*time.Minute)
}

var conditionReasonPattern = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`)

func lowUtilTimeline(t *testing.T, cl ctrlclient.Client, name string) []metav1.Condition {
	t.Helper()
	w, err := getLowUtilWorkload(t, cl, name)
	assert.NilError(t, err)
	var out []metav1.Condition
	for _, c := range w.Status.Conditions {
		if c.Type == string(v1.AdminLowUtilization) {
			assert.Assert(t, conditionReasonPattern.MatchString(c.Reason), c.Reason)
			assert.Equal(t, c.Status, metav1.ConditionTrue)
			out = append(out, c)
		}
	}
	return out
}

func lowUtilReasons(conds []metav1.Condition) []string {
	reasons := make([]string, 0, len(conds))
	for _, c := range conds {
		reasons = append(reasons, c.Reason)
	}
	return reasons
}

func lowUtilDetectedCondition() metav1.Condition {
	return metav1.Condition{Type: string(v1.AdminLowUtilization), Status: metav1.ConditionTrue,
		Reason: "Detected_1", Message: "detected", LastTransitionTime: metav1.NewTime(lowUtilTestNow.Add(-2 * time.Hour))}
}

func TestLowUtilDryRunDetectionAddsTimelineEntry(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.DryRun = true
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), lowUtilTestWorkload("w", "a"))

	r.runOnce(context.Background())

	timeline := lowUtilTimeline(t, cl, "w")
	assert.Equal(t, len(timeline), 1)
	assert.Equal(t, timeline[0].Reason, "Detected_1")
	assert.Assert(t, strings.HasPrefix(timeline[0].Message, "low GPU utilization: "), timeline[0].Message)
	assert.Assert(t, strings.HasSuffix(timeline[0].Message, "; dry run, the workload will not be stopped"), timeline[0].Message)
	assert.Assert(t, timeline[0].LastTransitionTime.Time.Equal(lowUtilTestNow))
}

func TestLowUtilDetectionAddsTimelineEntryWithStopTime(t *testing.T) {
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src,
		lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), lowUtilTestWorkload("w", "a"))

	r.runOnce(context.Background())
	r.runOnce(context.Background())

	timeline := lowUtilTimeline(t, cl, "w")
	assert.Equal(t, len(timeline), 1)
	assert.Equal(t, timeline[0].Reason, "Detected_1")
	want := "; the workload will be stopped at " + lowUtilTestNow.Add(30*time.Minute).Format(time.RFC3339) + " if it stays idle"
	assert.Assert(t, strings.HasSuffix(timeline[0].Message, want), timeline[0].Message)
}

func TestLowUtilDryRunWouldStopAddedOncePerEpisode(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.DryRun = true
	w := lowUtilTestWorkload("w", "a")
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, lowUtilTestNow.Add(-2*time.Hour).Format(time.RFC3339))
	w.Status.Conditions = []metav1.Condition{lowUtilDetectedCondition()}
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), w)

	r.runOnce(context.Background())
	r.runOnce(context.Background())

	timeline := lowUtilTimeline(t, cl, "w")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{"Detected_1", "DryRunWouldStop_1"})
	assert.Assert(t, strings.HasPrefix(timeline[1].Message, "dry run: the workload would be stopped now (low GPU utilization: "),
		timeline[1].Message)
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
}

func TestLowUtilRecoveryAddsTimelineEntry(t *testing.T) {
	marked := lowUtilTestWorkload("marked", "a")
	v1.SetAnnotation(marked, v1.WorkloadLowUtilDetectedAtAnnotation, lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339))
	v1.SetAnnotation(marked, v1.WorkloadLowUtilActiveSinceAnnotation, lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339))
	marked.Status.Conditions = []metav1.Condition{lowUtilDetectedCondition()}
	unmarked := lowUtilTestWorkload("unmarked", "a")
	unmarked.Status.Conditions = []metav1.Condition{lowUtilDetectedCondition()}
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{
		"marked-uid":   gpuWindow("marked-uid", 50, 60),
		"unmarked-uid": gpuWindow("unmarked-uid", 50, 60),
	}}
	r, cl, _ := newLowUtilTestController(t, src,
		lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), marked, unmarked)

	r.runOnce(context.Background())

	timeline := lowUtilTimeline(t, cl, "marked")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{"Detected_1", "Recovered_1"})
	assert.Equal(t, timeline[1].Message, "GPU utilization recovered: observed max 50.0%")
	got, err := getLowUtilWorkload(t, cl, "marked")
	assert.NilError(t, err)
	_, stillMarked := got.Annotations[v1.WorkloadLowUtilDetectedAtAnnotation]
	assert.Assert(t, !stillMarked)

	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "unmarked")), []string{"Detected_1"})
}

// Each idle episode adds its own entries: detect, recover, detect again.
func TestLowUtilEpisodesAddNewTimelineEntries(t *testing.T) {
	policy := lowUtilPolicy(v1.LowUtilizationAverage)
	policy.DryRun = true
	policy.NotifyBefore = ""
	policy.Cooldown = "0s"
	idle := map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}
	active := map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 50, 60)}
	src := &fakeGPUWindow{windows: idle}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), lowUtilTestWorkload("w", "a"))

	r.runOnce(context.Background())
	src.windows = active
	r.runOnce(context.Background())
	r.runOnce(context.Background())
	src.windows = idle
	r.runOnce(context.Background())
	r.runOnce(context.Background())

	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "w")), []string{
		"Detected_1", "DryRunWouldStop_1", "Recovered_1", "Detected_2", "DryRunWouldStop_2"})
}

// With the first episode trimmed from the history, the next one still gets a new index.
func TestLowUtilEpisodeSurvivesTrimmedHistory(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	detected := lowUtilDetectedCondition()
	detected.Reason = "Detected_2"
	recovered := detected
	recovered.Reason = "Recovered_2"
	w.Status.Conditions = []metav1.Condition{detected, recovered}
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), w)

	r.runOnce(context.Background())

	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "w")), []string{"Detected_2", "Recovered_2", "Detected_3"})
}

// lowUtilRulesPolicy is the 11%/30m or 30%/1h policy.
func lowUtilRulesPolicy() v1.LowUtilizationPolicy {
	return v1.LowUtilizationPolicy{
		Rules:        []v1.LowUtilizationRule{{Threshold: 11, Window: "30m"}, {Threshold: 30, Window: "1h"}},
		Mode:         v1.LowUtilizationAverage,
		NotifyBefore: "30m",
	}
}

func lowUtilCalls(src *fakeGPUWindow) []string {
	calls := make([]string, 0, len(src.calls))
	for _, q := range src.calls {
		calls = append(calls, q.cluster+"/"+q.window)
	}
	return calls
}

func lowUtilRulesSource(max30m float64, samples30m int, max1h float64, samples1h int) *fakeGPUWindow {
	return &fakeGPUWindow{byWindow: map[string]map[string]robustclient.WorkloadGPUWindow{
		"30m": {"w-uid": gpuWindow("w-uid", max30m, samples30m)},
		"1h":  {"w-uid": gpuWindow("w-uid", max1h, samples1h)},
	}}
}

// lowUtilNoRuleIndex checks that no entry names a rule by its position.
func lowUtilNoRuleIndex(t *testing.T, conds []metav1.Condition) {
	t.Helper()
	for _, c := range conds {
		assert.Assert(t, !strings.Contains(c.Message, "rule "), c.Message)
	}
}

func TestLowUtilRulesFirstRuleMatches(t *testing.T) {
	src := lowUtilRulesSource(3, 60, 40, 60)
	r, cl, rec := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilRulesPolicy()), lowUtilTestWorkload("w", "a"))

	r.runOnce(context.Background())

	assert.DeepEqual(t, lowUtilCalls(src), []string{"c1/30m", "c1/1h"})
	timeline := lowUtilTimeline(t, cl, "w")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{"Detected_1_lt11_30m"})
	assert.Equal(t, timeline[0].Message, fmt.Sprintf(
		"low GPU utilization below 11%% for 30m: mode %s, observed max 3.0%%; the workload will be stopped at %s if it stays idle",
		v1.LowUtilizationAverage, lowUtilTestNow.Add(30*time.Minute).Format(time.RFC3339)))
	event := <-rec.Events
	assert.Assert(t, strings.Contains(event, "below 11% for 30m") && !strings.Contains(event, "rule "), event)
}

func TestLowUtilRulesSecondRuleMatches(t *testing.T) {
	src := lowUtilRulesSource(20, 60, 25, 60)
	r, cl, rec := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilRulesPolicy()), lowUtilTestWorkload("w", "a"))

	r.runOnce(context.Background())

	timeline := lowUtilTimeline(t, cl, "w")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{"Detected_1_lt30_1h"})
	assert.Assert(t, strings.HasPrefix(timeline[0].Message, "low GPU utilization below 30% for 1h: mode Average, observed max 25.0%; "),
		timeline[0].Message)
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), lowUtilTestNow.Format(time.RFC3339))
	assert.Equal(t, len(rec.Events), 1)
}

// The 30m rule goes idle first and the 1h rule rounds later: each gets one entry in the episode.
func TestLowUtilRulesEachRuleAddsOneEntryPerEpisode(t *testing.T) {
	src := lowUtilRulesSource(3, 60, 40, 60)
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilRulesPolicy()), lowUtilTestWorkload("w", "a"))

	for i := 0; i < 3; i++ {
		r.runOnce(context.Background())
	}
	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "w")), []string{"Detected_1_lt11_30m"})
	src.byWindow["1h"] = map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 25, 60)}
	for i := 0; i < 3; i++ {
		r.runOnce(context.Background())
	}

	timeline := lowUtilTimeline(t, cl, "w")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{"Detected_1_lt11_30m", "Detected_1_lt30_1h"})
	assert.Assert(t, strings.HasPrefix(timeline[1].Message, "low GPU utilization below 30% for 1h: "), timeline[1].Message)
	lowUtilNoRuleIndex(t, timeline)
}

func TestLowUtilRulesBothMatchInOneRound(t *testing.T) {
	policy := lowUtilRulesPolicy()
	policy.DryRun = true
	src := lowUtilRulesSource(3, 60, 25, 60)
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), lowUtilTestWorkload("w", "a"))

	r.runOnce(context.Background())
	r.runOnce(context.Background())

	timeline := lowUtilTimeline(t, cl, "w")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{"Detected_1_lt11_30m", "Detected_1_lt30_1h"})
	for _, c := range timeline {
		assert.Assert(t, strings.HasSuffix(c.Message, "; dry run, the workload will not be stopped"), c.Message)
	}
	lowUtilNoRuleIndex(t, timeline)
}

func TestLowUtilRulesNewEpisodeAfterRecovery(t *testing.T) {
	policy := lowUtilRulesPolicy()
	policy.DryRun = true
	policy.NotifyBefore = ""
	policy.Cooldown = "0s"
	src := lowUtilRulesSource(3, 60, 40, 60)
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(policy), lowUtilTestWorkload("w", "a"))

	r.runOnce(context.Background())
	r.runOnce(context.Background())
	src.byWindow = lowUtilRulesSource(20, 60, 40, 60).byWindow
	r.runOnce(context.Background())
	src.byWindow = lowUtilRulesSource(20, 60, 25, 60).byWindow
	r.runOnce(context.Background())
	r.runOnce(context.Background())

	timeline := lowUtilTimeline(t, cl, "w")
	assert.DeepEqual(t, lowUtilReasons(timeline), []string{
		"Detected_1_lt11_30m", "DryRunWouldStop_1", "Recovered_1", "Detected_2_lt30_1h", "DryRunWouldStop_2"})
	lowUtilNoRuleIndex(t, timeline)
}

func TestLowUtilEpisodeParsesLegacyAndRuleReasons(t *testing.T) {
	cond := func(reason string) metav1.Condition {
		c := lowUtilDetectedCondition()
		c.Reason = reason
		return c
	}
	w := lowUtilTestWorkload("w", "a")
	w.Status.Conditions = []metav1.Condition{cond("Detected_1"), cond("Recovered_1"), cond("Detected_2_lt11_30m"),
		cond("Detected_x_lt5_1h"), cond("Detected_9abc")}
	episode, open := lowUtilEpisode(w)
	assert.Equal(t, episode, 2)
	assert.Assert(t, open)

	w.Status.Conditions = append(w.Status.Conditions, cond("Recovered_2"), cond("Detected_3"))
	episode, open = lowUtilEpisode(w)
	assert.Equal(t, episode, 3)
	assert.Assert(t, open)
}

func TestLowUtilRuleReason(t *testing.T) {
	for rule, want := range map[v1.LowUtilizationRule]string{
		{Threshold: 11, Window: "30m"}:   "Detected_1_lt11_30m",
		{Threshold: 30, Window: "1h"}:    "Detected_1_lt30_1h",
		{Threshold: 0, Window: "1h30m"}:  "Detected_1_lt0_1h30m",
		{Threshold: 5, Window: "1.5h"}:   "Detected_1_lt5_1_5h",
		{Threshold: 5, Window: "90m:x,"}: "Detected_1_lt5_90m_x_",
	} {
		got := lowUtilRuleReason(1, rule)
		assert.Equal(t, got, want)
		assert.Assert(t, conditionReasonPattern.MatchString(got), got)
	}
}

func TestLowUtilRulesNoneMatchRecovers(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339))
	// Active since the shortest rule window, the default cooldown.
	v1.SetAnnotation(w, v1.WorkloadLowUtilActiveSinceAnnotation, lowUtilTestNow.Add(-30*time.Minute).Format(time.RFC3339))
	w.Status.Conditions = []metav1.Condition{lowUtilDetectedCondition()}
	src := lowUtilRulesSource(20, 60, 40, 60)
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilRulesPolicy()), w)

	r.runOnce(context.Background())

	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "w")), []string{"Detected_1", "Recovered_1"})
	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	_, marked := got.Annotations[v1.WorkloadLowUtilDetectedAtAnnotation]
	assert.Assert(t, !marked)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
}

func TestLowUtilRulesUnknownAndActiveIsUnknown(t *testing.T) {
	w := lowUtilTestWorkload("w", "a")
	detectedAt := lowUtilTestNow.Add(-time.Hour).Format(time.RFC3339)
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, detectedAt)
	w.Status.Conditions = []metav1.Condition{lowUtilDetectedCondition()}
	src := lowUtilRulesSource(20, 60, 1, 20)
	r, cl, _ := newLowUtilTestController(t, src, lowUtilTestWorkspace(lowUtilRulesPolicy()), w)

	r.runOnce(context.Background())

	got, err := getLowUtilWorkload(t, cl, "w")
	assert.NilError(t, err)
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
	assert.Equal(t, v1.GetAnnotation(got, v1.WorkloadLowUtilDetectedAtAnnotation), detectedAt)
	assert.DeepEqual(t, lowUtilReasons(lowUtilTimeline(t, cl, "w")), []string{"Detected_1"})
}

func TestLowUtilLegacyPolicyDescription(t *testing.T) {
	src := &fakeGPUWindow{windows: map[string]robustclient.WorkloadGPUWindow{"w-uid": gpuWindow("w-uid", 1, 60)}}
	r, cl, _ := newLowUtilTestController(t, src,
		lowUtilTestWorkspace(lowUtilPolicy(v1.LowUtilizationAverage)), lowUtilTestWorkload("w", "a"))

	r.runOnce(context.Background())

	assert.DeepEqual(t, lowUtilCalls(src), []string{"c1/1h"})
	timeline := lowUtilTimeline(t, cl, "w")
	assert.Equal(t, len(timeline), 1)
	assert.Assert(t, strings.HasPrefix(timeline[0].Message, fmt.Sprintf(
		"low GPU utilization: mode %s, threshold 5%%, window 1h, observed max 1.0%%; ", v1.LowUtilizationAverage)),
		timeline[0].Message)
}
