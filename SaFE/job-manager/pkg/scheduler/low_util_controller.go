/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jmmetrics "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/metrics"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

const (
	lowUtilInterval = time.Minute
	lowUtilStep     = "1m"
	// Share of the expected samples Robust must return before utilization is judged.
	lowUtilMinCoverage = 0.9
	lowUtilEventReason = "LowGPUUtilization"

	// AdminLowUtilization condition reasons, suffixed with the 1-based idle
	// episode so each episode adds its own timeline entries.
	lowUtilReasonDetected  = "Detected_"
	lowUtilReasonWouldStop = "DryRunWouldStop_"
	lowUtilReasonRecovered = "Recovered_"
	// A dry-run policy switched to enforcement during the episode.
	lowUtilReasonEnforced = "Enforced_"
)

var errLowUtilNoClient = errors.New("no Robust client for the cluster")

// gpuWindowSource returns the GPU utilization windows of workloads in one cluster.
type gpuWindowSource interface {
	GetWorkloadGPUWindow(ctx context.Context, cluster string,
		uids []string, window, step string) ([]robustclient.WorkloadGPUWindow, error)
}

type robustGPUWindowSource struct {
	rc *robustclient.Client
}

func (s robustGPUWindowSource) GetWorkloadGPUWindow(ctx context.Context, cluster string,
	uids []string, window, step string) ([]robustclient.WorkloadGPUWindow, error) {
	cc := s.rc.ForCluster(cluster)
	if cc == nil {
		return nil, errLowUtilNoClient
	}
	return cc.GetWorkloadGPUWindow(ctx, uids, window, step)
}

// LowUtilizationController stops running workloads whose GPU utilization stays
// below the LowUtilization policy of their workspace.
type LowUtilizationController struct {
	client.Client
	source   gpuWindowSource
	recorder record.EventRecorder
	now      func() time.Time
}

// SetupLowUtilizationController registers the controller when it is enabled in the config.
func SetupLowUtilizationController(mgr manager.Manager) error {
	if !commonconfig.IsWorkloadLowUtilizationEnable() {
		return nil
	}
	rc := robustclient.NewClient(robustclient.DefaultClientConfig())
	r := &LowUtilizationController{
		Client:   mgr.GetClient(),
		source:   robustGPUWindowSource{rc: rc},
		recorder: mgr.GetEventRecorderFor("low-utilization-controller"),
		now:      time.Now,
	}
	err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		robustclient.NewDiscovery(mgr.GetClient(), rc, 30*time.Second).Start(ctx)
		ticker := time.NewTicker(lowUtilInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				r.runOnce(ctx)
			}
		}
	}))
	if err != nil {
		return err
	}
	klog.Infof("Setup Workload Low Utilization Controller successfully")
	return nil
}

type lowUtilCandidate struct {
	workload *v1.Workload
	policy   *v1.LowUtilizationPolicy
	cluster  string
}

// lowUtilQuery groups the workloads that share one GPU window request.
type lowUtilQuery struct {
	cluster string
	window  string
}

type lowUtilState int

const (
	lowUtilUnknown lowUtilState = iota
	lowUtilActive
	lowUtilIdle
)

type lowUtilDecision struct {
	state    lowUtilState
	observed float64
	// The idle node in Min mode.
	node string
	// The rule the decision describes.
	rule v1.LowUtilizationRule
	// Every idle rule, set on an idle workload decision.
	idleRules []lowUtilDecision
}

func (r *LowUtilizationController) runOnce(ctx context.Context) {
	candidates, err := r.collectCandidates(ctx)
	if err != nil {
		klog.ErrorS(err, "[low-util] failed to collect workloads")
		return
	}
	now := r.now()
	checked := make([]lowUtilCandidate, 0, len(candidates))
	for _, c := range candidates {
		if len(c.policy.EffectiveRules()) == 0 {
			if err = r.clearDetected(ctx, c.workload); err != nil {
				klog.ErrorS(err, "[low-util] failed to clear the detected mark", "workload", c.workload.Name)
			}
			continue
		}
		if now.Before(c.workload.Status.StartTime.Add(parseLowUtilDuration(c.policy.GracePeriod))) {
			continue
		}
		checked = append(checked, c)
	}
	results := r.queryGPUWindows(ctx, checked)
	for _, c := range checked {
		d, complete := decideLowUtilRules(c, results)
		// Undecided for want of a query, not of samples: the state is kept and not counted as no data.
		if d.state == lowUtilUnknown && !complete {
			continue
		}
		if err = r.apply(ctx, c.cluster, c, d); err != nil {
			klog.ErrorS(err, "[low-util] failed to apply the decision", "workload", c.workload.Name)
		}
	}
}

// collectCandidates selects the started workloads under a policy. Workloads
// that left their policy lose the detected mark.
func (r *LowUtilizationController) collectCandidates(ctx context.Context) ([]lowUtilCandidate, error) {
	workspaces := &v1.WorkspaceList{}
	if err := r.List(ctx, workspaces); err != nil {
		return nil, err
	}
	byName := make(map[string]*v1.Workspace, len(workspaces.Items))
	for i := range workspaces.Items {
		byName[workspaces.Items[i].Name] = &workspaces.Items[i]
	}
	workloads := &v1.WorkloadList{}
	if err := r.List(ctx, workloads); err != nil {
		return nil, err
	}

	var candidates []lowUtilCandidate
	for i := range workloads.Items {
		w := &workloads.Items[i]
		if !w.IsRunning() || !w.GetDeletionTimestamp().IsZero() {
			continue
		}
		var policy *v1.LowUtilizationPolicy
		if ws := byName[w.Spec.Workspace]; ws != nil {
			policy = ws.GetLowUtilization(commonworkload.GetScope(w))
		}
		if policy == nil || v1.GetAnnotation(w, v1.WorkloadLowUtilExemptAnnotation) == v1.TrueStr {
			if err := r.clearDetected(ctx, w); err != nil {
				klog.ErrorS(err, "[low-util] failed to clear the detected mark", "workload", w.Name)
			}
			continue
		}
		if w.Status.StartTime == nil {
			continue
		}
		cluster := v1.GetClusterId(w)
		if cluster == "" {
			continue
		}
		candidates = append(candidates, lowUtilCandidate{workload: w, policy: policy, cluster: cluster})
	}
	return candidates, nil
}

// queryGPUWindows asks Robust once per cluster and distinct rule window for the
// candidates with a rule on that window. A query that failed has no entry.
func (r *LowUtilizationController) queryGPUWindows(ctx context.Context,
	candidates []lowUtilCandidate) map[lowUtilQuery]map[string]*robustclient.WorkloadGPUWindow {
	var queries []lowUtilQuery
	uids := make(map[lowUtilQuery][]string)
	for _, c := range candidates {
		for _, rule := range c.policy.EffectiveRules() {
			q := lowUtilQuery{cluster: c.cluster, window: rule.Window}
			if _, ok := uids[q]; !ok {
				queries = append(queries, q)
			}
			uids[q] = append(uids[q], string(c.workload.UID))
		}
	}
	results := make(map[lowUtilQuery]map[string]*robustclient.WorkloadGPUWindow, len(queries))
	for _, q := range queries {
		windows, err := r.source.GetWorkloadGPUWindow(ctx, q.cluster, uids[q], q.window, lowUtilStep)
		if err != nil {
			reason := "error"
			switch {
			case robustclient.IsNotSupported(err):
				reason = "unsupported"
			case errors.Is(err, errLowUtilNoClient):
				reason = "no_client"
			}
			jmmetrics.LowUtilClusterSkippedTotal.WithLabelValues(q.cluster, reason).Inc()
			klog.Warningf("[low-util] skip cluster %s (window %s, %d workloads): %v", q.cluster, q.window, len(uids[q]), err)
			continue
		}
		byUID := make(map[string]*robustclient.WorkloadGPUWindow, len(windows))
		for i := range windows {
			byUID[windows[i].UID] = &windows[i]
		}
		results[q] = byUID
	}
	return results
}

// decideLowUtilRules judges one workload against every rule of its policy: idle
// when any rule is idle (described by the one with the lowest observed value,
// the first on a tie), active when every rule is active, unknown otherwise.
// complete is false when the window of a rule could not be queried.
func decideLowUtilRules(c lowUtilCandidate,
	results map[lowUtilQuery]map[string]*robustclient.WorkloadGPUWindow) (lowUtilDecision, bool) {
	rules := c.policy.EffectiveRules()
	var idle []lowUtilDecision
	var active lowUtilDecision
	actives, complete := 0, true
	for _, rule := range rules {
		byUID, ok := results[lowUtilQuery{cluster: c.cluster, window: rule.Window}]
		if !ok {
			complete = false
			continue
		}
		d := decideLowUtil(c.policy.Mode, rule, byUID[string(c.workload.UID)])
		d.rule = rule
		switch d.state {
		case lowUtilIdle:
			idle = append(idle, d)
		case lowUtilActive:
			if actives == 0 || d.observed < active.observed {
				active = d
			}
			actives++
		}
	}
	switch {
	case len(idle) > 0:
		lowest := idle[0]
		for _, d := range idle[1:] {
			if d.observed < lowest.observed {
				lowest = d
			}
		}
		lowest.idleRules = idle
		return lowest, complete
	case actives == len(rules):
		return active, complete
	default:
		return lowUtilDecision{state: lowUtilUnknown}, complete
	}
}

// decideLowUtil judges one workload against one rule. Without enough samples,
// or in Min mode without any covered node, the state is unknown. Robust reports
// Min nodes per physical host and leaves out hosts without an exporter, so a
// missing node cannot make the workload active.
func decideLowUtil(mode v1.LowUtilizationMode, rule v1.LowUtilizationRule,
	w *robustclient.WorkloadGPUWindow) lowUtilDecision {
	unknown := lowUtilDecision{state: lowUtilUnknown}
	if w == nil || !lowUtilCovered(w.Overall.Samples, w.Overall.ExpectedSamples) {
		return unknown
	}
	threshold := float64(rule.Threshold)
	switch mode {
	case v1.LowUtilizationAverage:
		if w.Overall.Max < threshold {
			return lowUtilDecision{state: lowUtilIdle, observed: w.Overall.Max}
		}
		return lowUtilDecision{state: lowUtilActive, observed: w.Overall.Max}
	case v1.LowUtilizationMin:
		lowest := -1
		for i := range w.Nodes {
			if !lowUtilCovered(w.Nodes[i].Samples, w.Overall.ExpectedSamples) {
				continue
			}
			if lowest < 0 || w.Nodes[i].Max < w.Nodes[lowest].Max {
				lowest = i
			}
		}
		if lowest < 0 {
			return unknown
		}
		d := lowUtilDecision{state: lowUtilActive, observed: w.Nodes[lowest].Max}
		if d.observed < threshold {
			d.state, d.node = lowUtilIdle, w.Nodes[lowest].Node
		}
		return d
	default:
		return unknown
	}
}

func lowUtilCovered(samples, expected int) bool {
	return expected > 0 && float64(samples) >= lowUtilMinCoverage*float64(expected)
}

func (r *LowUtilizationController) apply(ctx context.Context,
	cluster string, c lowUtilCandidate, d lowUtilDecision) error {
	w := c.workload
	switch d.state {
	case lowUtilUnknown:
		jmmetrics.LowUtilSkippedNoDataTotal.WithLabelValues(cluster).Inc()
		return nil
	case lowUtilActive:
		if _, ok := w.GetAnnotations()[v1.WorkloadLowUtilDetectedAtAnnotation]; ok {
			if episode, open := lowUtilEpisode(w); open {
				if err := r.addLowUtilCondition(ctx, w, lowUtilReasonRecovered, episode,
					fmt.Sprintf("GPU utilization recovered: observed max %.1f%%", d.observed)); err != nil {
					return client.IgnoreNotFound(err)
				}
			}
		}
		return r.clearDetected(ctx, w)
	}

	now := r.now().UTC()
	notifyBefore := parseLowUtilDuration(c.policy.NotifyBefore)
	description := describeLowUtil(c.policy, d)
	detectedAt, err := time.Parse(time.RFC3339, v1.GetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation))
	firstDetection := err != nil
	if firstDetection {
		detectedAt = now
		if err = r.setDetected(ctx, w, detectedAt); err != nil {
			return err
		}
		jmmetrics.LowUtilDetectedTotal.WithLabelValues(cluster).Inc()
		if c.policy.DryRun {
			r.recorder.Eventf(w, corev1.EventTypeWarning, lowUtilEventReason,
				"%s; dry run, the workload will not be stopped", description)
		}
	}
	// The countdown of an enforced policy starts at its own mark, so a workload detected in dry
	// run gets the full notifyBefore once the policy is enforced.
	enforcedAt, err := time.Parse(time.RFC3339, v1.GetAnnotation(w, v1.WorkloadLowUtilEnforcedAtAnnotation))
	startEnforcement := !c.policy.DryRun && err != nil
	if startEnforcement {
		enforcedAt = now
	}
	if c.policy.DryRun {
		if err = r.removeLowUtilAnnotations(ctx, w, v1.WorkloadLowUtilEnforcedAtAnnotation); err != nil {
			return err
		}
	}
	stopAt := enforcedAt.Add(notifyBefore)
	// The annotation is written before the condition, so an episode marked
	// without its Detected entry (a failed status patch) gets it on a later round.
	episode, open := lowUtilEpisode(w)
	newEpisode := firstDetection || !open
	if newEpisode {
		episode++
	}
	outcome := "dry run, the workload will not be stopped"
	if !c.policy.DryRun {
		outcome = fmt.Sprintf("the workload will be stopped at %s if it stays idle", stopAt.Format(time.RFC3339))
	}
	if len(c.policy.Rules) == 0 {
		if newEpisode {
			if err = r.addLowUtilCondition(ctx, w, lowUtilReasonDetected, episode,
				description+"; "+outcome); err != nil {
				return err
			}
		}
	} else {
		// Each rule gets its own entry the first time it is idle in the episode.
		for _, idle := range d.idleRules {
			if err = r.addCondition(ctx, w, jobutils.NewCondition(string(v1.AdminLowUtilization),
				describeLowUtil(c.policy, idle)+"; "+outcome, lowUtilRuleReason(episode, idle.rule))); err != nil {
				return err
			}
		}
	}
	// Written after the Detected entries: the owner's notice fires on these marks and lists them.
	if startEnforcement {
		if !newEpisode {
			if err = r.addLowUtilCondition(ctx, w, lowUtilReasonEnforced, episode,
				"the low utilization policy is enforced now; "+outcome); err != nil {
				return err
			}
		}
		if err = r.setEnforced(ctx, w, enforcedAt); err != nil {
			return err
		}
		r.recorder.Eventf(w, corev1.EventTypeWarning, lowUtilEventReason, "%s; %s", description, outcome)
	}
	if c.policy.DryRun && now.Before(detectedAt.Add(notifyBefore)) || !c.policy.DryRun && now.Before(stopAt) {
		return nil
	}
	if c.policy.DryRun {
		klog.Infof("[low-util] dry run, workload %s would be stopped: %s", w.Name, description)
		return r.addLowUtilCondition(ctx, w, lowUtilReasonWouldStop, episode,
			fmt.Sprintf("dry run: the workload would be stopped now (%s)", description))
	}
	if err = jobutils.MarkWorkloadStopped(ctx, r.Client, w,
		jobutils.StopReasonLowUtilization, "stopped for "+description); err != nil {
		return err
	}
	jmmetrics.LowUtilStoppedTotal.WithLabelValues(cluster).Inc()
	if err = r.Delete(ctx, w); err != nil {
		klog.ErrorS(err, "failed to delete workload", "workload", w.Name)
		return client.IgnoreNotFound(err)
	}
	return nil
}

func describeLowUtil(policy *v1.LowUtilizationPolicy, d lowUtilDecision) string {
	msg := fmt.Sprintf("low GPU utilization: mode %s, threshold %d%%, window %s, observed max %.1f%%",
		policy.Mode, d.rule.Threshold, d.rule.Window, d.observed)
	if len(policy.Rules) > 0 {
		msg = fmt.Sprintf("low GPU utilization below %d%% for %s: mode %s, observed max %.1f%%",
			d.rule.Threshold, d.rule.Window, policy.Mode, d.observed)
	}
	if d.node != "" {
		msg += " on node " + d.node
	}
	return msg
}

// lowUtilEpisode returns the index of the last idle episode on the timeline of
// w, 0 for none, and whether that episode has not recovered yet. The highest
// Detected index is used rather than a count, because the condition history
// may be trimmed. Both Detected_<n> and Detected_<n>_<rule> carry the index.
func lowUtilEpisode(w *v1.Workload) (int, bool) {
	episode := 0
	for _, c := range w.Status.Conditions {
		if c.Type != string(v1.AdminLowUtilization) {
			continue
		}
		suffix, ok := strings.CutPrefix(c.Reason, lowUtilReasonDetected)
		if !ok {
			continue
		}
		index, _, _ := strings.Cut(suffix, "_")
		if n, err := strconv.Atoi(index); err == nil && n > episode {
			episode = n
		}
	}
	if episode == 0 {
		return 0, false
	}
	recovered := jobutils.FindCondition(w, &metav1.Condition{
		Type: string(v1.AdminLowUtilization), Reason: lowUtilReason(lowUtilReasonRecovered, episode)})
	return episode, recovered == nil
}

func lowUtilReason(prefix string, episode int) string {
	return fmt.Sprintf("%s%d", prefix, episode)
}

// lowUtilRuleReason names the Detected entry of one rule in an episode by the
// rule's content, e.g. Detected_1_lt11_30m. Condition reasons allow only
// letters, digits and a few separators, so anything else becomes "_".
func lowUtilRuleReason(episode int, rule v1.LowUtilizationRule) string {
	window := strings.Map(func(r rune) rune {
		if r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, rule.Window)
	return fmt.Sprintf("%s_lt%d_%s", lowUtilReason(lowUtilReasonDetected, episode), rule.Threshold, window)
}

func (r *LowUtilizationController) addLowUtilCondition(ctx context.Context, w *v1.Workload,
	prefix string, episode int, message string) error {
	return r.addCondition(ctx, w,
		jobutils.NewCondition(string(v1.AdminLowUtilization), message, lowUtilReason(prefix, episode)))
}

// addCondition appends a timeline entry unless one with the same type and reason exists.
func (r *LowUtilizationController) addCondition(ctx context.Context, w *v1.Workload, cond *metav1.Condition) error {
	if jobutils.FindCondition(w, cond) != nil {
		return nil
	}
	cond.LastTransitionTime = metav1.NewTime(r.now().UTC())
	conditions := append(append([]metav1.Condition(nil), w.Status.Conditions...), *cond)
	if err := jobutils.PatchWorkloadStatusFields(ctx, r.Client, w,
		map[string]any{"conditions": conditions}); err != nil {
		return err
	}
	w.Status.Conditions = conditions
	return nil
}

func (r *LowUtilizationController) setDetected(ctx context.Context, w *v1.Workload, at time.Time) error {
	orig := w.DeepCopy()
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, at.Format(time.RFC3339))
	return r.Patch(ctx, w, client.MergeFrom(orig))
}

// setEnforced starts the stop countdown and marks the owner as told of it.
func (r *LowUtilizationController) setEnforced(ctx context.Context, w *v1.Workload, at time.Time) error {
	orig := w.DeepCopy()
	v1.SetAnnotation(w, v1.WorkloadLowUtilEnforcedAtAnnotation, at.Format(time.RFC3339))
	v1.SetAnnotation(w, v1.WorkloadLowUtilNotifiedAtAnnotation, at.Format(time.RFC3339))
	return r.Patch(ctx, w, client.MergeFrom(orig))
}

func (r *LowUtilizationController) clearDetected(ctx context.Context, w *v1.Workload) error {
	return client.IgnoreNotFound(r.removeLowUtilAnnotations(ctx, w,
		v1.WorkloadLowUtilDetectedAtAnnotation, v1.WorkloadLowUtilEnforcedAtAnnotation))
}

// removeLowUtilAnnotations removes the given annotations, patching only when one is present.
func (r *LowUtilizationController) removeLowUtilAnnotations(ctx context.Context, w *v1.Workload, keys ...string) error {
	orig := w.DeepCopy()
	changed := false
	for _, key := range keys {
		if _, ok := w.GetAnnotations()[key]; ok {
			delete(w.Annotations, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return r.Patch(ctx, w, client.MergeFrom(orig))
}

// parseLowUtilDuration reads an optional policy duration; the webhook has
// already rejected malformed values, so anything unreadable counts as zero.
func parseLowUtilDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0
	}
	return d
}
