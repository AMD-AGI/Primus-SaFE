/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	originFineTuned = "fine_tuned"
	originRLTrained = "rl_trained"
)

// register turns a successful result into the run's output Model and finishes the run once the Model is Ready.
func (r *RunReconciler) register(ctx context.Context, u *runUpdate, res *marker, endTime time.Time) (ctrlruntime.Result, error) {
	run := u.run
	output := filepath.Clean(res.Body.Output)
	if !withinDir(output, parseRecipe(run).Output.Base) {
		u.fail("register", fmt.Sprintf("The output path %q is not under the run directory", res.Body.Output), endTime)
		return ctrlruntime.Result{}, r.save(ctx, u)
	}
	u.moveTo("register")
	model, err := r.ensureModel(ctx, run, output)
	if err != nil {
		if serr := r.save(ctx, u); serr != nil {
			return ctrlruntime.Result{}, serr
		}
		return ctrlruntime.Result{}, err
	}
	u.set("output_model_id", run.OutputModelID, model.Name)
	if !model.IsReady() {
		return ctrlruntime.Result{RequeueAfter: runningRequeue}, r.save(ctx, u)
	}
	u.moveTo("done")
	u.end(v1.WorkloadSucceeded, endTime)
	return ctrlruntime.Result{}, r.save(ctx, u)
}

// withinDir reports whether path lies strictly inside dir.
func withinDir(path, dir string) bool {
	if dir == "" || !filepath.IsAbs(path) {
		return false
	}
	dir = filepath.Clean(dir)
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

// ensureModel returns the run's output Model, creating it on first use; a Model found by the run label is
// reused, so a retried registration never creates a second one. The base model's structure summary is copied
// once the new Model is visible.
func (r *RunReconciler) ensureModel(ctx context.Context, run *dbclient.PostTrainingRun, output string) (*v1.Model, error) {
	list := &v1.ModelList{}
	if err := r.List(ctx, list, client.MatchingLabels{v1.PostTrainingRunIdLabel: run.RunID}); err != nil {
		return nil, err
	}
	for i := range list.Items {
		if model := &list.Items[i]; !v1.HasLabel(model, v1.PostTrainingExportStepLabel) {
			return model, r.inheritModelConfig(ctx, run, model)
		}
	}
	model := outputModel(run, output)
	return model, r.Create(ctx, model)
}

// outputModel builds the local_path Model that registers the run's output.
func outputModel(run *dbclient.PostTrainingRun, output string) *v1.Model {
	origin := originFineTuned
	if run.TrainType == registry.TrainTypeRL {
		origin = originRLTrained
	}
	m := &v1.Model{}
	m.Name = commonutils.GenerateName(run.RunID)
	m.Labels = map[string]string{v1.PostTrainingRunIdLabel: run.RunID}
	for key, value := range map[string]string{v1.DisplayNameLabel: run.DisplayName, v1.UserIdLabel: run.UserID} {
		if value != "" && len(validation.IsValidLabelValue(value)) == 0 {
			m.Labels[key] = value
		}
	}
	if run.UserName != "" {
		m.Annotations = map[string]string{v1.UserNameAnnotation: run.UserName}
	}
	m.Spec = v1.ModelSpec{
		DisplayName:       run.DisplayName,
		Source:            v1.ModelSource{AccessMode: v1.AccessModeLocalPath, LocalPath: output},
		Workspace:         run.Workspace,
		Origin:            origin,
		BaseModel:         run.BaseModelName,
		PostTrainingRunId: run.RunID,
	}
	return m
}

// inheritModelConfig copies the base model's structure summary to the new Model's status.
func (r *RunReconciler) inheritModelConfig(ctx context.Context, run *dbclient.PostTrainingRun, model *v1.Model) error {
	if model.Status.ModelConfig != nil {
		return nil
	}
	base := &v1.Model{}
	if err := r.Get(ctx, client.ObjectKey{Name: run.BaseModelID}, base); err != nil || base.Status.ModelConfig == nil {
		return client.IgnoreNotFound(err)
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, client.ObjectKeyFromObject(model), model); err != nil {
			return err
		}
		if model.Status.ModelConfig != nil {
			return nil
		}
		model.Status.ModelConfig = base.Status.ModelConfig.DeepCopy()
		return r.Status().Update(ctx, model)
	})
}
