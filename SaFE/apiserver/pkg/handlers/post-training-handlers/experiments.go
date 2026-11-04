/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	// maxVariants is how many variants an experiment adds to its baseline, five runs in all like the compare page.
	maxVariants = 4
	// baselineVariant names the baseline of every experiment.
	baselineVariant = "baseline"
)

var variantNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// experimentRequest submits an A/B experiment: a baseline request and the variants that change some of its fields.
type experimentRequest struct {
	DisplayName string               `json:"displayName"`
	Baseline    recipe.SubmitRequest `json:"baseline"`
	Variants    []experimentVariant  `json:"variants"`
}

// experimentVariant is one variant: its name and the request fields it changes, as a JSON object merged into the
// baseline request.
type experimentVariant struct {
	Name    string                 `json:"name"`
	Changes map[string]interface{} `json:"changes"`
}

// variantDiagnostic is a compile finding of one variant.
type variantDiagnostic struct {
	Variant string `json:"variant"`
	recipe.Diagnostic
}

// experimentView is an experiment with its variants as JSON and its runs.
type experimentView struct {
	*dbclient.PostTrainingExperiment
	Variants json.RawMessage             `json:"variants"`
	Runs     []*dbclient.PostTrainingRun `json:"runs"`
}

// mergeChanges merges a JSON object into another: nested objects merge, other values replace, null removes.
func mergeChanges(dst, changes map[string]interface{}) {
	for k, v := range changes {
		if v == nil {
			delete(dst, k)
			continue
		}
		if sub, ok := v.(map[string]interface{}); ok {
			if cur, ok := dst[k].(map[string]interface{}); ok {
				mergeChanges(cur, sub)
				continue
			}
		}
		dst[k] = v
	}
}

// variantRequests returns the request of every variant, baseline first, with their names: the baseline request with
// the variant's changes merged in, named <experiment>-<variant>, in the baseline's workspace and train type, with
// the experiment's seed and the baseline's evaluation, which is on whenever it names benchmarks.
func variantRequests(req *experimentRequest, seed int) ([]*recipe.SubmitRequest, []string, error) {
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		return nil, nil, commonerrors.NewBadRequest("displayName is required")
	}
	if n := len(req.Variants); n < 1 || n > maxVariants {
		return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("an experiment has 1 to %d variants", maxVariants))
	}
	base := req.Baseline
	evaluation := base.Evaluation
	evaluation.Enabled = len(evaluation.Benchmarks) > 0
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return nil, nil, commonerrors.NewBadRequest("invalid baseline: " + err.Error())
	}
	names := []string{baselineVariant}
	all := append([]experimentVariant{{Name: baselineVariant}}, req.Variants...)
	out := make([]*recipe.SubmitRequest, 0, len(all))
	for i, v := range all {
		if i > 0 {
			if !variantNamePattern.MatchString(v.Name) {
				return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("variant name %q must be lowercase letters, "+
					"digits and dashes, at most 32 characters", v.Name))
			}
			for _, seen := range names {
				if seen == v.Name {
					return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("variant %s appears twice", v.Name))
				}
			}
			names = append(names, v.Name)
		}
		fields := map[string]interface{}{}
		if err = json.Unmarshal(baseJSON, &fields); err != nil {
			return nil, nil, commonerrors.NewInternalError(err.Error())
		}
		mergeChanges(fields, v.Changes)
		data, _ := json.Marshal(fields)
		r := &recipe.SubmitRequest{}
		if err = json.Unmarshal(data, r); err != nil {
			return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("variant %s: %v", v.Name, err))
		}
		if r.TrainType != base.TrainType {
			return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("variant %s changes the train type; "+
				"every variant of an experiment has the baseline's", v.Name))
		}
		r.Workspace, r.DisplayName, r.Seed, r.Evaluation = base.Workspace, name+"-"+v.Name, seed, evaluation
		out = append(out, r)
	}
	return out, names, nil
}

// SubmitExperiment compiles every variant of an A/B experiment and, when none has an error, records the experiment
// and submits one run per variant.
func (h *Handler) SubmitExperiment(c *gin.Context) {
	reg, regErr, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	var req experimentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	if regErr != nil {
		apiutils.AbortWithApiError(c, apierrors.NewServiceUnavailable("Post Training registry is invalid: "+regErr.Error()))
		return
	}
	if err := pausedError(reg); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	seed := rand.Intn(1<<30) + 1
	reqs, names, err := variantRequests(&req, seed)
	if err == nil {
		err = h.authorize(c, v1.CreateVerb, req.Baseline.Workspace, "", "", req.Baseline.Resources.Priority)
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	var diags []variantDiagnostic
	for i, r := range reqs {
		in, _, _, err := h.buildInput(c, reg, r, "preview")
		if err != nil {
			apiutils.AbortWithApiError(c, err)
			return
		}
		for _, d := range recipe.Compile(in).Diagnostics {
			if d.Level == recipe.LevelError {
				diags = append(diags, variantDiagnostic{Variant: names[i], Diagnostic: d})
			}
		}
	}
	if len(diags) > 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"errorMessage": fmt.Sprintf("variant %s does not compile", diags[0].Variant), "diagnostics": diags})
		return
	}
	variants, _ := json.Marshal(req.Variants)
	exp := &dbclient.PostTrainingExperiment{ExperimentID: commonutils.GenerateName("exp"),
		Workspace: req.Baseline.Workspace, DisplayName: strings.TrimSpace(req.DisplayName),
		TrainType: req.Baseline.TrainType, BaselineVariant: baselineVariant, Variants: string(variants), Seed: seed,
		UserID: c.GetString(common.UserId), UserName: c.GetString(common.UserName)}
	if err = h.db.CreatePostTrainingExperiment(c.Request.Context(), exp); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to record the experiment: "+err.Error()))
		return
	}
	runIDs := make([]string, 0, len(reqs))
	for i, r := range reqs {
		r.ExperimentID, r.VariantName = exp.ExperimentID, names[i]
		resp, _, err := h.submit(c, reg, r)
		if err == nil && resp == nil {
			err = commonerrors.NewInternalError(fmt.Sprintf("variant %s no longer compiles", names[i]))
		}
		if err != nil {
			klog.ErrorS(err, "failed to submit a variant", "experiment", exp.ExperimentID, "variant", names[i])
			apiutils.AbortWithApiError(c, commonerrors.NewInternalError(fmt.Sprintf(
				"experiment %s: variant %s failed to submit after runs %v: %v", exp.ExperimentID, names[i], runIDs, err)))
			return
		}
		runIDs = append(runIDs, resp.RunID)
	}
	klog.Infof("submit post training experiment %s with runs %v by user %s", exp.ExperimentID, runIDs,
		c.GetString(common.UserName))
	c.JSON(http.StatusOK, gin.H{"experimentId": exp.ExperimentID, "runIds": runIDs})
}

// experimentRuns returns the runs of an experiment without their recipes, in variant order.
func (h *Handler) experimentRuns(c *gin.Context, exp *dbclient.PostTrainingExperiment) ([]*dbclient.PostTrainingRun, error) {
	runs, _, err := h.db.ListPostTrainingRuns(c.Request.Context(), dbclient.PostTrainingRunFilter{
		ExperimentID: exp.ExperimentID, Limit: maxVariants + 1})
	if err != nil {
		return nil, commonerrors.NewInternalError("failed to list the experiment's runs: " + err.Error())
	}
	order := map[string]int{exp.BaselineVariant: 0}
	var variants []experimentVariant
	_ = json.Unmarshal([]byte(exp.Variants), &variants)
	for i, v := range variants {
		order[v.Name] = i + 1
	}
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0 && order[runs[j].VariantName] < order[runs[j-1].VariantName]; j-- {
			runs[j], runs[j-1] = runs[j-1], runs[j]
		}
	}
	for _, r := range runs {
		r.Recipe = ""
	}
	return runs, nil
}

// ListExperiments returns a workspace's experiments with their runs, newest first.
func (h *Handler) ListExperiments(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	workspace := c.Query("workspace")
	if workspace == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("workspace is required"))
		return
	}
	if err := h.authorize(c, v1.ListVerb, workspace, "", "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	var ids []string
	if raw := c.Query("experimentIds"); raw != "" {
		ids = strings.Split(raw, ",")
	}
	exps, total, err := h.db.ListPostTrainingExperiments(c.Request.Context(), dbclient.PostTrainingExperimentFilter{
		Workspace: workspace, ExperimentIDs: ids, Limit: limit, Offset: offset})
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to list experiments: "+err.Error()))
		return
	}
	items := make([]experimentView, 0, len(exps))
	for _, e := range exps {
		runs, err := h.experimentRuns(c, e)
		if err != nil {
			apiutils.AbortWithApiError(c, err)
			return
		}
		items = append(items, experimentView{PostTrainingExperiment: e, Variants: rawJSON(e.Variants), Runs: runs})
	}
	c.JSON(http.StatusOK, gin.H{"total": total, "items": items})
}

// GetExperiment returns an experiment with its variants and runs.
func (h *Handler) GetExperiment(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	exp, err := h.getExperiment(c, v1.GetVerb)
	var runs []*dbclient.PostTrainingRun
	if err == nil {
		runs, err = h.experimentRuns(c, exp)
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, experimentView{PostTrainingExperiment: exp, Variants: rawJSON(exp.Variants), Runs: runs})
}

// StopExperiment stops every run of an experiment that has not finished.
func (h *Handler) StopExperiment(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	exp, err := h.getExperiment(c, v1.DeleteVerb)
	var runs []*dbclient.PostTrainingRun
	if err == nil {
		runs, err = h.experimentRuns(c, exp)
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	message := fmt.Sprintf("the experiment is stopped by %s", c.GetString(common.UserName))
	stopped := []string{}
	for _, r := range runs {
		switch v1.WorkloadPhase(r.Status) {
		case v1.WorkloadSucceeded, v1.WorkloadFailed, v1.WorkloadStopped:
			continue
		}
		if err = h.stopWorkload(c.Request.Context(), r.WorkloadID, message); err != nil {
			apiutils.AbortWithApiError(c, err)
			return
		}
		stopped = append(stopped, r.RunID)
	}
	klog.Infof("stop post training experiment %s by user %s", exp.ExperimentID, c.GetString(common.UserName))
	c.JSON(http.StatusOK, gin.H{"experimentId": exp.ExperimentID, "stopped": stopped})
}

// DeleteExperiment deletes the experiment's runs, with their output models when deleteModel is true, and then the
// experiment; runs that fail to delete keep the experiment and are returned.
func (h *Handler) DeleteExperiment(c *gin.Context) {
	if _, _, ok := h.requireEnabled(c); !ok {
		return
	}
	exp, err := h.getExperiment(c, v1.DeleteVerb)
	var runs []*dbclient.PostTrainingRun
	if err == nil {
		runs, err = h.experimentRuns(c, exp)
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	failed := map[string]string{}
	for _, r := range runs {
		if err = h.deleteRun(c, r, c.Query("deleteModel") == "true"); err != nil {
			failed[r.RunID] = err.Error()
		}
	}
	if len(failed) > 0 {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"errorMessage": fmt.Sprintf("%d runs of the experiment failed to delete", len(failed)), "failed": failed})
		return
	}
	if err = h.db.SetPostTrainingExperimentDeleted(c.Request.Context(), exp.ExperimentID); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to delete the experiment: "+err.Error()))
		return
	}
	klog.Infof("delete post training experiment %s by user %s", exp.ExperimentID, c.GetString(common.UserName))
	c.JSON(http.StatusOK, gin.H{"experimentId": exp.ExperimentID})
}

// getExperiment loads the experiment of the :id parameter and checks the caller may act on it with verb.
func (h *Handler) getExperiment(c *gin.Context, verb v1.RoleVerb) (*dbclient.PostTrainingExperiment, error) {
	exp, err := h.db.GetPostTrainingExperiment(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("experiment %s not found", c.Param("id")))
		}
		return nil, commonerrors.NewInternalError("failed to read the experiment: " + err.Error())
	}
	if err = h.authorize(c, verb, exp.Workspace, exp.UserID, exp.ExperimentID, 0); err != nil {
		return nil, err
	}
	return exp, nil
}
