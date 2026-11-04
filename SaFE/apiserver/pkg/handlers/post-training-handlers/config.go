/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// supportQuery selects the workspace, framework, train type and method a support verdict is for.
type supportQuery struct {
	Workspace string `form:"workspace"`
	ModelID   string `form:"modelId"`
	TrainType string `form:"trainType"`
	Method    string `form:"method"`
	Framework string `form:"framework"`
}

// modelSupport is the verdict for one model.
type modelSupport struct {
	ModelID     string `json:"modelId"`
	DisplayName string `json:"displayName"`
	Spec        string `json:"spec,omitempty"`
	Supported   bool   `json:"supported"`
	Reason      string `json:"reason,omitempty"`
}

// modelsResponse lists the ready local models of a workspace with their verdicts, and why the whole workspace is
// unavailable when it is.
type modelsResponse struct {
	Items             []modelSupport `json:"items"`
	UnavailableReason string         `json:"unavailableReason,omitempty"`
	PausedReason      string         `json:"pausedReason,omitempty"`
}

// frameworkItem is one selectable framework.
type frameworkItem struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// configResponse is the wizard configuration of one model.
type configResponse struct {
	Frameworks []frameworkItem `json:"frameworks"`
	Framework  string          `json:"framework"`
	Spec       string          `json:"spec,omitempty"`
	Supported  bool            `json:"supported"`
	Reason     string          `json:"reason,omitempty"`
	Options    *recipe.Options `json:"options,omitempty"`
}

// previewResponse is the compilation of a request without side effects.
type previewResponse struct {
	Diagnostics   []recipe.Diagnostic `json:"diagnostics"`
	Layout        *recipe.Layout      `json:"layout,omitempty"`
	FrameworkArgs []string            `json:"frameworkArgs,omitempty"`
	Recipe        *recipe.Recipe      `json:"recipe,omitempty"`
	// VerifiedRuns counts the runs of the same recipe hash that succeeded on the platform; LatestVerifiedRun is
	// the one that ended last.
	VerifiedRuns      int    `json:"verifiedRuns"`
	LatestVerifiedRun string `json:"latestVerifiedRun,omitempty"`
}

// ListModels returns, for every ready local model of a workspace, whether the selected framework, train type and
// method support it, plus the workspace-level reason when nothing can run there.
func (h *Handler) ListModels(c *gin.Context) {
	reg, regErr, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	var q supportQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid query: "+err.Error()))
		return
	}
	ctx := c.Request.Context()
	if err := h.authorize(c, v1.ListVerb, q.Workspace, "", "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	ws, err := h.getWorkspace(ctx, q.Workspace)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	resp := modelsResponse{Items: []modelSupport{}}
	if regErr != nil {
		resp.UnavailableReason = "Post Training registry is invalid: " + regErr.Error()
		c.JSON(http.StatusOK, resp)
		return
	}
	resp.UnavailableReason = workspaceUnavailable(reg, q.Framework, ws.GPUModel)
	if err := pausedError(reg); err != nil {
		resp.PausedReason = err.Error()
	}
	models := &v1.ModelList{}
	if err := h.client.List(ctx, models); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	for i := range models.Items {
		m := &models.Items[i]
		mi := modelInfo(m, q.Workspace)
		if !mi.Ready {
			continue
		}
		item := modelSupport{ModelID: m.Name, DisplayName: baseModelName(m)}
		if spec := reg.MatchSpec(m.Status.ModelConfig); spec != nil {
			item.Spec = spec.Name
		}
		in := &recipe.Input{Registry: reg, Model: mi, Workspace: ws.WorkspaceInfo,
			Request: &recipe.SubmitRequest{Framework: q.Framework, TrainType: q.TrainType, Method: q.Method}}
		item.Supported, item.Reason = recipe.Support(in)
		resp.Items = append(resp.Items, item)
	}
	sort.Slice(resp.Items, func(i, j int) bool { return resp.Items[i].DisplayName < resp.Items[j].DisplayName })
	c.JSON(http.StatusOK, resp)
}

// workspaceUnavailable explains why a framework cannot run in a workspace, or returns "" when it can.
func workspaceUnavailable(reg *registry.Registry, framework, gpuModel string) string {
	fw := reg.FrameworkByName(framework)
	if fw == nil {
		return fmt.Sprintf("Framework %s is not available", framework)
	}
	arch := reg.Arch(gpuModel)
	if arch == "" {
		return fmt.Sprintf("The workspace GPU %q has no known architecture", gpuModel)
	}
	if reg.DefaultImage(fw.Name, arch) == nil {
		return fmt.Sprintf("No validated %s image for workspace GPU architecture %s", fw.Name, arch)
	}
	return ""
}

// GetConfig returns the frameworks, the support verdict and the wizard options of one model.
func (h *Handler) GetConfig(c *gin.Context) {
	reg, regErr, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	var q supportQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid query: "+err.Error()))
		return
	}
	if regErr != nil {
		apiutils.AbortWithApiError(c, apierrors.NewServiceUnavailable("Post Training registry is invalid: "+regErr.Error()))
		return
	}
	ctx := c.Request.Context()
	if err := h.authorize(c, v1.ListVerb, q.Workspace, "", "", 0); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	ws, err := h.getWorkspace(ctx, q.Workspace)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	_, mi, err := h.getModel(ctx, q.ModelID, q.Workspace)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	resp := configResponse{}
	for _, f := range reg.Frameworks {
		resp.Frameworks = append(resp.Frameworks, frameworkItem{Name: f.Name, Default: f.Default})
		if (q.Framework == "" && f.Default) || f.Name == q.Framework {
			resp.Framework = f.Name
		}
	}
	if spec := reg.MatchSpec(mi.Config); spec != nil {
		resp.Spec = spec.Name
	}
	req := &recipe.SubmitRequest{Framework: q.Framework, TrainType: q.TrainType, Method: q.Method,
		Workspace: q.Workspace, ModelID: q.ModelID, Resources: recipe.Resources{Nodes: 1}}
	if ws.flavor != nil && ws.flavor.Spec.Gpu != nil {
		req.Resources.GpusPerNode = int(ws.flavor.Spec.Gpu.Quantity.Value())
	}
	in := &recipe.Input{Registry: reg, Model: mi, Workspace: ws.WorkspaceInfo, Request: req}
	resp.Supported, resp.Reason = recipe.Support(in)
	if resp.Supported && q.TrainType != "" {
		resp.Options, resp.Reason = recipe.FrameworkOptions(in)
		resp.Supported = resp.Options != nil
	}
	c.JSON(http.StatusOK, resp)
}

// PreviewRecipe compiles a request and returns its diagnostics, layout, framework arguments and recipe without
// recording anything.
func (h *Handler) PreviewRecipe(c *gin.Context) {
	reg, regErr, ok := h.requireEnabled(c)
	if !ok {
		return
	}
	var req recipe.SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	if regErr != nil {
		apiutils.AbortWithApiError(c, apierrors.NewServiceUnavailable("Post Training registry is invalid: "+regErr.Error()))
		return
	}
	if err := h.authorize(c, v1.CreateVerb, req.Workspace, "", "", req.Resources.Priority); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	in, _, _, err := h.buildInput(c, reg, &req, "preview")
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	res := recipe.Compile(in)
	resp := previewResponse{Diagnostics: res.Diagnostics}
	if resp.Diagnostics == nil {
		resp.Diagnostics = []recipe.Diagnostic{}
	}
	if res.Recipe != nil {
		resp.Layout, resp.FrameworkArgs, resp.Recipe = &res.Recipe.Layout, res.Recipe.FrameworkArgs, res.Recipe
		hash := res.Recipe.Provenance.RecipeHash
		if records, err := h.db.CountPostTrainingRunsByRecipe(c.Request.Context(), []string{hash}); err != nil {
			klog.ErrorS(err, "failed to count the successful runs of a recipe", "hash", hash)
		} else {
			resp.VerifiedRuns, resp.LatestVerifiedRun = records[hash].Succeeded, records[hash].LatestRun
		}
	}
	c.JSON(http.StatusOK, resp)
}
