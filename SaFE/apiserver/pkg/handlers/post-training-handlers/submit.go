/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	commonqos "github.com/AMD-AIG-AIMA/SAFE/common/pkg/qos"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
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
	// maxRequestBytes bounds the body of a preview or submission: ext and frameworkOptions take 16 KiB each, and
	// the other free-form fields far less.
	maxRequestBytes = 256 << 10
	// maxDisplayNameLen is what the display name column of a run holds.
	maxDisplayNameLen = 128
	rdmaResource      = "1k"
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
	if err := bindRequest(c, &req); err != nil {
		apiutils.AbortWithApiError(c, err)
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

// bindRequest reads the body of a preview or submission, which may be at most maxRequestBytes.
func bindRequest(c *gin.Context, req *recipe.SubmitRequest) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
	if err := c.ShouldBindJSON(req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return commonerrors.NewRequestEntityTooLargeError(fmt.Sprintf("the request body is larger than %d bytes",
				maxRequestBytes))
		}
		return commonerrors.NewBadRequest("invalid request body: " + err.Error())
	}
	return nil
}

// preparedRun is a run whose request passed every check, with its row and workload built but not yet created.
type preparedRun struct {
	row      *dbclient.PostTrainingRun
	workload *v1.Workload
	warnings []recipe.Diagnostic
	// runtimeCond is the timeline entry of a timeout set above the runtime ceiling, added once the workload exists.
	runtimeCond *metav1.Condition
}

// submit runs the whole submission; it returns diagnostics without a response when the request does not compile.
func (h *Handler) submit(c *gin.Context, reg *registry.Registry,
	req *recipe.SubmitRequest) (*submitResponse, []recipe.Diagnostic, error) {
	p, diags, err := h.prepareRun(c, reg, req)
	if err != nil || p == nil {
		return nil, diags, err
	}
	resp, err := h.createRun(c, p)
	return resp, nil, err
}

// prepareRun checks a request, compiles it and builds its run row and workload without creating anything; it
// returns diagnostics without a run when the request does not compile.
func (h *Handler) prepareRun(c *gin.Context, reg *registry.Registry,
	req *recipe.SubmitRequest) (*preparedRun, []recipe.Diagnostic, error) {
	if strings.TrimSpace(req.DisplayName) == "" {
		return nil, nil, commonerrors.NewBadRequest("displayName is required")
	}
	if utf8.RuneCountInString(req.DisplayName) > maxDisplayNameLen {
		return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("displayName has more than %d characters",
			maxDisplayNameLen))
	}
	if r := req.Resources.MaxRetry; r != nil && (*r < 0 || *r > maxRetryLimit) {
		return nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("maxRetry must be between 0 and %d", maxRetryLimit))
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
	h.markConvertCache(c, req.Workspace, res.Recipe)
	recipeJSON, err := json.Marshal(res.Recipe)
	if err != nil {
		return nil, nil, commonerrors.NewInternalError("failed to encode the recipe: " + err.Error())
	}
	workload, err := buildWorkload(c, reg, req, res, ws, runID, recipeJSON)
	if err != nil {
		return nil, nil, err
	}
	runtimeCond, err := h.authorizeRuntime(c, workload)
	if err != nil {
		return nil, nil, err
	}
	row := runRow(c, req, res, ws, model, runID, string(recipeJSON))
	if in.Dataset != nil {
		row.DatasetName = in.Dataset.Name
	}
	return &preparedRun{row: row, workload: workload, warnings: res.Diagnostics, runtimeCond: runtimeCond}, nil, nil
}

// refused tells whether the cluster answered a creation with a refusal, after which the object does not exist. That
// is unknown after a cancelled request, a timeout or a broken connection, after an answer that is not the API
// server's own, and after one that an object of the name exists, that the server is unavailable, or that the
// server failed without a reason it names.
func refused(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		apierrors.IsUnexpectedServerError(err) {
		return false
	}
	switch apierrors.ReasonForError(err) {
	case metav1.StatusReasonTimeout, metav1.StatusReasonServerTimeout, metav1.StatusReasonServiceUnavailable,
		metav1.StatusReasonAlreadyExists:
		return false
	case metav1.StatusReasonUnknown:
		// Without a reason, an answer that blames the request is the denial of a webhook, which the API server
		// passes on as the webhook worded it; one that blames the server may be an error of its storage.
		var answer apierrors.APIStatus
		return errors.As(err, &answer) && answer.Status().Code >= http.StatusBadRequest &&
			answer.Status().Code < http.StatusInternalServerError
	}
	return true
}

// createRun records a prepared run and creates its workload; a run whose workload the cluster refuses is
// recorded as failed.
func (h *Handler) createRun(c *gin.Context, p *preparedRun) (*submitResponse, error) {
	ctx := c.Request.Context()
	if err := h.db.CreatePostTrainingRun(ctx, p.row); err != nil {
		return nil, commonerrors.NewInternalError("failed to record the run: " + err.Error())
	}
	if err := h.client.Create(ctx, p.workload); err != nil {
		klog.ErrorS(err, "failed to create the Post Training workload", "run", p.row.RunID)
		if !refused(err) {
			// The workload may exist, and a failed run is not followed: the run stays pending, so the controller
			// follows the workload or, when none shows, fails the run.
			return nil, err
		}
		// Apart from the request: a caller that left must not leave the run recorded as pending.
		mark, cancel := detached(ctx)
		defer cancel()
		if uerr := h.db.UpdatePostTrainingRun(mark, p.row.RunID, map[string]interface{}{
			"status": string(v1.WorkloadFailed), "message": "failed to create the workload: " + err.Error(),
		}); uerr != nil {
			klog.ErrorS(uerr, "failed to mark the run failed", "run", p.row.RunID)
		}
		return nil, err
	}
	if p.runtimeCond != nil {
		// As POST /workloads does, the timeline says who set the timeout above the ceiling; the run goes on without it.
		if err := h.appendWorkloadCondition(ctx, p.workload, p.runtimeCond); err != nil {
			klog.ErrorS(err, "failed to record the runtime extension", "run", p.row.RunID)
		}
	}
	return &submitResponse{RunID: p.row.RunID, WorkloadID: p.workload.Name, Warnings: p.warnings}, nil
}

// buildInput gathers what the compiler needs about the workspace, model, datasets and secrets of a request.
func (h *Handler) buildInput(c *gin.Context, reg *registry.Registry, req *recipe.SubmitRequest,
	runID string) (*recipe.Input, *workspaceInfo, *v1.Model, error) {
	ctx := c.Request.Context()
	ws, err := h.getWorkspace(ctx, req.Workspace)
	if err != nil {
		return nil, nil, nil, err
	}
	if ws.storageRoot == "" {
		return nil, nil, nil, commonerrors.NewBadRequest(fmt.Sprintf("workspace %s has no volume for the run to "+
			"write to", ws.ID))
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

// datasetMissing reports whether the error of a dataset lookup says that the dataset does not exist, which an
// empty id does too.
func datasetMissing(err error) bool {
	return commonerrors.IsNotFound(err) || commonerrors.IsBadRequest(err)
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
	if s := req.Resources.EphemeralStorage; s != "" {
		q, err := resource.ParseQuantity(s)
		if err != nil || q.Sign() <= 0 {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("resources.ephemeralStorage %q is not a positive quantity", s))
		}
		node.EphemeralStorage = q.String()
	}
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
		v1.DisplayNameLabel:       commonutils.DNSDisplayName("pt-", req.DisplayName, commonutils.MaxRayJobNameLen),
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

// authorizeRuntime applies to a run's workload what POST /workloads applies to a new workload: a timeout above the
// runtime ceiling of the user's QoS needs the workload/long-runtime grant in the workspace, and the workload is then
// marked with WorkloadRuntimeExtendedByAnnotation, which the workload webhook takes as that authorization. A timeout
// that is unset is filled from the ceiling by the webhook. It runs before the run is recorded, so a timeout the user
// may not set is refused without leaving a failed run behind. For an extended timeout it returns the RuntimeExtended
// timeline entry that POST /workloads adds as well.
func (h *Handler) authorizeRuntime(c *gin.Context, w *v1.Workload) (*metav1.Condition, error) {
	v1.RemoveAnnotation(w, v1.WorkloadRuntimeExtendedByAnnotation)
	if w.Spec.Timeout == nil || *w.Spec.Timeout <= 0 {
		return nil, nil
	}
	ctx := c.Request.Context()
	userID := c.GetString(common.UserId)
	limits, err := commonqos.Ceiling(ctx, h.client, userID, commonqos.WorkspaceID(w),
		string(commonworkload.GetScope(w)))
	if err != nil {
		return nil, commonerrors.NewInternalError("failed to read the runtime limit: " + err.Error())
	}
	if ceiling := limits.RuntimeSeconds; ceiling <= 0 || *w.Spec.Timeout <= ceiling {
		return nil, nil
	}
	if err := h.authorizeKinds(c, v1.CreateVerb, w.Spec.Workspace, "", w.Name,
		authority.WorkloadLongRuntimeKind); err != nil {
		if apierrors.IsForbidden(err) {
			return nil, commonerrors.NewForbidden(fmt.Sprintf("resources.timeoutSeconds %d is above the limit of %d "+
				"seconds in workspace %s; a longer timeout requires the %s grant in the workspace",
				*w.Spec.Timeout, limits.RuntimeSeconds, w.Spec.Workspace, authority.WorkloadLongRuntimeKind))
		}
		return nil, err
	}
	v1.SetAnnotation(w, v1.WorkloadRuntimeExtendedByAnnotation, userID)
	name := c.GetString(common.UserName)
	if name == "" {
		name = userID
	}
	return &metav1.Condition{
		Type:   string(v1.RuntimeExtended),
		Status: metav1.ConditionTrue,
		Reason: "Extended",
		Message: fmt.Sprintf("runtime set to %s by %s, above the qos limit %s", formatRuntime(*w.Spec.Timeout), name,
			formatRuntime(limits.RuntimeSeconds)),
		LastTransitionTime: metav1.NewTime(time.Now().UTC()),
	}, nil
}

// formatRuntime renders a timeout in seconds as the workload timeline does, such as 3h or 1h30m.
func formatRuntime(seconds int) string {
	text := (time.Duration(seconds) * time.Second).String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}
	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}
	return text
}

// appendWorkloadCondition adds one timeline entry to a workload's status, reading the workload again on a conflict.
func (h *Handler) appendWorkloadCondition(ctx context.Context, w *v1.Workload, cond *metav1.Condition) error {
	for attempt := 0; ; attempt++ {
		base := w.DeepCopy()
		w.Status.Conditions = append(w.Status.Conditions, *cond)
		err := h.client.Status().Patch(ctx, w, ctrlclient.MergeFromWithOptions(base, ctrlclient.MergeFromWithOptimisticLock{}))
		if !apierrors.IsConflict(err) || attempt == 2 {
			return err
		}
		latest := &v1.Workload{}
		if err := h.client.Get(ctx, ctrlclient.ObjectKeyFromObject(w), latest); err != nil {
			return err
		}
		w = latest
	}
}

// rewardSecret returns the secret of an RL request's remote reward service, or an empty string.
func rewardSecret(req *recipe.SubmitRequest) string {
	remote := req.Choices.RewardRemote
	if req.TrainType != registry.TrainTypeRL || req.Choices.RewardType != "remote" || remote == nil {
		return ""
	}
	return remote.SecretID
}

// markConvertCache records on a compiled recipe whether the workspace already holds its converted weights.
func (h *Handler) markConvertCache(c *gin.Context, workspace string, r *recipe.Recipe) {
	if cv := r.Convert; cv != nil && cv.CacheKey != "" {
		cache, err := h.db.GetPostTrainingConvertCache(c.Request.Context(), workspace, cv.CacheKey)
		cv.CacheHit = err == nil && cache.Status == dbclient.ConvertCacheReady
	}
}

// nodeResource requests the share of a node that gpus of its GPUs carry, the whole node when they are all of them:
// that fraction of the flavor's allocatable CPU, memory and ephemeral storage after the workspace reserves, and
// half that memory as shared memory. The ephemeral storage is capped at the per-workload maximum the workload
// webhook enforces before it is shared.
func nodeResource(nf *v1.NodeFlavor, gpus int) v1.WorkloadResource {
	nodeResources := nf.ToResourceList("")
	avail := quantity.GetAvailableResource(nodeResources)
	r := v1.WorkloadResource{GPU: strconv.Itoa(gpus)}
	total := nf.GetGpuCount()
	share := func(v int64) int64 {
		if total <= 0 || gpus >= total {
			return v
		}
		return v * int64(gpus) / int64(total)
	}
	if q, ok := avail[corev1.ResourceCPU]; ok {
		r.CPU = resource.NewMilliQuantity(share(q.MilliValue()), resource.DecimalSI).String()
	}
	if q, ok := avail[corev1.ResourceMemory]; ok {
		r.Memory = resource.NewQuantity(share(q.Value()), resource.BinarySI).String()
		r.SharedMemory = resource.NewQuantity(share(q.Value())/2, resource.BinarySI).String()
	}
	if q, ok := avail[corev1.ResourceEphemeralStorage]; ok {
		if limit, err := quantity.GetMaxEphemeralStoreQuantity(nodeResources); err == nil && limit.Cmp(q) < 0 {
			q = *limit
		}
		r.EphemeralStorage = resource.NewQuantity(share(q.Value()), resource.BinarySI).String()
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
