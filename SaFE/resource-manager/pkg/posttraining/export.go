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

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// exportRecipeEnv carries the export job's recipe, whose exportOnly part names the new model.
const exportRecipeEnv = "PT_RECIPE"

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

// registerExport creates the Model of a succeeded export job unless its step already has one. A job whose result
// marker is not stored yet is taken again when its pod row changes.
func (r *RunReconciler) registerExport(ctx context.Context, run *dbclient.PostTrainingRun, w *v1.Workload) error {
	step := v1.GetLabel(w, v1.PostTrainingExportStepLabel)
	existing := &v1.ModelList{}
	if err := r.List(ctx, existing, client.MatchingLabels{v1.PostTrainingRunIdLabel: run.RunID,
		v1.PostTrainingExportStepLabel: step}); err != nil || len(existing.Items) > 0 {
		return err
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
	model.Name = commonutils.GenerateName(fmt.Sprintf("%s-s%s", run.RunID, step))
	model.Labels[v1.PostTrainingExportStepLabel] = step
	if name := exportName(w); name != "" {
		model.Spec.DisplayName = name
		delete(model.Labels, v1.DisplayNameLabel)
		if len(validation.IsValidLabelValue(name)) == 0 {
			model.Labels[v1.DisplayNameLabel] = name
		}
	}
	if err := r.Create(ctx, model); err != nil {
		return err
	}
	klog.Infof("registered model %s from step %s of post training run %s", model.Name, step, run.RunID)
	return r.inheritModelConfig(ctx, run, model)
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
