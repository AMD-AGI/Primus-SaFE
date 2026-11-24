/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

const (
	originFineTuned = "fine_tuned"
	originRLTrained = "rl_trained"
)

// errModelTaken is returned when a Model of another owner has the name of the run's output Model.
var errModelTaken = errors.New("a model of another owner has the name of the output model")

// register turns a successful result into the run's output Model and finishes the run once the Model is Ready; a
// run that exports no model finishes at once, with nothing to register.
func (r *RunReconciler) register(ctx context.Context, u *runUpdate, res *marker, endTime time.Time) (ctrlruntime.Result, error) {
	run := u.run
	rec := parseRecipe(run)
	if res.Body.Output == "" && rec.Output.ExportHF != nil && !*rec.Output.ExportHF {
		u.end(v1.WorkloadSucceeded, endTime)
		u.moveTo("done")
		return ctrlruntime.Result{}, r.save(ctx, u)
	}
	output, base := filepath.Clean(res.Body.Output), rec.Output.Base
	if !withinDir(output, base) {
		u.fail("register", fmt.Sprintf("The output path %q is not under the run directory", res.Body.Output), endTime)
		return ctrlruntime.Result{}, r.save(ctx, u)
	}
	u.moveTo("register")
	start := u.stageStart("register")
	timedOut := !start.IsZero() && u.now.Sub(start) > registerTimeout
	model, err := r.ensureModel(ctx, run, output)
	if err != nil {
		// A Model the API server rejects is rejected again on every retry, and one that keeps failing otherwise
		// must not keep the run in register for ever.
		switch {
		case permanentModelError(err):
			u.fail("register", fmt.Sprintf("The output model could not be registered: %v", err), endTime)
			return ctrlruntime.Result{}, r.save(ctx, u)
		case timedOut:
			u.fail("register", fmt.Sprintf("The output model was not registered within 30 minutes: %v", err), endTime)
			return ctrlruntime.Result{}, r.save(ctx, u)
		}
		if serr := r.save(ctx, u); serr != nil {
			return ctrlruntime.Result{}, serr
		}
		return ctrlruntime.Result{}, err
	}
	u.set("output_model_id", run.OutputModelID, model.Name)
	if !model.IsReady() {
		if timedOut {
			u.fail("register", "The output model did not become ready within 30 minutes", endTime)
			return ctrlruntime.Result{}, r.save(ctx, u)
		}
		return ctrlruntime.Result{RequeueAfter: runningRequeue}, r.save(ctx, u)
	}
	u.end(v1.WorkloadSucceeded, endTime)
	u.moveTo("done")
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

// permanentModelError reports whether creating or reading the output Model failed in a way a retry repeats.
func permanentModelError(err error) bool {
	return apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsBadRequest(err) ||
		errors.Is(err, errModelTaken)
}

// ensureModel returns the run's output Model, creating it on first use. The Model is named after the run, so a
// reconcile that runs before the cache shows the Model a previous one created finds it there instead of creating
// a second one. The base model's structure summary is copied once the new Model is visible.
func (r *RunReconciler) ensureModel(ctx context.Context, run *dbclient.PostTrainingRun, output string) (*v1.Model, error) {
	key := client.ObjectKey{Name: outputModelName(run)}
	model := &v1.Model{}
	err := r.Get(ctx, key, model)
	if apierrors.IsNotFound(err) {
		model = outputModel(run, output)
		if err = r.Create(ctx, model); !apierrors.IsAlreadyExists(err) {
			return model, err
		}
		model = &v1.Model{}
		err = r.reader.Get(ctx, key, model)
	}
	if err != nil {
		return nil, err
	}
	if v1.GetLabel(model, v1.PostTrainingRunIdLabel) != run.RunID {
		return nil, fmt.Errorf("%w: %s", errModelTaken, key.Name)
	}
	return model, r.inheritModelConfig(ctx, run, model)
}

// outputModelName returns the name of the run's output Model: the run id, which is unique, at most 63 characters
// and made of lower case letters, digits and hyphens, so it is a valid Model name.
func outputModelName(run *dbclient.PostTrainingRun) string {
	return run.RunID
}

// outputModel builds the local_path Model that registers the run's output.
func outputModel(run *dbclient.PostTrainingRun, output string) *v1.Model {
	origin := originFineTuned
	if run.TrainType == registry.TrainTypeRL {
		origin = originRLTrained
	}
	m := &v1.Model{}
	m.Name = outputModelName(run)
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
