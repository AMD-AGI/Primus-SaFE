/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

const (
	// exportRecipeEnv carries the export job's recipe, whose exportOnly part names the new model.
	exportRecipeEnv = "PT_RECIPE"
	// exportModelAnnotation on an export job names the Model registered from it, so a Model the user deleted is not
	// registered again while the job is kept.
	exportModelAnnotation = v1.PostTrainingPrefix + "export-model"
)

// reconcileExports registers a Model for every checkpoint export of a finished run whose job succeeded; the run's
// status and output model stay as they are.
func (r *RunReconciler) reconcileExports(ctx context.Context, run *dbclient.PostTrainingRun) error {
	jobs := &v1.WorkloadList{}
	if err := r.List(ctx, jobs, client.MatchingLabels{v1.PostTrainingRunIdLabel: run.RunID,
		v1.PostTrainingRoleLabel: v1.PostTrainingRoleExport}); err != nil {
		return err
	}
	for i := range jobs.Items {
		if w := &jobs.Items[i]; w.Status.Phase == v1.WorkloadSucceeded {
			if err := r.registerExport(ctx, run, w); err != nil {
				return err
			}
		}
	}
	return nil
}

// registerExport creates the Model of a succeeded export job, named after the run and the step so a retry finds
// it, and records it on the job; once recorded, the Model only inherits the base model's structure summary when it
// is visible, and is not created again after it was deleted. A job whose result marker is not stored yet is taken
// again when its pod row changes.
func (r *RunReconciler) registerExport(ctx context.Context, run *dbclient.PostTrainingRun, w *v1.Workload) error {
	if name := v1.GetAnnotation(w, exportModelAnnotation); name != "" {
		model := &v1.Model{}
		if err := r.Get(ctx, client.ObjectKey{Name: name}, model); err != nil {
			return client.IgnoreNotFound(err)
		}
		return r.inheritModelConfig(ctx, run, model)
	}
	step := v1.GetLabel(w, v1.PostTrainingExportStepLabel)
	existing := &v1.ModelList{}
	if err := r.List(ctx, existing, client.MatchingLabels{v1.PostTrainingRunIdLabel: run.RunID,
		v1.PostTrainingExportStepLabel: step}); err != nil {
		return err
	}
	if len(existing.Items) > 0 {
		return r.markExport(ctx, w, existing.Items[0].Name)
	}
	var res *marker
	for _, pod := range commonworkload.PodsOf(ctx, r.db, w) {
		for _, c := range pod.Containers {
			if m := lastResult(c.Message, run.RunID); c.Name == SubmitterContainer && m != nil && m.Kind == markerSuccess {
				res = m
			}
		}
	}
	if res == nil {
		return nil
	}
	output := filepath.Clean(res.Body.Output)
	if !withinDir(output, parseRecipe(run).Output.Base) {
		klog.Warningf("export %s of post training run %s wrote outside the run directory: %s", w.Name, run.RunID, output)
		return nil
	}
	model := outputModel(run, output)
	model.Name = fmt.Sprintf("%s-s%s", run.RunID, step)
	model.Labels[v1.PostTrainingExportStepLabel] = step
	if name := exportName(w); name != "" {
		model.Spec.DisplayName = name
		delete(model.Labels, v1.DisplayNameLabel)
		if len(validation.IsValidLabelValue(name)) == 0 {
			model.Labels[v1.DisplayNameLabel] = name
		}
	}
	if err := r.Create(ctx, model); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	klog.Infof("registered model %s from step %s of post training run %s", model.Name, step, run.RunID)
	return r.markExport(ctx, w, model.Name)
}

// markExport records on an export job the Model registered from it.
func (r *RunReconciler) markExport(ctx context.Context, w *v1.Workload, model string) error {
	patch := client.MergeFrom(w.DeepCopy())
	v1.SetAnnotation(w, exportModelAnnotation, model)
	return client.IgnoreNotFound(r.Patch(ctx, w, patch))
}

// exportName returns the display name the export job's recipe gives the new model, or an empty string.
func exportName(w *v1.Workload) string {
	data, err := base64.StdEncoding.DecodeString(w.Spec.Env[exportRecipeEnv])
	if err != nil {
		return ""
	}
	var rec struct {
		ExportOnly struct {
			DisplayName string `json:"displayName"`
		} `json:"exportOnly"`
	}
	if json.Unmarshal(data, &rec) != nil {
		return ""
	}
	return rec.ExportOnly.DisplayName
}
