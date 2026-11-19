/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
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
}

func (r *LowUtilizationController) runOnce(ctx context.Context) {
	queries, err := r.collectCandidates(ctx)
	if err != nil {
		klog.ErrorS(err, "[low-util] failed to collect workloads")
		return
	}
	for q, candidates := range queries {
		r.evaluate(ctx, q, candidates)
	}
}

// collectCandidates selects the running workloads under a policy whose grace
// period has passed. Workloads that left their policy lose the detected mark.
func (r *LowUtilizationController) collectCandidates(ctx context.Context) (map[lowUtilQuery][]lowUtilCandidate, error) {
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

	now := r.now()
	queries := make(map[lowUtilQuery][]lowUtilCandidate)
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
		if w.Status.StartTime == nil || now.Before(w.Status.StartTime.Add(parseLowUtilDuration(policy.GracePeriod))) {
			continue
		}
		cluster := v1.GetClusterId(w)
		if cluster == "" {
			continue
		}
		q := lowUtilQuery{cluster: cluster, window: policy.Window}
		queries[q] = append(queries[q], lowUtilCandidate{workload: w, policy: policy})
	}
	return queries, nil
}

func (r *LowUtilizationController) evaluate(ctx context.Context, q lowUtilQuery, candidates []lowUtilCandidate) {
	uids := make([]string, 0, len(candidates))
	for _, c := range candidates {
		uids = append(uids, string(c.workload.UID))
	}
	windows, err := r.source.GetWorkloadGPUWindow(ctx, q.cluster, uids, q.window, lowUtilStep)
	if err != nil {
		reason := "error"
		switch {
		case robustclient.IsNotSupported(err):
			reason = "unsupported"
		case errors.Is(err, errLowUtilNoClient):
			reason = "no_client"
		}
		jmmetrics.LowUtilClusterSkippedTotal.WithLabelValues(q.cluster, reason).Inc()
		klog.Warningf("[low-util] skip cluster %s (window %s, %d workloads): %v", q.cluster, q.window, len(uids), err)
		return
	}
	byUID := make(map[string]*robustclient.WorkloadGPUWindow, len(windows))
	for i := range windows {
		byUID[windows[i].UID] = &windows[i]
	}
	for _, c := range candidates {
		d := decideLowUtil(c.policy, byUID[string(c.workload.UID)])
		if err = r.apply(ctx, q.cluster, c, d); err != nil {
			klog.ErrorS(err, "[low-util] failed to apply the decision", "workload", c.workload.Name)
		}
	}
}

// decideLowUtil judges one workload. Without enough samples, or in Min mode
// without any covered node, the state is unknown. Robust reports Min nodes per
// physical host and leaves out hosts without an exporter, so a missing node
// cannot make the workload active.
func decideLowUtil(policy *v1.LowUtilizationPolicy, w *robustclient.WorkloadGPUWindow) lowUtilDecision {
	unknown := lowUtilDecision{state: lowUtilUnknown}
	if w == nil || !lowUtilCovered(w.Overall.Samples, w.Overall.ExpectedSamples) {
		return unknown
	}
	threshold := float64(policy.Threshold)
	switch policy.Mode {
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
		return r.clearDetected(ctx, w)
	}

	now := r.now().UTC()
	notifyBefore := parseLowUtilDuration(c.policy.NotifyBefore)
	description := describeLowUtil(c.policy, d)
	detectedAt, err := time.Parse(time.RFC3339, v1.GetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation))
	if err != nil {
		detectedAt = now
		if err = r.setDetected(ctx, w, detectedAt); err != nil {
			return err
		}
		jmmetrics.LowUtilDetectedTotal.WithLabelValues(cluster).Inc()
		if c.policy.DryRun {
			r.recorder.Eventf(w, corev1.EventTypeWarning, lowUtilEventReason,
				"%s; dry run, the workload will not be stopped", description)
		} else {
			r.recorder.Eventf(w, corev1.EventTypeWarning, lowUtilEventReason,
				"%s; the workload will be stopped at %s if it stays idle",
				description, detectedAt.Add(notifyBefore).Format(time.RFC3339))
		}
	}
	if now.Before(detectedAt.Add(notifyBefore)) {
		return nil
	}
	if c.policy.DryRun {
		klog.Infof("[low-util] dry run, workload %s would be stopped: %s", w.Name, description)
		return nil
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
		policy.Mode, policy.Threshold, policy.Window, d.observed)
	if d.node != "" {
		msg += " on node " + d.node
	}
	return msg
}

func (r *LowUtilizationController) setDetected(ctx context.Context, w *v1.Workload, at time.Time) error {
	orig := w.DeepCopy()
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, at.Format(time.RFC3339))
	return r.Patch(ctx, w, client.MergeFrom(orig))
}

func (r *LowUtilizationController) clearDetected(ctx context.Context, w *v1.Workload) error {
	if _, ok := w.GetAnnotations()[v1.WorkloadLowUtilDetectedAtAnnotation]; !ok {
		return nil
	}
	orig := w.DeepCopy()
	delete(w.Annotations, v1.WorkloadLowUtilDetectedAtAnnotation)
	return client.IgnoreNotFound(r.Patch(ctx, w, client.MergeFrom(orig)))
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
