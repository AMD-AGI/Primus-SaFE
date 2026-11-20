/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

const (
	// backfilledEvent is the stage_history event of the end-of-run backfill, which runs once.
	backfilledEvent    = "backfilled"
	backfillEntrypoint = "python3 /shared-data/safe_post_training/backfill.py"
	backfillTTLSeconds = 3600
	// backfillTimeoutSeconds bounds the backfill job once it runs, and backfillWait the backfill from its creation,
	// queueing included.
	backfillTimeoutSeconds = 1800
	backfillWait           = time.Hour
)

// backfillSpec is the PT_BACKFILL of the end-of-run backfill.
type backfillSpec struct {
	Run     string                `json:"run"`
	Dir     string                `json:"dir"`
	Attempt string                `json:"attempt"`
	Stage   string                `json:"stage"`
	Ranges  map[string][][2]int64 `json:"ranges"`
}

// backfillWorkloadName names the end-of-run backfill workload of a run.
func backfillWorkloadName(runID string) string {
	sum := sha256.Sum256([]byte(runID))
	return "pt-backfill-" + hex.EncodeToString(sum[:8])
}

// backfillPending reports whether a finished run still needs its end-of-run backfill: gaps are left, or a run
// that failed or was stopped needs its directory counted again. A run without the UID of a workload has none that
// is known to have run, as when its workload was never created: it has no directory to go over.
func backfillPending(run *dbclient.PostTrainingRun, history []stageEntry) bool {
	if run.OutputPath == "" || run.WorkloadUID == "" || hasEvent(history, backfilledEvent) {
		return false
	}
	for _, a := range loadCursor(run.MetricCursor).Attempts {
		if len(a.Gaps) > 0 {
			return true
		}
	}
	return run.Status == string(v1.WorkloadFailed) || run.Status == string(v1.WorkloadStopped)
}

// reconcileBackfill runs the end-of-run backfill of a finished run: a read-only workload re-prints the missing
// points from metrics/metrics.jsonl and the run directory's usage, and the controller reads its log like the
// submitter's. A failed backfill, or one that did not finish within backfillWait, only leaves the gaps; the run's
// status does not change. It writes the run only once the backfill is over, and then returns no requeue.
func (r *RunReconciler) reconcileBackfill(ctx context.Context, run *dbclient.PostTrainingRun) (ctrlruntime.Result, error) {
	cur := loadCursor(run.MetricCursor)
	w := &v1.Workload{}
	err := r.Get(ctx, client.ObjectKey{Name: backfillWorkloadName(run.RunID)}, w)
	if apierrors.IsNotFound(err) {
		return ctrlruntime.Result{RequeueAfter: pendingRequeue}, client.IgnoreAlreadyExists(r.Create(ctx, newBackfillWorkload(run, cur)))
	}
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if !w.IsEnd() {
		if time.Since(w.CreationTimestamp.Time) < backfillWait {
			return ctrlruntime.Result{RequeueAfter: pendingRequeue}, nil
		}
		u := newRunUpdate(run)
		u.event(backfilledEvent, fmt.Sprintf("the backfill did not finish within %s", backfillWait))
		if err = r.save(ctx, u); err != nil {
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{}, client.IgnoreNotFound(r.Delete(ctx, w))
	}
	u := newRunUpdate(run)
	in := r.newIngestion(ctx, u, cur)
	in.backfill = true
	message := "backfilled"
	if err = r.readBackfill(ctx, w, in); err != nil {
		message = "the backfill log could not be read: " + err.Error()
		klog.ErrorS(err, "failed to read the backfill of a run", "run", run.RunID)
	}
	points := in.finish()
	u.event(backfilledEvent, message)
	if in.disk != "" {
		u.set("disk_usage", run.DiskUsage, in.disk)
	}
	u.set("metric_gap", run.MetricGap, cur.hasGap(u.now))
	if data, _ := json.Marshal(cur); string(data) != run.MetricCursor {
		u.fields["metric_cursor"] = string(data)
	}
	if data, _ := json.Marshal(in.sum); (len(points) > 0 || in.capReached(run.MetricSummary)) &&
		string(data) != run.MetricSummary {
		u.fields["metric_summary"] = string(data)
	}
	if err = r.db.SavePostTrainingRunMetrics(ctx, run.RunID, points, u.columns()); err != nil {
		return ctrlruntime.Result{}, ignoreRecordNotFound(err)
	}
	return ctrlruntime.Result{}, client.IgnoreNotFound(r.Delete(ctx, w))
}

// newBackfillWorkload builds the low-priority CPU job that runs backfill.py on the run directory for the gaps of
// cur, reporting as the latest attempt.
func newBackfillWorkload(run *dbclient.PostTrainingRun, cur *logCursor) *v1.Workload {
	spec := backfillSpec{Run: run.RunID, Dir: run.OutputPath, Attempt: "a1", Stage: run.Stage, Ranges: map[string][][2]int64{}}
	if n := len(cur.Order); n > 0 {
		spec.Attempt = cur.Order[n-1]
	}
	for attempt, a := range cur.Attempts {
		for _, g := range a.Gaps {
			spec.Ranges[attempt] = append(spec.Ranges[attempt], [2]int64{g.From, g.To})
		}
	}
	data, _ := json.Marshal(spec)
	w := &v1.Workload{}
	w.Name = backfillWorkloadName(run.RunID)
	w.Labels = map[string]string{
		v1.DisplayNameLabel: commonutils.DNSDisplayName("backfill-", run.DisplayName,
			commonutils.MaxGeneratedNameLength),
		v1.PostTrainingRunIdLabel: run.RunID,
		v1.PostTrainingRoleLabel:  v1.PostTrainingRoleBackfill,
		v1.UserIdLabel:            run.UserID,
	}
	w.Annotations = map[string]string{
		v1.UserNameAnnotation:    run.UserName,
		v1.DescriptionAnnotation: fmt.Sprintf("Metric backfill of Post Training run %s", run.RunID),
	}
	w.Spec = v1.WorkloadSpec{
		GroupVersionKind:        v1.GroupVersionKind{Kind: common.JobKind, Version: common.DefaultVersion},
		Workspace:               run.Workspace,
		Images:                  []string{commonconfig.GetDownloadJoImage()},
		EntryPoints:             []string{base64.StdEncoding.EncodeToString([]byte(backfillEntrypoint))},
		Env:                     map[string]string{"PT_BACKFILL": base64.StdEncoding.EncodeToString(data)},
		Resources:               []v1.WorkloadResource{{Replica: 1, CPU: "1", Memory: "2Gi", EphemeralStorage: "1Gi"}},
		Priority:                common.LowPriorityInt,
		Timeout:                 ptr.To(backfillTimeoutSeconds),
		TTLSecondsAfterFinished: ptr.To(backfillTTLSeconds),
	}
	return w
}

// readBackfill reads the whole log of the backfill workload's pod into in.
func (r *RunReconciler) readBackfill(ctx context.Context, w *v1.Workload, in *ingestion) error {
	clients, err := utils.GetK8sClientFactory(r.clientManager, v1.GetClusterId(w))
	if err != nil || !clients.IsValid() {
		return fmt.Errorf("the clients of cluster %s are not ready", v1.GetClusterId(w))
	}
	pods, err := clients.ClientSet().CoreV1().Pods(w.Spec.Workspace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", v1.WorkloadIdLabel, w.Name)})
	if err != nil {
		return err
	}
	var pod *corev1.Pod
	for i := range pods.Items {
		if p := &pods.Items[i]; len(p.Spec.Containers) > 0 && (pod == nil || p.CreationTimestamp.After(pod.CreationTimestamp.Time)) {
			pod = p
		}
	}
	if pod == nil {
		return fmt.Errorf("the backfill pod is gone")
	}
	opts := &corev1.PodLogOptions{Container: pod.Spec.Containers[0].Name, Timestamps: true,
		LimitBytes: ptr.To(int64(maxReadBytes))}
	stream, err := clients.ClientSet().CoreV1().Pods(w.Spec.Workspace).GetLogs(pod.Name, opts).Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	_, err = scanLog(stream, &logCursor{}, func(_ time.Time, line string) {
		if m, ok := parseMarker(line, in.u.run.RunID); ok {
			in.add(m)
		}
	})
	return err
}
