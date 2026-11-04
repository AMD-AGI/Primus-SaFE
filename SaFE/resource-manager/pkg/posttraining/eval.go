/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/evaluation"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/topic/posttrainingrun"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	// The annotations of an evaluation inference service: the automatic evaluation after training, a service
	// that outlives its evaluation, the benchmark datasets and the evaluation OpsJob once created. A kept
	// service whose evaluation is over carries v1.PostTrainingEvalDoneAnnotation.
	evalAutoAnnotation       = v1.PostTrainingPrefix + "eval-auto"
	evalKeepAnnotation       = v1.PostTrainingPrefix + "eval-keep"
	evalBenchmarksAnnotation = v1.PostTrainingPrefix + "eval-benchmarks"
	evalJobAnnotation        = v1.PostTrainingPrefix + "eval-job"

	// evalStartedEvent is the stage_history event that records the service of the automatic evaluation;
	// evalRepeatedEvent records the second one of an A/B baseline.
	evalStartedEvent  = "evaluate"
	evalRepeatedEvent = "evaluate-repeat"
	evalFailedEvent   = "evaluate-failed"

	defaultEvalPort      = 8000
	defaultEvalReadiness = "/v1/models"
	defaultEvalWait      = 2 * time.Hour
	evalRequeue          = 30 * time.Second
)

// evalRequest is what one evaluation runs: the benchmark datasets and whether the service outlives it.
type evalRequest struct {
	Benchmarks  []string `json:"benchmarks"`
	KeepService bool     `json:"keepService,omitempty"`
}

// reconcileEvaluations drives the evaluations of a finished run: the automatic one while the run is at stage
// eval, and the ones requested on its models. Each runs an inference service, then an evaluation OpsJob
// against it, and removes the service afterwards unless asked to keep it.
func (r *RunReconciler) reconcileEvaluations(ctx context.Context, run *dbclient.PostTrainingRun) (ctrlruntime.Result, error) {
	services := &v1.WorkloadList{}
	if err := r.List(ctx, services, client.MatchingLabels{
		v1.PostTrainingRunIdLabel: run.RunID, v1.PostTrainingRoleLabel: v1.PostTrainingRoleEval}); err != nil {
		return ctrlruntime.Result{}, err
	}
	var requeue time.Duration
	present := map[string]bool{}
	autoSeen := false
	for i := range services.Items {
		s := &services.Items[i]
		present[s.Name] = true
		autoSeen = autoSeen || v1.GetAnnotation(s, evalAutoAnnotation) == v1.TrueStr
		if v1.GetAnnotation(s, v1.PostTrainingEvalDoneAnnotation) == v1.TrueStr {
			continue
		}
		after, err := r.advanceEvaluation(ctx, run, s)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		if after > 0 && (requeue == 0 || after < requeue) {
			requeue = after
		}
	}
	if err := r.cancelOrphanJobs(ctx, run, present); err != nil {
		return ctrlruntime.Result{}, err
	}
	if requeue > 0 {
		return ctrlruntime.Result{RequeueAfter: requeue}, nil
	}
	if run.Stage == "eval" {
		if autoSeen {
			return ctrlruntime.Result{}, nil
		}
		return r.startAutoEvaluation(ctx, run)
	}
	return r.startRequestedEvaluation(ctx, run)
}

// advanceEvaluation moves one evaluation forward and returns when to look again, zero once it is over.
func (r *RunReconciler) advanceEvaluation(ctx context.Context, run *dbclient.PostTrainingRun, s *v1.Workload) (time.Duration, error) {
	jobName := v1.GetAnnotation(s, evalJobAnnotation)
	if s.IsEnd() || !s.GetDeletionTimestamp().IsZero() {
		// The service was stopped or deleted: the evaluation is cancelled.
		if jobName != "" {
			if err := r.cancelJob(ctx, jobName); err != nil {
				return 0, err
			}
		}
		return 0, r.finishEvaluation(ctx, run, s, false)
	}
	if jobName == "" {
		if !s.IsRunning() {
			if wait := r.evalWait(ctx); time.Since(s.CreationTimestamp.Time) > wait {
				r.recordEvalFailure(ctx, run, s, fmt.Sprintf("The inference service was not ready within %s", wait))
				return 0, r.finishEvaluation(ctx, run, s, true)
			}
			return evalRequeue, nil
		}
		name, err := r.createEvalJob(ctx, run, s)
		if err != nil {
			return 0, err
		}
		patch := client.MergeFrom(s.DeepCopy())
		v1.SetAnnotation(s, evalJobAnnotation, name)
		return evalRequeue, client.IgnoreNotFound(r.Patch(ctx, s, patch))
	}
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobName}, job); err != nil {
		if !apierrors.IsNotFound(err) {
			return 0, err
		}
		// The evaluation job was deleted: the evaluation is cancelled.
		return 0, r.finishEvaluation(ctx, run, s, true)
	}
	if job.Status.Phase != v1.OpsJobSucceeded && job.Status.Phase != v1.OpsJobFailed {
		return evalRequeue, nil
	}
	if v1.GetAnnotation(s, evalAutoAnnotation) == v1.TrueStr && !hasEvent(newRunUpdate(run).history, evalRepeatedEvent) {
		r.notify(ctx, run, nil, posttrainingrun.EventEvaluated, nil)
	}
	return 0, r.finishEvaluation(ctx, run, s, true)
}

// finishEvaluation ends an evaluation: the service is deleted, or marked done when it is kept, and the
// automatic evaluation moves the run to done.
func (r *RunReconciler) finishEvaluation(ctx context.Context, run *dbclient.PostTrainingRun, s *v1.Workload, removeService bool) error {
	switch {
	case !s.GetDeletionTimestamp().IsZero():
	case removeService && v1.GetAnnotation(s, evalKeepAnnotation) != v1.TrueStr:
		if err := r.Delete(ctx, s); client.IgnoreNotFound(err) != nil {
			return err
		}
	default:
		patch := client.MergeFrom(s.DeepCopy())
		v1.SetAnnotation(s, v1.PostTrainingEvalDoneAnnotation, v1.TrueStr)
		if err := r.Patch(ctx, s, patch); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	if v1.GetAnnotation(s, evalAutoAnnotation) != v1.TrueStr || run.Stage != "eval" {
		return nil
	}
	u := newRunUpdate(run)
	repeated, err := r.repeatBaselineEvaluation(ctx, run, u)
	if err != nil {
		return err
	}
	if !repeated {
		u.moveTo("done")
	}
	return r.save(ctx, u)
}

// repeatBaselineEvaluation evaluates the baseline of an A/B experiment a second time on the same benchmarks, so the
// experiment can tell evaluation noise from a real difference. It returns whether it started the second evaluation.
func (r *RunReconciler) repeatBaselineEvaluation(ctx context.Context, run *dbclient.PostTrainingRun, u *runUpdate) (bool, error) {
	if run.ExperimentID == "" || hasEvent(u.history, evalRepeatedEvent) {
		return false, nil
	}
	exp, err := r.db.GetPostTrainingExperiment(ctx, run.ExperimentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	rec := parseRecipe(run)
	if exp.BaselineVariant != run.VariantName || rec.Evaluation == nil || len(rec.Evaluation.Benchmarks) == 0 {
		return false, nil
	}
	name, err := r.startEvaluation(ctx, run, run.OutputModelID, evalRequest{Benchmarks: rec.Evaluation.Benchmarks}, true)
	if err != nil {
		klog.ErrorS(err, "failed to repeat the baseline evaluation", "run", run.RunID)
		u.event(evalFailedEvent, err.Error())
		return false, nil
	}
	u.event(evalRepeatedEvent, name)
	return true, nil
}

// cancelOrphanJobs stops the evaluation jobs of a run whose inference service is gone.
func (r *RunReconciler) cancelOrphanJobs(ctx context.Context, run *dbclient.PostTrainingRun, services map[string]bool) error {
	jobs := &v1.OpsJobList{}
	if err := r.List(ctx, jobs, client.MatchingLabels{
		v1.PostTrainingRunIdLabel: run.RunID, v1.OpsJobTypeLabel: string(v1.OpsJobEvaluationType)}); err != nil {
		return err
	}
	for i := range jobs.Items {
		job := &jobs.Items[i]
		service := job.GetParameter(v1.ParameterEvalServiceId)
		if service == nil || services[service.Value] || job.Status.Phase == v1.OpsJobSucceeded ||
			job.Status.Phase == v1.OpsJobFailed {
			continue
		}
		if err := r.cancelJob(ctx, job.Name); err != nil {
			return err
		}
	}
	return nil
}

// cancelJob deletes an unfinished evaluation job and records its task as cancelled.
func (r *RunReconciler) cancelJob(ctx context.Context, name string) error {
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, job); err != nil {
		return client.IgnoreNotFound(err)
	}
	if job.Status.Phase == v1.OpsJobSucceeded || job.Status.Phase == v1.OpsJobFailed {
		return nil
	}
	if task := job.GetParameter(v1.ParameterEvalTaskId); task != nil {
		if err := r.db.UpdateEvaluationTaskStatus(ctx, task.Value, dbclient.EvaluationTaskStatusCancelled); err != nil {
			klog.ErrorS(err, "failed to cancel an evaluation task", "task", task.Value)
		}
	}
	return client.IgnoreNotFound(r.Delete(ctx, job))
}

// startAutoEvaluation starts the evaluation configured at submission on the run's output model, or moves the
// run to done when there is none to start.
func (r *RunReconciler) startAutoEvaluation(ctx context.Context, run *dbclient.PostTrainingRun) (ctrlruntime.Result, error) {
	u := newRunUpdate(run)
	rec := parseRecipe(run)
	switch {
	case hasEvent(u.history, evalStartedEvent):
		// The service of the automatic evaluation is gone without finishing it, e.g. the user deleted it.
	case rec.Evaluation == nil || len(rec.Evaluation.Benchmarks) == 0 || run.OutputModelID == "":
	default:
		req := evalRequest{Benchmarks: rec.Evaluation.Benchmarks, KeepService: rec.Evaluation.KeepService}
		name, err := r.startEvaluation(ctx, run, run.OutputModelID, req, true)
		if err == nil {
			u.event(evalStartedEvent, name)
			return ctrlruntime.Result{RequeueAfter: evalRequeue}, r.save(ctx, u)
		}
		klog.ErrorS(err, "failed to start the automatic evaluation", "run", run.RunID)
		u.event(evalFailedEvent, err.Error())
	}
	u.moveTo("done")
	return ctrlruntime.Result{}, r.save(ctx, u)
}

// startRequestedEvaluation starts one evaluation requested by the Post Training API on a model of the run and
// clears the request.
func (r *RunReconciler) startRequestedEvaluation(ctx context.Context, run *dbclient.PostTrainingRun) (ctrlruntime.Result, error) {
	models := &v1.ModelList{}
	if err := r.List(ctx, models, client.MatchingLabels{v1.PostTrainingRunIdLabel: run.RunID}); err != nil {
		return ctrlruntime.Result{}, err
	}
	for i := range models.Items {
		model := &models.Items[i]
		raw := v1.GetAnnotation(model, v1.PostTrainingEvaluateAnnotation)
		if raw == "" {
			continue
		}
		var req evalRequest
		err := json.Unmarshal([]byte(raw), &req)
		if err == nil {
			_, err = r.startEvaluation(ctx, run, model.Name, req, false)
		}
		if err != nil {
			klog.ErrorS(err, "failed to start a requested evaluation", "run", run.RunID, "model", model.Name)
			u := newRunUpdate(run)
			u.event(evalFailedEvent, err.Error())
			if serr := r.save(ctx, u); serr != nil {
				return ctrlruntime.Result{}, serr
			}
		}
		patch := client.MergeFrom(model.DeepCopy())
		delete(model.Annotations, v1.PostTrainingEvaluateAnnotation)
		if err = r.Patch(ctx, model, patch); client.IgnoreNotFound(err) != nil {
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{RequeueAfter: evalRequeue}, nil
	}
	return ctrlruntime.Result{}, nil
}

// hasEvent reports whether history records the event.
func hasEvent(history []stageEntry, event string) bool {
	for _, e := range history {
		if e.Event == event {
			return true
		}
	}
	return false
}

// startEvaluation creates the inference service that evaluates modelID and returns its name.
func (r *RunReconciler) startEvaluation(ctx context.Context, run *dbclient.PostTrainingRun, modelID string,
	req evalRequest, auto bool) (string, error) {
	if len(req.Benchmarks) == 0 {
		return "", fmt.Errorf("the evaluation has no benchmarks")
	}
	model := &v1.Model{}
	if err := r.Get(ctx, client.ObjectKey{Name: modelID}, model); err != nil {
		return "", err
	}
	if !model.IsReady() {
		return "", fmt.Errorf("model %s is not ready", modelID)
	}
	s, err := r.evalService(ctx, run, model, req, auto)
	if err != nil {
		return "", err
	}
	return s.Name, r.Create(ctx, s)
}

// evalService builds the inference service of an evaluation: a Deployment workload in the run's workspace that
// serves the model on the registry's evaluation inference command, with the spec's evalTp GPUs.
func (r *RunReconciler) evalService(ctx context.Context, run *dbclient.PostTrainingRun, model *v1.Model,
	req evalRequest, auto bool) (*v1.Workload, error) {
	reg := r.registry.get(ctx, r.reader)
	if reg == nil {
		return nil, fmt.Errorf("the Post Training registry is not loaded")
	}
	spec := reg.MatchSpec(model.Status.ModelConfig)
	if spec == nil {
		return nil, fmt.Errorf("no Post Training spec matches model %s", model.Name)
	}
	inference := mergeInference(reg.Evaluation.Inference, spec.EvalInference)
	tp := max(spec.EvalTp, 1)
	path := model.Spec.Source.LocalPath
	if path == "" {
		return nil, fmt.Errorf("model %s has no local path", model.Name)
	}
	ws := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: run.Workspace}, ws); err != nil {
		return nil, err
	}
	var flavor *v1.NodeFlavor
	if ws.Spec.NodeFlavor != "" {
		nf := &v1.NodeFlavor{}
		if err := r.Get(ctx, client.ObjectKey{Name: ws.Spec.NodeFlavor}, nf); err == nil {
			flavor = nf
		}
	}
	image := inference.Image
	if image == "" || image == "fromImages" {
		image = run.Image
		if flavor != nil && flavor.Spec.Gpu != nil {
			if img := reg.DefaultImage(run.Framework, reg.Arch(string(flavor.Spec.Gpu.Product))); img != nil {
				image = img.Image
			}
		}
	}
	command := strings.NewReplacer("{model}", path, "{name}", model.Name, "{tp}", strconv.Itoa(tp)).Replace(inference.Command)
	if inference.ExtraArgs != "" {
		command += " " + inference.ExtraArgs
	}
	env := map[string]string{"PRIMUS_SOURCE_MODEL": model.Name, "MODEL_PATH": path}
	for k, v := range inference.Env {
		env[k] = v
	}
	benchmarks, _ := json.Marshal(req.Benchmarks)
	w := &v1.Workload{}
	w.Name = commonutils.GenerateName(run.RunID + "-eval")
	w.Labels = map[string]string{
		v1.PostTrainingRunIdLabel: run.RunID,
		v1.PostTrainingRoleLabel:  v1.PostTrainingRoleEval,
		v1.UserIdLabel:            run.UserID,
	}
	if name := "eval-" + run.DisplayName; len(validation.IsValidLabelValue(name)) == 0 {
		w.Labels[v1.DisplayNameLabel] = name
	}
	w.Annotations = map[string]string{
		v1.UserNameAnnotation:    run.UserName,
		v1.DescriptionAnnotation: fmt.Sprintf("Evaluation inference service of Post Training run %s", run.RunID),
		evalBenchmarksAnnotation: string(benchmarks),
	}
	if auto {
		w.Annotations[evalAutoAnnotation] = v1.TrueStr
	}
	if req.KeepService {
		w.Annotations[evalKeepAnnotation] = v1.TrueStr
	}
	w.Spec = v1.WorkloadSpec{
		GroupVersionKind: v1.GroupVersionKind{Kind: common.DeploymentKind, Version: common.DefaultVersion},
		Workspace:        run.Workspace,
		Images:           []string{image},
		EntryPoints:      []string{base64.StdEncoding.EncodeToString([]byte(command))},
		Env:              env,
		Resources:        []v1.WorkloadResource{serviceResource(flavor, tp)},
		Priority:         run.Priority,
		Readiness: &v1.HealthCheck{Path: inference.ReadinessPath, Port: inference.Port,
			InitialDelaySeconds: 30, PeriodSeconds: 10, FailureThreshold: 3},
		Service: &v1.Service{Protocol: corev1.ProtocolTCP, Port: inference.Port, TargetPort: inference.Port,
			ServiceType: corev1.ServiceTypeClusterIP},
	}
	return w, nil
}

// mergeInference returns the global evaluation inference settings overridden by the spec's own.
func mergeInference(global registry.Inference, own *registry.Inference) registry.Inference {
	out := global
	if own != nil {
		if own.Image != "" {
			out.Image = own.Image
		}
		if own.Command != "" {
			out.Command = own.Command
		}
		if own.ExtraArgs != "" {
			out.ExtraArgs = own.ExtraArgs
		}
		if len(own.Env) > 0 {
			out.Env = own.Env
		}
		if own.Port > 0 {
			out.Port = own.Port
		}
		if own.ReadinessPath != "" {
			out.ReadinessPath = own.ReadinessPath
		}
		if own.MaxWaitSeconds > 0 {
			out.MaxWaitSeconds = own.MaxWaitSeconds
		}
	}
	if out.Port <= 0 {
		out.Port = defaultEvalPort
	}
	if out.ReadinessPath == "" {
		out.ReadinessPath = defaultEvalReadiness
	}
	return out
}

// serviceResource requests the share of a node that tp of its GPUs carry: the same fraction of its CPU, memory
// and ephemeral storage, and half that memory as shared memory.
func serviceResource(flavor *v1.NodeFlavor, tp int) v1.WorkloadResource {
	r := v1.WorkloadResource{Replica: 1, GPU: strconv.Itoa(tp)}
	gpus := 0
	if flavor != nil {
		gpus = flavor.GetGpuCount()
	}
	if gpus <= 0 {
		r.CPU, r.Memory, r.SharedMemory, r.EphemeralStorage = "16", "64Gi", "32Gi", "50Gi"
		return r
	}
	share := func(q resource.Quantity) int64 { return q.Value() * int64(min(tp, gpus)) / int64(gpus) }
	avail := quantity.GetAvailableResource(flavor.ToResourceList(""))
	if q, ok := avail[corev1.ResourceCPU]; ok {
		r.CPU = resource.NewMilliQuantity(q.MilliValue()*int64(min(tp, gpus))/int64(gpus), resource.DecimalSI).String()
	}
	if q, ok := avail[corev1.ResourceMemory]; ok {
		r.Memory = resource.NewQuantity(share(q), resource.BinarySI).String()
		r.SharedMemory = resource.NewQuantity(share(q)/2, resource.BinarySI).String()
	}
	if q, ok := avail[corev1.ResourceEphemeralStorage]; ok {
		r.EphemeralStorage = resource.NewQuantity(share(q), resource.BinarySI).String()
	}
	return r
}

// evalWait returns how long an inference service may take to become ready.
func (r *RunReconciler) evalWait(ctx context.Context) time.Duration {
	if reg := r.registry.get(ctx, r.reader); reg != nil && reg.Evaluation.Inference.MaxWaitSeconds > 0 {
		return time.Duration(reg.Evaluation.Inference.MaxWaitSeconds) * time.Second
	}
	return defaultEvalWait
}

// evalJobName is the name of the evaluation job that runs against an inference service.
func evalJobName(service string) string {
	sum := sha256.Sum256([]byte(service))
	return "pt-eval-" + hex.EncodeToString(sum[:8])
}

// createEvalJob builds the evaluation task of a ready inference service, with the registry's output probes,
// and creates its OpsJob; it returns the job name.
func (r *RunReconciler) createEvalJob(ctx context.Context, run *dbclient.PostTrainingRun, s *v1.Workload) (string, error) {
	name := evalJobName(s.Name)
	if err := r.Get(ctx, client.ObjectKey{Name: name}, &v1.OpsJob{}); err == nil {
		return name, nil
	}
	task, err := evaluation.BuildTask(ctx, r.Client, r.db, r.adminSecret, evaluation.TaskRequest{
		Name: s.Name, ServiceID: s.Name, ServiceType: evaluation.ServiceTypeLocalWorkload,
		Benchmarks: benchmarkList(v1.GetAnnotation(s, evalBenchmarksAnnotation)), Workspace: run.Workspace,
		UserID: run.UserID, UserName: run.UserName, PostTrainingRunID: run.RunID, ModelID: s.Spec.Env["PRIMUS_SOURCE_MODEL"],
	})
	if err != nil {
		return "", err
	}
	inputs := task.Inputs
	if reg := r.registry.get(ctx, r.reader); reg != nil && len(reg.Evaluation.Probes) > 0 {
		probes, _ := json.Marshal(reg.Evaluation.Probes)
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterEvalProbes, Value: string(probes)})
	}
	userName := run.UserName
	if userName == "" {
		userName = run.UserID
	}
	job := &v1.OpsJob{}
	job.Name = name
	job.Labels = map[string]string{
		v1.UserIdLabel:            run.UserID,
		v1.DisplayNameLabel:       name,
		v1.PostTrainingRunIdLabel: run.RunID,
	}
	if task.Cluster != "" {
		job.Labels[v1.ClusterIdLabel] = task.Cluster
	}
	if task.Workspace != "" {
		job.Labels[v1.WorkspaceIdLabel] = task.Workspace
	}
	job.Annotations = map[string]string{v1.UserNameAnnotation: userName}
	job.Spec = v1.OpsJobSpec{Type: v1.OpsJobEvaluationType, Inputs: inputs, TimeoutSecond: evaluation.DefaultTimeoutSecond}
	if err = r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		if ferr := r.db.SetEvaluationTaskFailed(ctx, task.TaskID, err.Error()); ferr != nil {
			klog.ErrorS(ferr, "failed to mark an evaluation task failed", "task", task.TaskID)
		}
		return "", err
	}
	if err = r.db.UpdateEvaluationTaskOpsJobId(ctx, task.TaskID, name); err != nil {
		klog.ErrorS(err, "failed to record the evaluation job of a task", "task", task.TaskID, "job", name)
	}
	return name, nil
}

// benchmarkList turns the JSON list of benchmark dataset ids into the benchmarks of an evaluation task.
func benchmarkList(ids string) string {
	var list []string
	_ = json.Unmarshal([]byte(ids), &list)
	benchmarks := make([]evaluation.Benchmark, 0, len(list))
	for _, id := range list {
		benchmarks = append(benchmarks, evaluation.Benchmark{DatasetId: id})
	}
	data, _ := json.Marshal(benchmarks)
	return string(data)
}

// recordEvalFailure stores a failed evaluation task for a service that never became ready, so the run shows why
// its evaluation has no scores.
func (r *RunReconciler) recordEvalFailure(ctx context.Context, run *dbclient.PostTrainingRun, s *v1.Workload, reason string) {
	task := &dbclient.EvaluationTask{
		TaskId:            fmt.Sprintf("eval-task-%s", uuid.New().String()[:8]),
		TaskName:          s.Name,
		ServiceId:         s.Name,
		ServiceType:       evaluation.ServiceTypeLocalWorkload,
		ServiceName:       s.Name,
		Benchmarks:        benchmarkList(v1.GetAnnotation(s, evalBenchmarksAnnotation)),
		EvalParams:        "{}",
		Status:            dbclient.EvaluationTaskStatusPending,
		Workspace:         run.Workspace,
		UserId:            run.UserID,
		UserName:          run.UserName,
		CreationTime:      pq.NullTime{Time: time.Now().UTC(), Valid: true},
		PostTrainingRunId: sql.NullString{String: run.RunID, Valid: true},
		ModelId:           sql.NullString{String: s.Spec.Env["PRIMUS_SOURCE_MODEL"], Valid: s.Spec.Env["PRIMUS_SOURCE_MODEL"] != ""},
	}
	if err := r.db.UpsertEvaluationTask(ctx, task); err != nil {
		klog.ErrorS(err, "failed to record a failed evaluation", "run", run.RunID)
		return
	}
	if err := r.db.SetEvaluationTaskFailed(ctx, task.TaskId, reason); err != nil {
		klog.ErrorS(err, "failed to record a failed evaluation", "run", run.RunID)
	}
}

// adminSecret reads a Secret of the platform namespace straight from the API server.
func (r *RunReconciler) adminSecret(ctx context.Context, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.reader.Get(ctx, client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: name}, secret)
	return secret, err
}
