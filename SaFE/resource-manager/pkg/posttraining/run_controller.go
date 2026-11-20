/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	rmmetrics "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

const (
	runningRequeue = 10 * time.Second
	moreLogRequeue = time.Second
	pendingRequeue = 30 * time.Second
	// resultWait is how long a succeeded workload may go without a result marker.
	resultWait = 5 * time.Minute
	// createGrace covers a run whose workload is not in the cache yet.
	createGrace       = 2 * time.Minute
	registryTTL       = time.Minute
	registerTimeout   = 30 * time.Minute
	defaultMaxPoints  = 2000000
	recoverPageSize   = 500
	recoverBufferSize = 1024
)

var (
	stageOrder           = []string{"queued", "prepare", "convert", "check", "train", "export", "register", "eval", "done"}
	separateConvertOrder = []string{"queued", "convert", "prepare", "check", "train", "export", "register", "eval", "done"}
	launcherStages       = map[string]bool{"prepare": true, "convert": true, "check": true, "train": true, "export": true}
)

// lostMessage starts the message of a run whose launcher stopped reporting.
const lostMessage = "The launcher has not reported"

// notRetriedEvent records that the run failed in a way a retry would repeat, so the platform does not retry it.
const notRetriedEvent = "not-retried"

// stageEntry is one record of the stage_history column.
type stageEntry struct {
	Stage      string `json:"stage"`
	Time       string `json:"time"`
	Attempt    string `json:"attempt,omitempty"`
	ResumeStep *int64 `json:"resumeStep,omitempty"`
	Event      string `json:"event,omitempty"`
	Message    string `json:"message,omitempty"`
	// KeptStep is the checkpoint a checkpoint cleanup kept, CleanedSteps the ones it removed.
	KeptStep     *int   `json:"keptStep,omitempty"`
	CleanedSteps *[]int `json:"cleanedSteps,omitempty"`
}

// runRecipe is the part of the recipe snapshot the controller reads.
type runRecipe struct {
	Convert *struct {
		CacheKey    string `json:"cacheKey"`
		Dir         string `json:"dir"`
		SeparateJob bool   `json:"separateJob"`
	} `json:"convert"`
	Evaluation *struct {
		Benchmarks  []string `json:"benchmarks"`
		KeepService bool     `json:"keepService"`
	} `json:"evaluation"`
	Output struct {
		Base string `json:"base"`
		// ExportHF is false when the run exports no model; recipes without it export one.
		ExportHF *bool `json:"exportHf"`
	} `json:"output"`
	Metrics struct {
		Samples *struct {
			MaxRows int64 `json:"maxRows"`
		} `json:"samples"`
	} `json:"metrics"`
}

// defaultMaxSamples caps the stored samples of a run whose recipe names no cap.
const defaultMaxSamples = 50000

// parseRecipe decodes the run's recipe snapshot.
func parseRecipe(run *dbclient.PostTrainingRun) *runRecipe {
	rec := &runRecipe{}
	_ = json.Unmarshal([]byte(run.Recipe), rec)
	return rec
}

// launcherStage reports whether the launcher reports stage with [STAGE] markers.
func launcherStage(stage string) bool {
	return launcherStages[stage]
}

// stageIndex returns the position of stage in order, -1 when absent.
func stageIndex(order []string, stage string) int {
	for i, s := range order {
		if s == stage {
			return i
		}
	}
	return -1
}

// runUpdate collects the columns to change on a run; only values that differ from the row are written.
type runUpdate struct {
	run            *dbclient.PostTrainingRun
	order          []string
	stage          string
	history        []stageEntry
	historyChanged bool
	fields         map[string]interface{}
	now            time.Time
}

// newRunUpdate starts an update of run.
func newRunUpdate(run *dbclient.PostTrainingRun) *runUpdate {
	u := &runUpdate{run: run, order: stageOrder, stage: run.Stage, fields: map[string]interface{}{}, now: time.Now().UTC()}
	if rec := parseRecipe(run); rec.Convert != nil && rec.Convert.SeparateJob {
		u.order = separateConvertOrder
	}
	_ = json.Unmarshal([]byte(run.StageHistory), &u.history)
	return u
}

// set writes column to value when it differs from the current one.
func (u *runUpdate) set(column string, current, value interface{}) {
	if current != value {
		u.fields[column] = value
	}
}

// moveTo advances the run to stage when stage comes later in the run's order, and records it.
func (u *runUpdate) moveTo(stage string) {
	if stageIndex(u.order, stage) <= stageIndex(u.order, u.stage) {
		return
	}
	u.stage = stage
	u.history = append(u.history, stageEntry{Stage: stage, Time: u.now.Format(time.RFC3339)})
	u.historyChanged = true
}

// event records an event, such as a preemption, at the current stage.
func (u *runUpdate) event(name, message string) {
	u.history = append(u.history, stageEntry{Stage: u.stage, Time: u.now.Format(time.RFC3339), Event: name, Message: message})
	u.historyChanged = true
}

// end marks the run finished with status at endTime.
func (u *runUpdate) end(status v1.WorkloadPhase, endTime time.Time) {
	u.set("status", u.run.Status, string(status))
	if !u.run.EndTime.Valid {
		u.fields["end_time"] = endTime
	}
	if u.run.StalledSince.Valid {
		u.fields["stalled_since"] = nil
	}
}

// stageStart returns when the run entered stage, or the zero time when it has not.
func (u *runUpdate) stageStart(stage string) time.Time {
	for _, e := range u.history {
		if e.Stage == stage && e.Event == "" {
			t, _ := time.Parse(time.RFC3339, e.Time)
			return t
		}
	}
	return time.Time{}
}

// fail marks the run failed at stage for reason.
func (u *runUpdate) fail(stage, reason string, endTime time.Time) {
	u.end(v1.WorkloadFailed, endTime)
	u.set("failed_stage", u.run.FailedStage, stage)
	u.set("message", u.run.Message, reason)
	u.event("failed", reason)
}

// columns returns every changed column, including the stage and its history.
func (u *runUpdate) columns() map[string]interface{} {
	if u.stage != u.run.Stage {
		u.fields["stage"] = u.stage
	}
	if u.historyChanged {
		data, _ := json.Marshal(u.history)
		u.fields["stage_history"] = string(data)
	}
	return u.fields
}

// registryCache reloads the Post Training registry at most once per registryTTL and keeps the last valid one.
type registryCache struct {
	mu       sync.Mutex
	store    registry.Store
	version  string
	loadedAt time.Time
}

// get returns the current registry, or nil when none has been valid yet.
func (c *registryCache) get(ctx context.Context, reader client.Reader) *registry.Registry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.loadedAt) >= registryTTL {
		c.loadedAt = time.Now()
		cm := &corev1.ConfigMap{}
		key := client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: registry.ConfigMapName}
		if err := reader.Get(ctx, key, cm); err == nil && cm.ResourceVersion != c.version {
			if err := c.store.Load([]byte(cm.Data[registry.DataKey]), nil); err != nil {
				rmmetrics.PostTrainingRegistryLoadFailuresTotal.Inc()
				klog.ErrorS(err, "rejected a Post Training registry version", "resourceVersion", cm.ResourceVersion)
			}
			c.version = cm.ResourceVersion
		}
	}
	reg, _ := c.store.Get()
	return reg
}

// RunReconciler advances Post Training runs from their workloads, submitter logs and output models.
type RunReconciler struct {
	client.Client
	reader        client.Reader
	db            dbclient.Interface
	clientManager *commonutils.ObjectManager
	registry      registryCache
	recovered     chan event.GenericEvent
}

// SetupPostTrainingController registers the Post Training run controller; it needs the database.
func SetupPostTrainingController(_ context.Context, mgr manager.Manager) error {
	if !commonconfig.IsDBEnable() {
		klog.Info("The database is disabled, skip the Post Training controller")
		return nil
	}
	r := &RunReconciler{
		Client:        mgr.GetClient(),
		reader:        mgr.GetAPIReader(),
		db:            dbclient.NewClient(),
		clientManager: commonutils.NewObjectManagerSingleton(),
		recovered:     make(chan event.GenericEvent, recoverBufferSize),
	}
	ofRun := builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return v1.GetLabel(obj, v1.PostTrainingRunIdLabel) != ""
	}))
	err := ctrlruntime.NewControllerManagedBy(mgr).
		Named("post-training-run").
		Watches(&v1.Workload{}, handler.EnqueueRequestsFromMapFunc(runRequest), ofRun).
		Watches(&v1.Model{}, handler.EnqueueRequestsFromMapFunc(runRequest), ofRun).
		Watches(&v1.OpsJob{}, handler.EnqueueRequestsFromMapFunc(runRequest), ofRun).
		Watches(&v1.Model{}, handler.Funcs{DeleteFunc: func(_ context.Context, _ event.DeleteEvent, q v1.RequestWorkQueue) {
			q.Add(reconcile.Request{NamespacedName: client.ObjectKey{Name: cacheSweepRequest}})
		}}).
		WatchesRawSource(source.Channel(r.recovered, &handler.EnqueueRequestForObject{})).
		Complete(r)
	if err != nil {
		return err
	}
	if err = mgr.Add(manager.RunnableFunc(r.recoverRuns)); err != nil {
		return err
	}
	if err = mgr.Add(manager.RunnableFunc(r.runUnfinishedSweeps)); err != nil {
		return err
	}
	if err = mgr.Add(manager.RunnableFunc(r.runCacheSweeps)); err != nil {
		return err
	}
	if err = mgr.Add(manager.RunnableFunc(r.runCleanupPolls)); err != nil {
		return err
	}
	klog.Info("Setup Post Training controller successfully")
	return nil
}

// runRequest maps an object labeled with a run id to that run.
func runRequest(_ context.Context, obj client.Object) []reconcile.Request {
	return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: v1.GetLabel(obj, v1.PostTrainingRunIdLabel)}}}
}

// recoverRuns queues every unfinished run once the controller leads, so runs whose workloads changed while it
// was down are advanced too.
func (r *RunReconciler) recoverRuns(ctx context.Context) error {
	filter := dbclient.PostTrainingRunFilter{
		Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}, Limit: recoverPageSize}
	for {
		runs, _, err := r.db.ListPostTrainingRuns(ctx, filter)
		if err != nil {
			klog.ErrorS(err, "failed to list unfinished Post Training runs")
			return nil
		}
		for _, run := range runs {
			obj := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: run.RunID}}
			select {
			case r.recovered <- event.GenericEvent{Object: obj}:
			case <-ctx.Done():
				return nil
			}
		}
		if len(runs) < recoverPageSize {
			break
		}
		filter.Offset += recoverPageSize
	}
	<-ctx.Done()
	return nil
}

// requeue queues a run for a reconcile. A run the queue has no room for waits for its next event or sweep.
func (r *RunReconciler) requeue(runID string) {
	obj := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: runID}}
	select {
	case r.recovered <- event.GenericEvent{Object: obj}:
	default:
	}
}

// Reconcile advances one run, keyed by run id.
func (r *RunReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	if req.Name == cacheSweepRequest {
		res, err := r.sweepCaches(ctx)
		return res, reconcileError("convert-cache", err)
	}
	run, err := r.db.GetPostTrainingRun(ctx, req.Name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Only a run that was deleted leaves orphans. A run this database has no row of, as after a restore
			// from a backup, may well be running, and what runs under its id is left alone.
			deleted, err := r.db.IsPostTrainingRunDeleted(ctx, req.Name)
			if err != nil {
				return ctrlruntime.Result{}, reconcileError("database", err)
			}
			if !deleted {
				return ctrlruntime.Result{}, nil
			}
			return ctrlruntime.Result{}, reconcileError("orphans", r.removeOrphans(ctx, req.Name))
		}
		return ctrlruntime.Result{}, reconcileError("database", err)
	}
	if run.Source != "" && run.Source != dbclient.PostTrainingSourceSafe {
		// An imported run is written once by the import API and has no workload to follow.
		return ctrlruntime.Result{}, nil
	}
	if run.CleanupRequest != "" && finished(run) {
		res, err := r.reconcileCleanup(ctx, run)
		return res, reconcileError("cleanup", err)
	}
	if finished(run) && backfillPending(run, newRunUpdate(run).history) {
		res, err := r.reconcileBackfill(ctx, run)
		if err != nil || res.RequeueAfter == 0 {
			return res, reconcileError("backfill", err)
		}
		// A backfill that still runs wrote nothing, so the exports and evaluations go on meanwhile.
		if err = r.reconcileExports(ctx, run); err != nil {
			return ctrlruntime.Result{}, reconcileError("export", err)
		}
		evalRes, err := r.reconcileEvaluations(ctx, run)
		if err != nil {
			return ctrlruntime.Result{}, reconcileError("evaluate", err)
		}
		if evalRes.RequeueAfter > 0 && evalRes.RequeueAfter < res.RequeueAfter {
			res.RequeueAfter = evalRes.RequeueAfter
		}
		return res, nil
	}
	if finished(run) {
		if err := r.reconcileExports(ctx, run); err != nil {
			return ctrlruntime.Result{}, reconcileError("export", err)
		}
	}
	if finished(run) || run.Stage == "eval" {
		res, err := r.reconcileEvaluations(ctx, run)
		return res, reconcileError("evaluate", err)
	}
	w := &v1.Workload{}
	var res ctrlruntime.Result
	if err = r.Get(ctx, client.ObjectKey{Name: run.WorkloadID}, w); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrlruntime.Result{}, reconcileError("workload", err)
		}
		res, err = r.reconcileLost(ctx, run)
	} else {
		res, err = r.reconcileWorkload(ctx, run, w)
	}
	return res, reconcileError("advance", err)
}

// removeOrphans removes what a deleted run left running: its workloads, except kept evaluation services whose
// evaluation is over, and its unfinished OpsJobs, whose evaluation tasks are recorded as cancelled.
func (r *RunReconciler) removeOrphans(ctx context.Context, runID string) error {
	workloads := &v1.WorkloadList{}
	if err := r.List(ctx, workloads, client.MatchingLabels{v1.PostTrainingRunIdLabel: runID}); err != nil {
		return err
	}
	for i := range workloads.Items {
		w := &workloads.Items[i]
		// A done service stays only when it was to be kept; one that is done and not kept was not removed when its
		// evaluation ended, and nothing else would remove it now.
		kept := v1.GetAnnotation(w, v1.PostTrainingEvalDoneAnnotation) == v1.TrueStr &&
			v1.GetAnnotation(w, evalKeepAnnotation) == v1.TrueStr
		if w.IsEnd() || kept {
			continue
		}
		klog.Infof("delete workload %s of deleted post training run %s", w.Name, runID)
		if err := r.Delete(ctx, w); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	jobs := &v1.OpsJobList{}
	if err := r.List(ctx, jobs, client.MatchingLabels{v1.PostTrainingRunIdLabel: runID}); err != nil {
		return err
	}
	for i := range jobs.Items {
		if err := r.cancelJob(ctx, jobs.Items[i].Name); err != nil {
			return err
		}
	}
	return nil
}

// finished reports whether the controller has nothing left to do for run.
func finished(run *dbclient.PostTrainingRun) bool {
	switch v1.WorkloadPhase(run.Status) {
	case v1.WorkloadFailed, v1.WorkloadStopped:
		return true
	case v1.WorkloadSucceeded:
		return run.Stage == "done"
	}
	return false
}

// noteWorkload records on u what a look at the run's workload tells of it: its UID and when it started.
func noteWorkload(u *runUpdate, w *v1.Workload) {
	u.set("workload_uid", u.run.WorkloadUID, string(w.UID))
	if w.Status.StartTime != nil && !u.run.StartTime.Valid {
		u.fields["start_time"] = w.Status.StartTime.Time
	}
}

// reconcileWorkload follows the run's workload through queueing, running and its end.
func (r *RunReconciler) reconcileWorkload(ctx context.Context, run *dbclient.PostTrainingRun, w *v1.Workload) (ctrlruntime.Result, error) {
	u := newRunUpdate(run)
	noteWorkload(u, w)
	switch {
	case w.IsPending():
		syncQueued(u, w)
		return ctrlruntime.Result{RequeueAfter: pendingRequeue}, r.save(ctx, u)
	case w.IsRunning():
		u.set("status", run.Status, string(v1.WorkloadRunning))
		if !run.StalledSince.Valid {
			u.set("message", run.Message, "")
		}
		u.set("queue_position", run.QueuePosition, 0)
		if run.Stage == "queued" {
			u.moveTo(u.order[1])
		}
		more, err := r.readLog(ctx, w, u)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		if more {
			return ctrlruntime.Result{RequeueAfter: moreLogRequeue}, nil
		}
		return ctrlruntime.Result{RequeueAfter: runningRequeue}, nil
	case w.IsEnd():
		if _, err := r.readLog(ctx, w, u); err != nil {
			klog.ErrorS(err, "failed the last read of the submitter log", "run", run.RunID)
		}
		run, err := r.db.GetPostTrainingRun(ctx, run.RunID)
		if err != nil {
			return ctrlruntime.Result{}, client.IgnoreNotFound(err)
		}
		endTime := u.now
		if w.Status.EndTime != nil {
			endTime = w.Status.EndTime.Time
		}
		end := newRunUpdate(run)
		// The read of the log saves these with what it read, and may have failed for a workload first seen here.
		noteWorkload(end, w)
		return r.finish(ctx, end, w.Status.Phase, endTime, w)
	}
	return ctrlruntime.Result{RequeueAfter: runningRequeue}, nil
}

// syncQueued copies the scheduling message and queue position of a pending workload. A workload sent back to
// the queue by a preemption or a retry keeps the run Running at its stage.
func syncQueued(u *runUpdate, w *v1.Workload) {
	run := u.run
	if v1.GetWorkloadDispatchCnt(w) > 0 && run.Status == string(v1.WorkloadRunning) {
		name, message := "preempted", "Preempted, waiting to be rescheduled"
		if !v1.IsWorkloadPreempted(w) {
			name, message = "retry", fmt.Sprintf("Retry %d, waiting to be scheduled", v1.GetRetryCount(w))
		}
		if run.Message != message {
			u.event(name, message)
		}
		u.set("message", run.Message, message)
	} else {
		u.set("status", run.Status, string(v1.WorkloadPending))
		u.set("message", run.Message, w.Status.Message)
	}
	u.set("queue_position", run.QueuePosition, w.Status.QueuePosition)
}

// reconcileLost handles a run whose workload record is gone: its end phase comes from the workload table and
// its result from the workload_pod rows that outlive the record.
func (r *RunReconciler) reconcileLost(ctx context.Context, run *dbclient.PostTrainingRun) (ctrlruntime.Result, error) {
	u := newRunUpdate(run)
	if run.Status == string(v1.WorkloadPending) && run.CreatedAt.Valid && u.now.Sub(run.CreatedAt.Time) < createGrace {
		return ctrlruntime.Result{RequeueAfter: runningRequeue}, nil
	}
	// A run is also reconciled without an event of its workload, so the cache may not show a workload the API
	// server has: the run is then followed, whatever its status, and not taken for lost.
	w := &v1.Workload{}
	if err := r.reader.Get(ctx, client.ObjectKey{Name: run.WorkloadID}, w); err == nil {
		return r.reconcileWorkload(ctx, run, w)
	} else if !apierrors.IsNotFound(err) {
		return ctrlruntime.Result{}, err
	}
	phase, endTime := v1.WorkloadPhase(""), u.now
	if row, err := r.db.GetWorkload(ctx, run.WorkloadID); err == nil && row != nil {
		phase = v1.WorkloadPhase(row.Phase.String)
		if row.EndTime.Valid {
			endTime = row.EndTime.Time
		}
		// The workload ran, although the controller did not see it. What it left is kept by its UID, which this
		// reconcile reads by as well.
		if run.WorkloadUID == "" && row.WorkloadUId.Valid {
			run.WorkloadUID = row.WorkloadUId.String
			u.fields["workload_uid"] = run.WorkloadUID
		}
	}
	switch phase {
	case v1.WorkloadSucceeded, v1.WorkloadFailed, v1.WorkloadStopped:
		return r.finish(ctx, u, phase, endTime, nil)
	}
	if res := r.result(ctx, run, nil); res != nil {
		if res.Kind == markerSuccess {
			return r.finish(ctx, u, v1.WorkloadSucceeded, endTime, nil)
		}
		return r.finish(ctx, u, v1.WorkloadFailed, endTime, nil)
	}
	return r.verify(ctx, u, endTime, run.Stage,
		"The workload ended while the controller was down and its result was lost")
}

// stopMessage returns the reason the stop API gave a stopped workload, such as who stopped it, or "" when w is
// unknown.
func stopMessage(w *v1.Workload) string {
	if w == nil {
		return ""
	}
	if c := meta.FindStatusCondition(w.Status.Conditions, string(v1.AdminStopped)); c != nil {
		return c.Message
	}
	return ""
}

// finish records how the run ended once its workload reached phase; a success goes on to registration.
func (r *RunReconciler) finish(ctx context.Context, u *runUpdate, phase v1.WorkloadPhase, endTime time.Time,
	w *v1.Workload) (ctrlruntime.Result, error) {
	run := u.run
	if phase == v1.WorkloadStopped {
		if message := stopMessage(w); message != "" {
			u.set("message", run.Message, message)
		}
		u.end(v1.WorkloadStopped, endTime)
		return ctrlruntime.Result{}, r.save(ctx, u)
	}
	res := r.result(ctx, run, w)
	if res != nil {
		u.set("result_line", run.ResultLine, res.Text)
	}
	switch {
	case res != nil && res.Kind == markerError:
		stage := run.Stage
		if launcherStage(res.Body.Stage) {
			stage = res.Body.Stage
		}
		u.fail(stage, res.Body.Reason, endTime)
	case phase == v1.WorkloadFailed:
		reason := "The workload failed"
		if w != nil && w.Status.Message != "" {
			reason = w.Status.Message
		}
		u.fail(run.Stage, reason, endTime)
	case res == nil && u.now.Sub(endTime) < resultWait:
		return ctrlruntime.Result{RequeueAfter: pendingRequeue}, r.save(ctx, u)
	case res == nil:
		return r.verify(ctx, u, endTime, "export",
			"The workload succeeded but the run directory has no complete exported model")
	default:
		return r.register(ctx, u, res, endTime)
	}
	return ctrlruntime.Result{}, r.save(ctx, u)
}

// result returns the run's last result marker from the result_line column and the submitter's workload_pod
// row, ignoring results of attempts older than the latest one read.
func (r *RunReconciler) result(ctx context.Context, run *dbclient.PostTrainingRun, w *v1.Workload) *marker {
	best := lastResult(run.ResultLine, run.RunID)
	var pods []v1.WorkloadPod
	if w != nil {
		pods = commonworkload.PodsOf(ctx, r.db, w)
	} else if rows, err := r.db.ListWorkloadPods(ctx, run.WorkloadID, run.WorkloadUID); err == nil {
		pods = dbclient.WorkloadPodsToV1(rows)
	}
	for _, pod := range pods {
		for _, c := range pod.Containers {
			if c.Name != SubmitterContainer {
				continue
			}
			if m := lastResult(c.Message, run.RunID); m != nil &&
				(best == nil || attemptIndex(m.Body.Attempt) >= attemptIndex(best.Body.Attempt)) {
				best = m
			}
		}
	}
	if best != nil && attemptIndex(best.Body.Attempt) < loadCursor(run.MetricCursor).latestIndex() {
		return nil
	}
	return best
}

// save writes the changed columns of u and sends the notifications they imply.
func (r *RunReconciler) save(ctx context.Context, u *runUpdate) error {
	fields := u.columns()
	if len(fields) == 0 {
		return nil
	}
	if err := r.db.UpdatePostTrainingRun(ctx, u.run.RunID, fields); err != nil {
		return ignoreRecordNotFound(err)
	}
	r.notifyChanges(ctx, u.run, fields)
	r.followEnd(u.run, fields)
	return nil
}

// followEnd queues a run that fields ended once more. What follows the end, such as the backfill of its metrics, is
// done by the next reconcile, which no event of the run's objects would start.
func (r *RunReconciler) followEnd(run *dbclient.PostTrainingRun, fields map[string]interface{}) {
	switch fields["status"] {
	case string(v1.WorkloadFailed), string(v1.WorkloadStopped):
		r.requeue(run.RunID)
		return
	}
	if fields["stage"] == "done" {
		r.requeue(run.RunID)
	}
}

// ignoreRecordNotFound drops the error of a run deleted in the meantime.
func ignoreRecordNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

// readLog reads the submitter log from the run's cursor, applies the markers and saves the points with the
// advanced cursor and the other changed columns in one transaction. It reports whether more log is waiting.
func (r *RunReconciler) readLog(ctx context.Context, w *v1.Workload, u *runUpdate) (bool, error) {
	run := u.run
	cur := loadCursor(run.MetricCursor)
	in := r.newIngestion(ctx, u, cur)
	rejected, skipped := in.sum.Rejected, cur.Skipped
	more, readErr := r.readSubmitter(ctx, w, cur, in)
	points := in.finish()
	if in.resumeFrom != nil {
		if err := r.db.DeletePostTrainingRunMetricsAfterStep(ctx, run.RunID, int(*in.resumeFrom)); err != nil {
			return false, err
		}
		resume := int(*in.resumeFrom)
		if err := r.db.DeletePostTrainingRunSamples(ctx, run.RunID, &resume); err != nil {
			return false, err
		}
	}
	// Samples the database refuses must not hold up the metrics, stages and cursor read with them, so saveSamples
	// drops those; it fails when the database could not be asked, and the same log is then read again.
	if err := r.saveSamples(ctx, run, in, in.finishSamples()); err != nil {
		return false, err
	}
	switch {
	case in.result != nil:
		u.set("result_line", run.ResultLine, in.result.Text)
	case in.newAttempt:
		u.set("result_line", run.ResultLine, "")
	}
	syncStall(u, in, w.IsRunning())
	if in.disk != "" {
		u.set("disk_usage", run.DiskUsage, in.disk)
	}
	if in.noRetry {
		markNotRetried(u)
		if err := r.disableFailover(ctx, w); err != nil {
			return false, err
		}
	}
	gap := cur.hasGap(u.now)
	u.set("metric_gap", run.MetricGap, gap)
	if data, _ := json.Marshal(cur); string(data) != run.MetricCursor {
		u.fields["metric_cursor"] = string(data)
	}
	if summary, ok := in.summaryUpdate(points, run.MetricSummary); ok {
		u.fields["metric_summary"] = summary
	}
	fields := u.columns()
	if err := r.db.SavePostTrainingRunMetrics(ctx, run.RunID, points, fields); err != nil {
		return false, ignoreRecordNotFound(err)
	}
	countRead(in, rejected, skipped, run.MetricGap, gap)
	r.notifyChanges(ctx, run, fields)
	if in.conversion != nil {
		r.registerConversion(ctx, run, in.conversion)
	}
	if readErr != nil {
		klog.V(2).InfoS("submitter log not readable yet", "run", run.RunID, "err", readErr)
	}
	return more, nil
}

// newIngestion prepares the marker ingestion of one read with the run's summary metric and point cap.
func (r *RunReconciler) newIngestion(ctx context.Context, u *runUpdate, cur *logCursor) *ingestion {
	in := &ingestion{u: u, cur: cur, sum: loadSummary(u.run.MetricSummary), maxPoints: defaultMaxPoints}
	if reg := r.registry.get(ctx, r.reader); reg != nil {
		if fw := reg.FrameworkByName(u.run.Framework); fw != nil {
			in.mainName = fw.Metrics.Summary[u.run.TrainType]
			if fw.Metrics.MaxPointsPerRun > 0 {
				in.maxPoints = int64(fw.Metrics.MaxPointsPerRun)
			}
		}
	}
	return in
}

// readSubmitter streams the newest submitter pod's log from the cursor into in. It reports whether the read
// stopped at the size limit.
func (r *RunReconciler) readSubmitter(ctx context.Context, w *v1.Workload, cur *logCursor, in *ingestion) (bool, error) {
	clients, err := utils.GetK8sClientFactory(r.clientManager, v1.GetClusterId(w))
	if err != nil || !clients.IsValid() {
		return false, fmt.Errorf("the clients of cluster %s are not ready", v1.GetClusterId(w))
	}
	pods, err := clients.ClientSet().CoreV1().Pods(w.Spec.Workspace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", v1.WorkloadIdLabel, w.Name)})
	if err != nil {
		return false, err
	}
	var pod *corev1.Pod
	for i := range pods.Items {
		p := &pods.Items[i]
		if hasContainer(p, SubmitterContainer) && (pod == nil || p.CreationTimestamp.After(pod.CreationTimestamp.Time)) {
			pod = p
		}
	}
	if pod == nil {
		return false, nil
	}
	cur.setPod(pod.Name)
	opts := &corev1.PodLogOptions{Container: SubmitterContainer, Timestamps: true, LimitBytes: ptr.To(int64(maxReadBytes))}
	if t := cur.sinceTime(); !t.IsZero() {
		opts.SinceTime = &metav1.Time{Time: t}
	}
	stream, err := clients.ClientSet().CoreV1().Pods(w.Spec.Workspace).GetLogs(pod.Name, opts).Stream(ctx)
	if err != nil {
		return false, err
	}
	defer stream.Close()
	n, err := scanLog(stream, cur, func(ts time.Time, line string) {
		if m, ok := parseMarker(line, in.u.run.RunID); ok {
			if m.Kind == markerStage {
				cur.LastStage = ts.Unix()
			}
			in.add(m)
		}
	})
	in.logRead = err == nil
	return n >= maxReadBytes, err
}

// syncStall marks the run stalled from the watchdog's warning, or when a running launcher whose log was read has
// printed no [STAGE] for lostAfter, and clears the mark on progress or, for a lost launcher, on its next [STAGE].
func syncStall(u *runUpdate, in *ingestion, running bool) {
	run := u.run
	lost := strings.HasPrefix(run.Message, lostMessage)
	switch {
	case in.stall != nil:
		since := time.Unix(0, int64(in.stall.Since*float64(time.Second))).UTC()
		last := "none"
		if in.stall.LastStep != nil {
			last = strconv.FormatInt(*in.stall.LastStep, 10)
		}
		message := fmt.Sprintf("No training progress for %d minutes (last step %s); the attempt is stopped and "+
			"retried at %d minutes", in.stall.Minutes, last, in.stall.StopMinutes)
		if !run.StalledSince.Valid || lost {
			u.event("stalled", message)
		}
		u.fields["stalled_since"] = since
		u.set("message", run.Message, message)
	case run.StalledSince.Valid && (in.unstalled || (lost && in.sawStage)):
		u.fields["stalled_since"] = nil
		u.set("message", run.Message, "")
	case running && in.logRead && !run.StalledSince.Valid && !in.sawStage && in.cur.LastStage > 0 &&
		u.now.Sub(time.Unix(in.cur.LastStage, 0)) > lostAfter:
		minutes := int(u.now.Sub(time.Unix(in.cur.LastStage, 0)).Minutes())
		message := fmt.Sprintf("%s for %d minutes", lostMessage, minutes)
		u.event("lost", message)
		u.fields["stalled_since"] = time.Unix(in.cur.LastStage, 0).UTC()
		u.set("message", run.Message, message)
	}
}

// refusedByDatabase tells whether the database took a statement and refused its data, which another try does not
// change, as opposed to not having been reached. The error is asked for its SQLSTATE, which the errors of both
// PostgreSQL drivers in use give: GORM writes through pgx, the other queries through pq.
func refusedByDatabase(err error) bool {
	var refused interface{ SQLState() string }
	if !errors.As(err, &refused) {
		return false
	}
	switch state := refused.SQLState(); {
	case len(state) < 2:
		return false
	case state[:2] == "21", state[:2] == "22", state[:2] == "23":
		// Cardinality violation, data exception, integrity constraint violation.
		return true
	}
	return false
}

// saveSamples stores the samples read up to the run's cap and counts the stored ones in the summary; once the cap
// is reached the summary says so and later samples stay only in the run directory. Samples the database refuses
// are dropped; it fails when the database could not be asked.
func (r *RunReconciler) saveSamples(ctx context.Context, run *dbclient.PostTrainingRun, in *ingestion,
	samples []*dbclient.PostTrainingRunSample) error {
	if len(samples) == 0 {
		return nil
	}
	limit := int64(defaultMaxSamples)
	if s := parseRecipe(run).Metrics.Samples; s != nil && s.MaxRows > 0 {
		limit = s.MaxRows
	}
	room := limit - in.sum.Samples
	if room < int64(len(samples)) {
		in.sum.SamplesCapped, in.sampled = true, true
		if room <= 0 {
			return nil
		}
		samples = samples[:room]
	}
	if err := r.db.UpsertPostTrainingRunSamples(ctx, samples); err != nil {
		if !refusedByDatabase(err) {
			return err
		}
		klog.ErrorS(err, "the database refused the samples of a post training run", "run", run.RunID)
		return nil
	}
	if n, err := r.db.CountPostTrainingRunSamples(ctx, run.RunID); err == nil {
		in.sum.Samples = n
	}
	in.sampled = true
	return nil
}

// markNotRetried records the not-retried event once.
func markNotRetried(u *runUpdate) {
	if !hasEvent(u.history, notRetriedEvent) {
		u.event(notRetriedEvent, "")
	}
}

// disableFailover stops the platform from dispatching the workload again after an error that would recur.
func (r *RunReconciler) disableFailover(ctx context.Context, w *v1.Workload) error {
	if v1.IsWorkloadDisableFailover(w) {
		return nil
	}
	patch := client.MergeFrom(w.DeepCopy())
	v1.SetAnnotation(w, v1.WorkloadDisableFailoverAnnotation, v1.TrueStr)
	return client.IgnoreNotFound(r.Patch(ctx, w, patch))
}

// hasContainer reports whether pod runs a container named name.
func hasContainer(pod *corev1.Pod, name string) bool {
	for _, c := range pod.Spec.Containers {
		if c.Name == name {
			return true
		}
	}
	return false
}
