/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	// launcherEntrypoint runs the launcher the preprocess init container copies into /shared-data.
	launcherEntrypoint = "python3 /shared-data/safe_post_training/launch.py"
	// recipeEnv carries the base64 recipe JSON to the launcher.
	recipeEnv = "PT_RECIPE"
	// runTTLSeconds keeps a finished workload long enough for the controller to read its result after a restart.
	runTTLSeconds = 86400
	// defaultMaxRetry is the retry count when the request does not set one.
	defaultMaxRetry = 1
	maxRetryLimit   = 3
	maxRunSlugLen   = 30
	maxExtBytes     = 16 * 1024
	rdmaResource    = "1k"
	// nodeInitScript clears stale aiter JIT locks, which hang SGLang after a pod restart, and lets every Python
	// process on the node, Ray workers included, import the launcher's slime plugins by name: a .pth file adds a
	// directory that holds only the safe_post_training package.
	nodeInitScript = "#!/bin/bash\nset -e\n" +
		"find /sgl-workspace/aiter/aiter/jit/build -name 'lock*' -exec rm -f {} \\; 2>/dev/null || true\n" +
		"{ mkdir -p /shared-data/pythonpath && ln -sfn /shared-data/safe_post_training " +
		"/shared-data/pythonpath/safe_post_training; } || true\n" +
		"python3 -c 'import os,site; d=[p for p in site.getsitepackages() if os.access(p, os.W_OK)]; " +
		"d and open(os.path.join(d[0], \"safe_post_training.pth\"), \"w\").write(\"/shared-data/pythonpath\\n\")' " +
		"|| true\n"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// submitResponse is the body returned for a new run.
type submitResponse struct {
	RunID      string              `json:"runId"`
	WorkloadID string              `json:"workloadId"`
	Warnings   []recipe.Diagnostic `json:"warnings,omitempty"`
}

// diagnosticsResponse is returned when a request does not compile.
type diagnosticsResponse struct {
	ErrorMessage string              `json:"errorMessage"`
	Diagnostics  []recipe.Diagnostic `json:"diagnostics"`
}

// SubmitRun compiles a request again on the server, records the run and creates its RayJob.
func (h *Handler) SubmitRun(c *gin.Context) {
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
	if err := pausedError(reg); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	resp, diags, err := h.submit(c, reg, &req)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	if resp == nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, diagnosticsResponse{
			ErrorMessage: "the request does not compile", Diagnostics: diags})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// submit runs the whole submission; it returns diagnostics without a response when the request does not compile.
func (h *Handler) submit(c *gin.Context, reg *registry.Registry,
	req *recipe.SubmitRequest) (*submitResponse, []recipe.Diagnostic, error) {
	ctx := c.Request.Context()
	if strings.TrimSpace(req.DisplayName) == "" {
		return nil, nil, commonerrors.NewBadRequest("displayName is required")
	}
	if r := req.Resources.MaxRetry; r != nil && (*r < 0 || *r > maxRetryLimit) {
		return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("maxRetry must be between 0 and %d", maxRetryLimit))
	}
	if b, _ := json.Marshal(req.Ext); len(req.Ext) > 0 && len(b) > maxExtBytes {
		return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("ext must not exceed %d bytes", maxExtBytes))
	}
	if b, err := json.Marshal(req.Ext); err != nil || len(b) > maxExtBytes {
		return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("ext must be JSON of at most %d bytes", maxExtBytes))
	}
	if err := h.authorize(c, v1.CreateVerb, req.Workspace, "", "", req.Resources.Priority); err != nil {
		return nil, nil, err
	}
	runID := newRunID(req.DisplayName)
	in, ws, model, err := h.buildInput(c, reg, req, runID)
	if err != nil {
		return nil, nil, err
	}
	res := recipe.Compile(in)
	if res.HasErrors() {
		return nil, res.Diagnostics, nil
	}
	if cv := res.Recipe.Convert; cv != nil && cv.CacheKey != "" {
		cache, err := h.db.GetPostTrainingConvertCache(ctx, req.Workspace, cv.CacheKey)
		cv.CacheHit = err == nil && cache.Status == dbclient.ConvertCacheReady
	}
	if req.TemplateID != "" {
		tpl, err := h.db.GetPostTrainingTemplate(ctx, req.TemplateID)
		if err != nil || tpl.Workspace != req.Workspace {
			return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("template %s not found in workspace %s",
				req.TemplateID, req.Workspace))
		}
		res.Recipe.Template = &recipe.RecipeTemplate{ID: tpl.TemplateID, Version: tpl.Version}
	}
	recipeJSON, err := json.Marshal(res.Recipe)
	if err != nil {
		return nil, nil, commonerrors.NewInternalError("failed to encode the recipe: " + err.Error())
	}
	workload, err := buildWorkload(c, reg, req, res, ws, runID, recipeJSON)
	if err != nil {
		return nil, nil, err
	}
	row := runRow(c, req, res, ws, model, runID, string(recipeJSON))
	if err := h.db.CreatePostTrainingRun(ctx, row); err != nil {
		return nil, nil, commonerrors.NewInternalError("failed to record the run: " + err.Error())
	}
	if err := h.client.Create(ctx, workload); err != nil {
		klog.ErrorS(err, "failed to create the Post Training workload", "run", runID)
		if uerr := h.db.UpdatePostTrainingRun(ctx, runID, map[string]interface{}{
			"status": string(v1.WorkloadFailed), "message": "failed to create the workload: " + err.Error(),
		}); uerr != nil {
			klog.ErrorS(uerr, "failed to mark the run failed", "run", runID)
		}
		return nil, nil, err
	}
	return &submitResponse{RunID: runID, WorkloadID: workload.Name, Warnings: res.Diagnostics}, nil, nil
}

// buildInput gathers what the compiler needs about the workspace, model, datasets and secrets of a request.
func (h *Handler) buildInput(c *gin.Context, reg *registry.Registry, req *recipe.SubmitRequest,
	runID string) (*recipe.Input, *workspaceInfo, *v1.Model, error) {
	ctx := c.Request.Context()
	ws, err := h.getWorkspace(ctx, req.Workspace)
	if err != nil {
		return nil, nil, nil, err
	}
	model, mi, err := h.getModel(ctx, req.ModelID, req.Workspace)
	if err != nil {
		return nil, nil, nil, err
	}
	dataset, err := h.getDataset(ctx, req.DatasetID, req.Workspace)
	if err != nil {
		return nil, nil, nil, err
	}
	evalDataset, err := h.getDataset(ctx, req.EvalDatasetID, req.Workspace)
	if err != nil {
		return nil, nil, nil, err
	}
	secrets, err := h.readSecrets(c, req)
	if err != nil {
		return nil, nil, nil, err
	}
	return &recipe.Input{Registry: reg, Request: req, RunID: runID, RunDir: ws.runDir(runID), Model: mi,
		Dataset: dataset, EvalDataset: evalDataset, Workspace: ws.WorkspaceInfo, Secrets: secrets}, ws, model, nil
}

// readSecrets returns the type and key names of the secrets a request names that the requesting user can read.
func (h *Handler) readSecrets(c *gin.Context, req *recipe.SubmitRequest) (map[string]*recipe.SecretInfo, error) {
	out := map[string]*recipe.SecretInfo{}
	var ids []string
	if w := req.Tracking.Wandb; w.Enabled && w.SecretID != "" {
		ids = append(ids, w.SecretID)
	}
	if id := rewardSecret(req); id != "" {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	ctx := c.Request.Context()
	user, err := h.access.GetRequestUser(ctx, c.GetString(common.UserId))
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		secret := &corev1.Secret{}
		if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: id, Namespace: common.PrimusSafeNamespace},
			secret); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to read secret %s: %v", id, err))
		}
		if h.access.Authorize(authority.AccessInput{Context: ctx, Resource: secret,
			ResourceKind: authority.SecretResourceKind, Verb: v1.GetVerb, User: user,
			Workspaces: []string{req.Workspace}}) != nil {
			continue
		}
		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out[id] = &recipe.SecretInfo{Type: v1.GetSecretType(secret), Keys: keys}
	}
	return out, nil
}

// pausedError is the 503 returned for new submissions while the registry pauses them.
func pausedError(reg *registry.Registry) error {
	if !reg.Paused.Enabled {
		return nil
	}
	reason := reg.Paused.Reason
	if reason == "" {
		reason = "new Post Training runs are paused"
	}
	return apierrors.NewServiceUnavailable(reason)
}

// newRunID builds a run id from the display name, short enough for the names KubeRay derives from it.
func newRunID(displayName string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(displayName), "-"), "-")
	if len(slug) > maxRunSlugLen {
		slug = strings.Trim(slug[:maxRunSlugLen], "-")
	}
	if slug == "" {
		slug = "run"
	}
	return commonutils.GenerateName("pt-" + slug)
}

// buildWorkload creates the RayJob of a run: the launcher entry point, the recipe, the log and network environment
// and whole-node resources from the workspace's node flavor.
func buildWorkload(c *gin.Context, reg *registry.Registry, req *recipe.SubmitRequest, res *recipe.Result,
	ws *workspaceInfo, runID string, recipeJSON []byte) (*v1.Workload, error) {
	if ws.flavor == nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("workspace %s has no node flavor", ws.ID))
	}
	nodes := req.Resources.Nodes
	env := map[string]string{
		common.RayJobEntrypoint: base64.StdEncoding.EncodeToString([]byte(launcherEntrypoint)),
		recipeEnv:               base64.StdEncoding.EncodeToString(recipeJSON),
		"PYTHONUNBUFFERED":      "1",
		"NCCL_DEBUG":            "WARN",
		"RAY_DEDUP_LOGS":        "0",
		"TENSORBOARD_DIR":       res.Recipe.Output.Base + "/tensorboard",
	}
	for k, v := range res.Image.Env {
		env[k] = v
	}
	for k, v := range res.Plan.Env {
		env[k] = v
	}
	if nodes > 1 && reg.Network.AINIC != nil {
		for k, v := range reg.Network.AINIC.Env {
			env[k] = v
		}
	}
	if level := strings.ToUpper(env["NCCL_DEBUG"]); level == "INFO" || level == "TRACE" {
		env["NCCL_DEBUG_FILE"] = res.Recipe.Output.Base + "/logs/nccl/%h-%p.log"
	}

	node := nodeResource(ws.flavor, req.Resources.GpusPerNode)
	if nodes > 1 {
		node.RdmaResource = rdmaResource
	}
	head := node
	head.Replica = 1
	init := base64.StdEncoding.EncodeToString([]byte(nodeInitScript))
	images, entryPoints, resources := []string{res.Image.Image}, []string{init}, []v1.WorkloadResource{head}
	if nodes > 1 {
		worker := node
		worker.Replica = nodes - 1
		images, entryPoints = append(images, res.Image.Image), append(entryPoints, init)
		resources = append(resources, worker)
	}
	maxRetry := defaultMaxRetry
	if req.Resources.MaxRetry != nil {
		maxRetry = *req.Resources.MaxRetry
	}
	ttl := runTTLSeconds
	w := &v1.Workload{}
	w.Name = runID
	w.Labels = map[string]string{
		v1.DisplayNameLabel:       req.DisplayName,
		v1.UserIdLabel:            c.GetString(common.UserId),
		v1.PostTrainingRunIdLabel: runID,
		v1.PostTrainingRoleLabel:  v1.PostTrainingRoleTrain,
	}
	w.Annotations = map[string]string{v1.UserNameAnnotation: c.GetString(common.UserName)}
	w.Spec = v1.WorkloadSpec{
		GroupVersionKind:        v1.GroupVersionKind{Kind: common.RayJobKind, Version: common.DefaultVersion},
		Images:                  images,
		EntryPoints:             entryPoints,
		Resources:               resources,
		Env:                     env,
		Priority:                req.Resources.Priority,
		Workspace:               req.Workspace,
		MaxRetry:                maxRetry,
		TTLSecondsAfterFinished: &ttl,
	}
	if t := req.Resources.TimeoutSeconds; t > 0 {
		w.Spec.Timeout = &t
	}
	if wb := req.Tracking.Wandb; wb.Enabled {
		w.Spec.Secrets = append(w.Spec.Secrets, v1.SecretEntity{Id: wb.SecretID, Type: v1.SecretGeneral, AsEnv: true})
	}
	if id := rewardSecret(req); id != "" {
		w.Spec.Secrets = append(w.Spec.Secrets, v1.SecretEntity{Id: id, Type: v1.SecretGeneral, AsEnv: true})
	}
	return w, nil
}

// rewardSecret returns the secret of an RL request's remote reward service, or an empty string.
func rewardSecret(req *recipe.SubmitRequest) string {
	remote := req.Choices.RewardRemote
	if req.TrainType != registry.TrainTypeRL || req.Choices.RewardType != "remote" || remote == nil {
		return ""
	}
	return remote.SecretID
}

// nodeResource requests a whole node: the flavor's allocatable CPU, memory and ephemeral storage after the
// workspace reserves, half the memory as shared memory, and the requested GPUs.
func nodeResource(nf *v1.NodeFlavor, gpus int) v1.WorkloadResource {
	avail := quantity.GetAvailableResource(nf.ToResourceList(""))
	r := v1.WorkloadResource{GPU: strconv.Itoa(gpus)}
	if q, ok := avail[corev1.ResourceCPU]; ok {
		r.CPU = q.String()
	}
	if q, ok := avail[corev1.ResourceMemory]; ok {
		r.Memory = q.String()
		r.SharedMemory = resource.NewQuantity(q.Value()/2, resource.BinarySI).String()
	}
	if q, ok := avail[corev1.ResourceEphemeralStorage]; ok {
		r.EphemeralStorage = q.String()
	}
	return r
}

// runRow is the post_training_run row of a new run.
func runRow(c *gin.Context, req *recipe.SubmitRequest, res *recipe.Result, ws *workspaceInfo, model *v1.Model,
	runID, recipeJSON string) *dbclient.PostTrainingRun {
	ext := ""
	if len(req.Ext) > 0 {
		if b, err := json.Marshal(req.Ext); err == nil {
			ext = string(b)
		}
	}
	history, _ := json.Marshal([]map[string]string{{"stage": "queued", "time": time.Now().UTC().Format(time.RFC3339)}})
	r := res.Recipe
	return &dbclient.PostTrainingRun{
		RunID:            runID,
		WorkloadID:       runID,
		DisplayName:      req.DisplayName,
		Framework:        r.Framework,
		FrameworkVersion: r.FrameworkVersion,
		TrainType:        r.TrainType,
		Method:           r.Method,
		Algorithm:        r.Algorithm,
		Workspace:        req.Workspace,
		Cluster:          ws.Cluster,
		UserID:           c.GetString(common.UserId),
		UserName:         c.GetString(common.UserName),
		BaseModelID:      model.Name,
		BaseModelName:    baseModelName(model),
		DatasetID:        req.DatasetID,
		EvalDatasetID:    req.EvalDatasetID,
		Image:            res.Image.Image,
		NodeCount:        r.Layout.Nodes,
		GpuPerNode:       r.Layout.GpusPerNode,
		Priority:         req.Resources.Priority,
		Timeout:          req.Resources.TimeoutSeconds,
		OutputPath:       r.Output.Base,
		Status:           string(v1.WorkloadPending),
		Stage:            "queued",
		StageHistory:     string(history),
		Recipe:           recipeJSON,
		RecipeHash:       r.Provenance.RecipeHash,
		EvalEnabled:      req.Evaluation.Enabled,
		TemplateID:       req.TemplateID,
		ExperimentID:     req.ExperimentID,
		VariantName:      req.VariantName,
		Ext:              ext,
	}
}

// baseModelName is the name shown for the base model: the display name, or the source model name.
func baseModelName(m *v1.Model) string {
	if m.Spec.DisplayName != "" {
		return m.Spec.DisplayName
	}
	if m.Spec.Source.ModelName != "" {
		return m.Spec.Source.ModelName
	}
	return m.Name
}
