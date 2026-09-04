/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	modelprewarm "github.com/AMD-AIG-AIMA/SAFE/common/pkg/model_prewarm"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

type ModelPrewarmJobReconciler struct {
	*OpsJobBaseReconciler
}

// SetupModelPrewarmJobController initializes and registers the ModelPrewarmJobReconciler.
func SetupModelPrewarmJobController(mgr manager.Manager) error {
	r := &ModelPrewarmJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client:        mgr.GetClient(),
			clientManager: utils.NewObjectManagerSingleton(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Model Prewarm Job Controller successfully")
	return nil
}

// Reconcile is the main control loop for model prewarm OpsJob resources.
func (r *ModelPrewarmJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupAnnotations}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

func (r *ModelPrewarmJobReconciler) observe(_ context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

func (r *ModelPrewarmJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobModelPrewarmType
}

func (r *ModelPrewarmJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		if err := r.setJobPhase(ctx, job, v1.OpsJobRunning); err != nil {
			return ctrlruntime.Result{}, err
		}
		if err := r.dispatchRequests(ctx, job); err != nil {
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if job.Status.Phase == v1.OpsJobRunning {
		return r.checkAndUpdateJobStatus(ctx, job)
	}
	return ctrlruntime.Result{}, nil
}

type modelPrewarmConfig struct {
	modelPath   string
	glob        string
	parallelism int
}

func (r *ModelPrewarmJobReconciler) parseConfig(job *v1.OpsJob) (*modelPrewarmConfig, error) {
	modelPathParam := job.GetParameter(v1.ParameterModelPath)
	if modelPathParam == nil || strings.TrimSpace(modelPathParam.Value) == "" {
		return nil, fmt.Errorf("model.path is missing")
	}
	cfg := &modelPrewarmConfig{
		modelPath:   strings.TrimSpace(modelPathParam.Value),
		glob:        modelprewarm.DefaultGlob,
		parallelism: modelprewarm.DefaultParallelism,
	}
	if globParam := job.GetParameter(v1.ParameterModelGlob); globParam != nil && globParam.Value != "" {
		cfg.glob = strings.TrimSpace(globParam.Value)
	}
	if parallelismParam := job.GetParameter(v1.ParameterParallelism); parallelismParam != nil && parallelismParam.Value != "" {
		p, err := strconv.Atoi(parallelismParam.Value)
		if err != nil || p <= 0 {
			return nil, fmt.Errorf("invalid parallelism: %s", parallelismParam.Value)
		}
		cfg.parallelism = p
	}
	return cfg, nil
}

func (r *ModelPrewarmJobReconciler) getTargetNodes(job *v1.OpsJob) []string {
	var nodes []string
	for _, input := range job.Spec.Inputs {
		if input.Name == v1.ParameterNode && input.Value != "" {
			nodes = append(nodes, input.Value)
		}
	}
	return nodes
}

func (r *ModelPrewarmJobReconciler) dispatchRequests(ctx context.Context, job *v1.OpsJob) error {
	cfg, err := r.parseConfig(job)
	if err != nil {
		return err
	}
	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, v1.GetClusterId(job))
	if err != nil {
		return err
	}

	reqPayload := &modelprewarm.Request{
		OpsJobId:    job.Name,
		ModelPath:   cfg.modelPath,
		Glob:        cfg.glob,
		Parallelism: cfg.parallelism,
		RequestedAt: time.Now().UTC(),
	}
	reqValue, err := modelprewarm.MarshalRequest(reqPayload)
	if err != nil {
		return err
	}
	reqKey := modelprewarm.RequestAnnotationKey(job.Name)

	for _, adminNodeName := range r.getTargetNodes(job) {
		adminNode, err := r.getAdminNode(ctx, adminNodeName)
		if err != nil {
			return err
		}
		if err := r.patchAdminNodeAnnotation(ctx, adminNode, reqKey, reqValue); err != nil {
			return err
		}
		k8sNodeName := adminNode.GetK8sNodeName()
		if k8sNodeName == "" {
			return fmt.Errorf("k8s node name is empty for admin node %s", adminNodeName)
		}
		if err := r.patchK8sNodeAnnotation(ctx, k8sClients, k8sNodeName, reqKey, reqValue); err != nil {
			return err
		}
	}
	return nil
}

func (r *ModelPrewarmJobReconciler) checkAndUpdateJobStatus(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	targets := r.getTargetNodes(job)
	if len(targets) == 0 {
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, "no target nodes found", nil)
	}

	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, v1.GetClusterId(job))
	if err != nil {
		if job.Status.StartedAt != nil && time.Since(job.Status.StartedAt.Time) < 2*time.Minute {
			return ctrlruntime.Result{RequeueAfter: 5 * time.Second}, nil
		}
		errMsg := fmt.Sprintf("failed to get k8s client factory: %v", err)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, errMsg, nil)
	}

	var (
		succeeded int
		failed    int
		details   []modelprewarm.NodeDetail
		failures  []string
	)

	for _, adminNodeName := range targets {
		adminNode, err := r.getAdminNode(ctx, adminNodeName)
		if err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("%s(%v)", adminNodeName, err))
			details = append(details, modelprewarm.NodeDetail{
				AdminNodeId: adminNodeName,
				Phase:       modelprewarm.PhaseFailed,
				Message:     err.Error(),
			})
			continue
		}
		k8sNodeName := adminNode.GetK8sNodeName()
		result, err := r.readK8sNodeResult(ctx, k8sClients, k8sNodeName, job.Name)
		detail := modelprewarm.NodeDetail{
			Node:        k8sNodeName,
			AdminNodeId: adminNodeName,
		}
		if err != nil || result == nil {
			detail.Phase = "Pending"
			details = append(details, detail)
			continue
		}
		detail.Phase = result.Phase
		detail.Message = result.Message
		detail.DurationSeconds = result.DurationSeconds
		detail.BytesRead = result.BytesRead
		details = append(details, detail)

		switch result.Phase {
		case modelprewarm.PhaseSucceeded:
			succeeded++
		case modelprewarm.PhaseFailed:
			failed++
			msg := result.Message
			if msg == "" {
				msg = "failed"
			}
			failures = append(failures, fmt.Sprintf("%s(%s)", k8sNodeName, msg))
		}
	}

	total := len(targets)
	pending := total - succeeded - failed
	if err := r.updateModelPrewarmProgress(ctx, job, succeeded, failed, pending, total); err != nil {
		klog.V(4).ErrorS(err, "failed to update model prewarm progress", "job", job.Name)
	}

	if failed > 0 {
		message := fmt.Sprintf("model prewarm failed on %d/%d nodes: %s", failed, total, strings.Join(failures, ", "))
		outputs := r.buildModelPrewarmOutputs(total, succeeded, failed, details, "Failed", message)
		if err := r.cleanupAnnotations(ctx, job); err != nil {
			klog.ErrorS(err, "failed to cleanup model prewarm annotations", "job", job.Name)
		}
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, message, outputs)
	}
	if succeeded == total && total > 0 {
		message := fmt.Sprintf("model prewarm completed successfully on %d nodes", total)
		outputs := r.buildModelPrewarmOutputs(total, succeeded, failed, details, "Succeeded", message)
		if err := r.cleanupAnnotations(ctx, job); err != nil {
			klog.ErrorS(err, "failed to cleanup model prewarm annotations", "job", job.Name)
		}
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobSucceeded, message, outputs)
	}

	return ctrlruntime.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ModelPrewarmJobReconciler) buildModelPrewarmOutputs(
	total, succeeded, failed int, details []modelprewarm.NodeDetail, status, message string) []v1.Parameter {
	detailJSON, _ := json.Marshal(details)
	progress := 0
	if total > 0 {
		progress = succeeded * 100 / total
	}
	return []v1.Parameter{
		{Name: "status", Value: status},
		{Name: "message", Value: message},
		{Name: "nodes_total", Value: strconv.Itoa(total)},
		{Name: "nodes_succeeded", Value: strconv.Itoa(succeeded)},
		{Name: "nodes_failed", Value: strconv.Itoa(failed)},
		{Name: "nodes_pending", Value: strconv.Itoa(total - succeeded - failed)},
		{Name: "prewarm_progress", Value: fmt.Sprintf("%d%%", progress)},
		{Name: "nodes_detail", Value: string(detailJSON)},
	}
}

func (r *ModelPrewarmJobReconciler) updateModelPrewarmProgress(
	ctx context.Context, job *v1.OpsJob, succeeded, failed, pending, total int) error {
	message := fmt.Sprintf("model prewarm %d/%d nodes completed", succeeded+failed, total)
	cond := &metav1.Condition{
		Type:               JobProcessingType,
		Status:             metav1.ConditionTrue,
		Reason:             "ModelPrewarmInProgress",
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now().UTC()),
	}
	return r.updateCondition(ctx, job, cond)
}

func (r *ModelPrewarmJobReconciler) readK8sNodeResult(
	ctx context.Context, k8sClients *k8sclient.ClientFactory, k8sNodeName, jobName string) (*modelprewarm.Result, error) {
	if k8sNodeName == "" {
		return nil, fmt.Errorf("k8s node name is empty")
	}
	k8sNode, err := k8sClients.ClientSet().CoreV1().Nodes().Get(ctx, k8sNodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	raw, ok := k8sNode.Annotations[modelprewarm.ResultAnnotationKey(jobName)]
	if !ok || raw == "" {
		return nil, nil
	}
	return modelprewarm.ParseResult(raw)
}

func (r *ModelPrewarmJobReconciler) patchAdminNodeAnnotation(
	ctx context.Context, adminNode *v1.Node, key, value string) error {
	original := client.MergeFrom(adminNode.DeepCopy())
	if !v1.SetAnnotation(adminNode, key, value) {
		return nil
	}
	return r.Patch(ctx, adminNode, original)
}

func (r *ModelPrewarmJobReconciler) patchK8sNodeAnnotation(
	ctx context.Context, k8sClients *k8sclient.ClientFactory, k8sNodeName, key, value string) error {
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"%s":%s}}}`, key, jsonEscape(value)))
	_, err := k8sClients.ClientSet().CoreV1().Nodes().Patch(
		ctx, k8sNodeName, apitypes.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

func (r *ModelPrewarmJobReconciler) cleanupAnnotations(ctx context.Context, job *v1.OpsJob) error {
	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, v1.GetClusterId(job))
	if err != nil {
		klog.V(4).InfoS("skip model prewarm annotation cleanup: cluster client not available",
			"job", job.Name, "cluster", v1.GetClusterId(job))
		return nil
	}
	reqKey := modelprewarm.RequestAnnotationKey(job.Name)
	resKey := modelprewarm.ResultAnnotationKey(job.Name)
	for _, adminNodeName := range r.getTargetNodes(job) {
		adminNode := &v1.Node{}
		if err := r.Get(ctx, client.ObjectKey{Name: adminNodeName}, adminNode); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if err := r.removeAdminNodeAnnotations(ctx, adminNode, reqKey, resKey); err != nil {
			return err
		}
		k8sNodeName := adminNode.GetK8sNodeName()
		if k8sNodeName == "" {
			continue
		}
		if err := r.removeK8sNodeAnnotations(ctx, k8sClients, k8sNodeName, reqKey, resKey); err != nil {
			return err
		}
	}
	return nil
}

func (r *ModelPrewarmJobReconciler) removeAdminNodeAnnotations(
	ctx context.Context, adminNode *v1.Node, keys ...string) error {
	original := client.MergeFrom(adminNode.DeepCopy())
	changed := false
	for _, key := range keys {
		if adminNode.Annotations != nil && adminNode.Annotations[key] != "" {
			delete(adminNode.Annotations, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return r.Patch(ctx, adminNode, original)
}

func (r *ModelPrewarmJobReconciler) removeK8sNodeAnnotations(
	ctx context.Context, k8sClients *k8sclient.ClientFactory, k8sNodeName string, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	var parts []string
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`"%s":null`, key))
	}
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{%s}}}`, strings.Join(parts, ",")))
	_, err := k8sClients.ClientSet().CoreV1().Nodes().Patch(
		ctx, k8sNodeName, apitypes.MergePatchType, patch, metav1.PatchOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func jsonEscape(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "\"\""
	}
	return string(data)
}
