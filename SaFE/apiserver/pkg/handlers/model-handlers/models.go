/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// CreateModel handles the creation of a new playground model.
func (h *Handler) CreateModel(c *gin.Context) {
	handle(c, h.createModel)
}

// ListModels handles listing of playground models.
func (h *Handler) ListModels(c *gin.Context) {
	handle(c, h.listModels)
}

// GetModel handles getting a single playground model by ID.
func (h *Handler) GetModel(c *gin.Context) {
	handle(c, h.getModel)
}

// DeleteModel handles the deletion of a playground model.
func (h *Handler) DeleteModel(c *gin.Context) {
	handle(c, h.deleteModel)
}

// RetryModel handles retrying a failed model download.
func (h *Handler) RetryModel(c *gin.Context) {
	handle(c, h.retryModel)
}

// PatchModel handles partial updates to a model's mutable fields.
func (h *Handler) PatchModel(c *gin.Context) {
	handle(c, h.patchModel)
}

// GetModelWorkloads handles listing workloads associated with a model.
func (h *Handler) GetModelWorkloads(c *gin.Context) {
	handle(c, h.getModelWorkloads)
}

// GetWorkloadConfig handles generating workload configuration for a model.
func (h *Handler) GetWorkloadConfig(c *gin.Context) {
	handle(c, h.getWorkloadConfig)
}

// createModel implements the model creation logic.
func (h *Handler) createModel(c *gin.Context) (interface{}, error) {
	var req CreateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	ctx := context.Background()

	// Validate URL first
	if req.Source.URL == "" {
		return nil, commonerrors.NewBadRequest("model source url is required")
	}

	// Validate AccessMode
	if req.Source.AccessMode != string(v1.AccessModeLocal) && req.Source.AccessMode != string(v1.AccessModeRemoteAPI) {
		return nil, commonerrors.NewBadRequest("accessMode must be 'local' or 'remote_api'")
	}

	// For remote_api mode, modelName is required
	if req.Source.AccessMode == string(v1.AccessModeRemoteAPI) {
		if req.Source.ModelName == "" {
			return nil, commonerrors.NewBadRequest("modelName is required for remote_api mode")
		}
		if req.DisplayName == "" {
			return nil, commonerrors.NewBadRequest("displayName is required for remote_api mode")
		}
		if req.Source.ApiKey == "" {
			klog.Warningf("Creating remote_api model '%s' without apiKey, authentication may fail", req.DisplayName)
		}
	}

	// Normalize URL for consistent matching (trim trailing slash and spaces)
	normalizedURL := strings.TrimSuffix(strings.TrimSpace(req.Source.URL), "/")

	// For local mode: ensure URL has complete HuggingFace prefix
	// This handles cases like "Qwen/Qwen2.5-7B-Instruct" -> "https://huggingface.co/Qwen/Qwen2.5-7B-Instruct"
	if req.Source.AccessMode == string(v1.AccessModeLocal) && !isFullURL(normalizedURL) {
		normalizedURL = fmt.Sprintf("https://huggingface.co/%s", normalizedURL)
		klog.InfoS("Added HuggingFace URL prefix", "original", req.Source.URL, "normalized", normalizedURL)
	}

	// Check if model with same URL already exists (only for local mode)
	// Remote API mode skips duplicate check since each user has their own API key
	if req.Source.AccessMode == string(v1.AccessModeLocal) && normalizedURL != "" {
		existingModel, _ := h.findModelBySourceURL(ctx, normalizedURL, req.Workspace)
		if existingModel != nil {
			if existingModel.Phase == string(v1.ModelPhaseReady) {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("model with URL '%s' already exists and is ready (id: %s)", existingModel.SourceURL, existingModel.ID))
			} else if existingModel.Phase == string(v1.ModelPhaseUploading) || existingModel.Phase == string(v1.ModelPhaseDownloading) {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("model with URL '%s' is currently being processed (id: %s, phase: %s)", existingModel.SourceURL, existingModel.ID, existingModel.Phase))
			} else if existingModel.Phase == string(v1.ModelPhasePending) {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("model with URL '%s' already exists and is pending (id: %s)", existingModel.SourceURL, existingModel.ID))
			} else if existingModel.Phase == string(v1.ModelPhaseFailed) {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("model with URL '%s' already exists but failed (id: %s). Please use the retry API (POST /models/%s/retry) to re-download", existingModel.SourceURL, existingModel.ID, existingModel.ID))
			}
		}
	}

	var (
		displayName string
		description string
		icon        string
		label       string
		tags        []string
		maxTokens   int
		modelName   string
	)

	// Handle metadata based on AccessMode
	if req.Source.AccessMode == string(v1.AccessModeLocal) {
		// Local mode: Auto-fill metadata from Hugging Face
		if hfInfo, err := GetHFModelInfo(req.Source.URL); err == nil {
			displayName = hfInfo.DisplayName
			description = hfInfo.Description
			icon = hfInfo.Icon
			label = hfInfo.Label
			tags = hfInfo.Tags
			maxTokens = hfInfo.MaxTokens
			// For local models, modelName defaults to the repo ID (e.g., "meta-llama/Llama-2-7b")
			modelName = cleanRepoID(normalizedURL)
			klog.InfoS("Auto-filled model metadata from Hugging Face", "model", hfInfo.DisplayName, "maxTokens", maxTokens)
		} else {
			klog.ErrorS(err, "Failed to fetch metadata from Hugging Face", "url", req.Source.URL)
			return nil, commonerrors.NewBadRequest("failed to fetch model info from Hugging Face: " + err.Error())
		}
	} else {
		// Remote API mode: Use user-provided metadata
		displayName = req.DisplayName
		description = req.Description
		icon = req.Icon
		label = req.Label
		tags = req.Tags
		maxTokens = req.MaxTokens
		modelName = req.Source.ModelName // Required for remote_api
		klog.InfoS("Using user-provided metadata for remote API model", "displayName", displayName, "modelName", modelName)
	}

	// Generate Name
	name := commonutils.GenerateName("model")

	// Determine initial phase based on access mode
	var initialPhase v1.ModelPhase
	if req.Source.AccessMode == string(v1.AccessModeRemoteAPI) {
		// Remote API models are immediately ready
		initialPhase = v1.ModelPhaseReady
	} else {
		// Local models need to be uploaded to S3 first
		initialPhase = v1.ModelPhasePending
	}

	// Create K8s Model CR
	k8sModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.ModelSpec{
			DisplayName: displayName,
			Description: description,
			Icon:        icon,
			Label:       label,
			Tags:        tags,
			MaxTokens:   maxTokens,
			Workspace:   req.Workspace, // Empty means public (available to all workspaces)
			Source: v1.ModelSource{
				URL:        normalizedURL,
				AccessMode: v1.AccessMode(req.Source.AccessMode),
				ModelName:  modelName,
			},
		},
		Status: v1.ModelStatus{
			Phase:      initialPhase,
			UpdateTime: &metav1.Time{Time: time.Now().UTC()},
		},
	}

	if err := h.k8sClient.Create(ctx, k8sModel); err != nil {
		klog.ErrorS(err, "Failed to create Model CR")
		return nil, commonerrors.NewInternalError("failed to create model resource: " + err.Error())
	}

	// If user provided a token (for local mode HuggingFace access), create a Secret
	if req.Source.Token != "" {
		tokenSecretName := name + "-token" // Secret name: model-xxx-token
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tokenSecretName,
				Namespace: common.PrimusSafeNamespace,
				Labels: map[string]string{
					v1.ModelIdLabel: name,
				},
			},
			StringData: map[string]string{
				"token": req.Source.Token,
			},
			Type: corev1.SecretTypeOpaque,
		}

		// Note: Cannot use OwnerReference here because Model is cluster-scoped
		// and Secret is namespace-scoped. Will delete manually in deleteModel.
		if err := h.k8sClient.Create(ctx, secret); err != nil {
			klog.ErrorS(err, "Failed to create token Secret")
			_ = h.k8sClient.Delete(ctx, k8sModel)
			return nil, commonerrors.NewInternalError("failed to create token secret: " + err.Error())
		}
		klog.Infof("Created token Secret: %s for Model %s", tokenSecretName, name)

		// Update Model to reference the Secret
		k8sModel.Spec.Source.Token = &corev1.LocalObjectReference{
			Name: tokenSecretName,
		}
		if err := h.k8sClient.Update(ctx, k8sModel); err != nil {
			klog.ErrorS(err, "Failed to update Model with token reference")
			return nil, commonerrors.NewInternalError("failed to update model with token: " + err.Error())
		}
	}

	// If user provided an API key (for remote_api mode), create a Secret
	if req.Source.ApiKey != "" {
		apiKeySecretName := name + "-apikey" // Secret name: model-xxx-apikey
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      apiKeySecretName,
				Namespace: common.PrimusSafeNamespace,
				Labels: map[string]string{
					v1.ModelIdLabel: name,
				},
			},
			StringData: map[string]string{
				"apiKey": req.Source.ApiKey,
			},
			Type: corev1.SecretTypeOpaque,
		}

		// Note: Cannot use OwnerReference here because Model is cluster-scoped
		// and Secret is namespace-scoped. Will delete manually in deleteModel.
		if err := h.k8sClient.Create(ctx, secret); err != nil {
			klog.ErrorS(err, "Failed to create apiKey Secret")
			_ = h.k8sClient.Delete(ctx, k8sModel)
			return nil, commonerrors.NewInternalError("failed to create apiKey secret: " + err.Error())
		}
		klog.Infof("Created apiKey Secret: %s for Model %s", apiKeySecretName, name)

		// Update Model to reference the Secret
		k8sModel.Spec.Source.ApiKey = &corev1.LocalObjectReference{
			Name: apiKeySecretName,
		}
		if err := h.k8sClient.Update(ctx, k8sModel); err != nil {
			klog.ErrorS(err, "Failed to update Model with apiKey reference")
			return nil, commonerrors.NewInternalError("failed to update model with apiKey: " + err.Error())
		}
	}

	// For remote_api models, update status to Ready
	if req.Source.AccessMode == string(v1.AccessModeRemoteAPI) {
		k8sModel.Status.Phase = v1.ModelPhaseReady
		k8sModel.Status.Message = "Remote API model is ready"
		if err := h.k8sClient.Status().Update(ctx, k8sModel); err != nil {
			klog.ErrorS(err, "Failed to update model status to Ready", "model", name)
		}
	}

	return &CreateResponse{ID: name}, nil
}

// getModel implements the logic to get a single model by ID.
func (h *Handler) getModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	ctx := c.Request.Context()

	// 1. Try to fetch from database first (if available)
	if h.dbClient != nil {
		dbModel, err := h.dbClient.GetModelByID(ctx, modelId)
		if err == nil && dbModel != nil && !dbModel.IsDeleted {
			return cvtDBModelToInfo(dbModel), nil
		}
		// If not found in DB or error, fall through to K8s
	}

	// 2. Fallback to K8s
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound("playground model", modelId)
		}
		return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
	}

	return h.convertK8sModelToInfo(k8sModel), nil
}

func parseListModelQuery(c *gin.Context) (*ListModelQuery, error) {
	query := &ListModelQuery{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	// Set default values
	if query.Limit <= 0 {
		query.Limit = 10 // Default limit
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return query, nil
}

// listModels implements the model listing logic.
func (h *Handler) listModels(c *gin.Context) (interface{}, error) {
	queryArgs, err := parseListModelQuery(c)
	if err != nil {
		return nil, err
	}

	ctx := c.Request.Context()

	// 1. Try to list from database first (if available)
	if h.dbClient != nil {
		dbModels, err := h.dbClient.ListModels(ctx, queryArgs.AccessMode, queryArgs.Workspace, false)
		if err == nil && len(dbModels) > 0 {
			var items []ModelInfo
			for _, dbModel := range dbModels {
				items = append(items, cvtDBModelToInfo(dbModel))
			}

			// Apply pagination
			total := int64(len(items))
			start := queryArgs.Offset
			end := queryArgs.Offset + queryArgs.Limit
			if start > int(total) {
				start = int(total)
			}
			if end > int(total) {
				end = int(total)
			}

			return &ListModelResponse{
				Total: total,
				Items: items[start:end],
			}, nil
		}
		// If error or empty, fall through to K8s
	}

	// 2. Fallback to K8s
	k8sModelList := &v1.ModelList{}
	if err := h.k8sClient.List(ctx, k8sModelList); err != nil {
		return nil, commonerrors.NewInternalError("failed to list models from K8s: " + err.Error())
	}

	// Filter and convert models
	var items []ModelInfo
	for _, k8sModel := range k8sModelList.Items {
		// Skip deleted models
		if k8sModel.DeletionTimestamp != nil {
			continue
		}

		// Apply filters
		if queryArgs.AccessMode != "" && string(k8sModel.Spec.Source.AccessMode) != queryArgs.AccessMode {
			continue
		}
		if queryArgs.Workspace != "" {
			// For workspace filter: include public models (empty workspace) + specific workspace models
			if k8sModel.Spec.Workspace != "" && k8sModel.Spec.Workspace != queryArgs.Workspace {
				continue
			}
		}

		items = append(items, h.convertK8sModelToInfo(&k8sModel))
	}

	// Apply pagination
	total := int64(len(items))
	start := queryArgs.Offset
	end := queryArgs.Offset + queryArgs.Limit
	if start > int(total) {
		start = int(total)
	}
	if end > int(total) {
		end = int(total)
	}

	return &ListModelResponse{
		Total: total,
		Items: items[start:end],
	}, nil
}

// convertK8sModelToInfo converts a K8s Model CR to ModelInfo response format
func (h *Handler) convertK8sModelToInfo(k8sModel *v1.Model) ModelInfo {
	// Convert local paths
	var localPaths []LocalPathInfo
	for _, lp := range k8sModel.Status.LocalPaths {
		localPaths = append(localPaths, LocalPathInfo{
			Workspace: lp.Workspace,
			Path:      lp.Path,
			Status:    string(lp.Status),
			Message:   lp.Message,
		})
	}

	// Determine tags for categorization
	var tagsStr string
	if len(k8sModel.Spec.Tags) > 0 {
		tagsStr = strings.Join(k8sModel.Spec.Tags, ",")
	}

	// Include unmatched tags for remote_api models
	includeUnmatched := k8sModel.Spec.Source.AccessMode == v1.AccessModeRemoteAPI

	return ModelInfo{
		ID:              k8sModel.Name,
		DisplayName:     k8sModel.Spec.DisplayName,
		Description:     k8sModel.Spec.Description,
		Icon:            k8sModel.Spec.Icon,
		Label:           k8sModel.Spec.Label,
		Tags:            tagsStr,
		CategorizedTags: CategorizeTagString(tagsStr, includeUnmatched),
		MaxTokens:       k8sModel.Spec.MaxTokens,
		SourceURL:       k8sModel.Spec.Source.URL,
		AccessMode:      string(k8sModel.Spec.Source.AccessMode),
		ModelName:       k8sModel.Spec.Source.ModelName,
		Phase:           string(k8sModel.Status.Phase),
		Message:         k8sModel.Status.Message,
		Workspace:       k8sModel.Spec.Workspace,
		S3Path:          k8sModel.Status.S3Path,
		LocalPaths:      localPaths,
		CreatedAt:       k8sModel.CreationTimestamp.Format(time.RFC3339),
	}
}

// deleteModel implements the model deletion logic with safety checks.
func (h *Handler) deleteModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	// Parse delete options from request body (optional)
	var req DeleteModelRequest
	_ = c.ShouldBindJSON(&req) // Ignore binding errors for optional body

	ctx := c.Request.Context()

	// 1. Get the model from K8s
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound("model", modelId)
		}
		klog.ErrorS(err, "Failed to get model from K8s", "model", modelId)
		return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
	}

	// 2. Check for associated workloads using label selector
	workloadList := &v1.WorkloadList{}
	labelSelector := ctrlclient.MatchingLabels{
		v1.SourceModelLabel: modelId,
	}
	if err := h.k8sClient.List(ctx, workloadList, labelSelector); err != nil {
		klog.ErrorS(err, "Failed to list workloads by label", "model", modelId)
		return nil, commonerrors.NewInternalError("failed to check associated workloads: " + err.Error())
	}

	// 3. Safety check: Reject deletion if running/pending workloads exist (unless force=true)
	var runningWorkloads []string
	var stoppedWorkloads []string
	for _, w := range workloadList.Items {
		phase := w.Status.Phase
		if phase == v1.WorkloadRunning || phase == v1.WorkloadPending {
			runningWorkloads = append(runningWorkloads, w.Name)
		} else if phase == v1.WorkloadStopped || phase == v1.WorkloadFailed {
			stoppedWorkloads = append(stoppedWorkloads, w.Name)
		}
	}

	if len(runningWorkloads) > 0 && !req.Force {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf(
			"cannot delete model with running workloads: %v. Use force=true to override or stop the workloads first",
			runningWorkloads,
		))
	}

	// 4. Delete associated stopped/failed workloads if requested
	if req.DeleteAssociated && len(stoppedWorkloads) > 0 {
		for _, wName := range stoppedWorkloads {
			w := &v1.Workload{}
			if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: wName, Namespace: k8sModel.Spec.Workspace}, w); err == nil {
				if err := h.k8sClient.Delete(ctx, w); err != nil && !errors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete associated workload", "workload", wName)
				} else {
					klog.InfoS("Deleted associated workload", "workload", wName, "model", modelId)
				}
			}
		}
	}

	// 5. Delete Token Secret manually (if exists)
	if k8sModel.Spec.Source.Token != nil && k8sModel.Spec.Source.Token.Name != "" {
		tokenSecret := &corev1.Secret{}
		tokenSecretKey := ctrlclient.ObjectKey{
			Name:      k8sModel.Spec.Source.Token.Name,
			Namespace: common.PrimusSafeNamespace,
		}
		if err := h.k8sClient.Get(ctx, tokenSecretKey, tokenSecret); err == nil {
			if err := h.k8sClient.Delete(ctx, tokenSecret); err != nil && !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete token secret", "secret", tokenSecretKey.Name)
			} else {
				klog.InfoS("Token secret deleted", "secret", tokenSecretKey.Name, "model", modelId)
			}
		}
	}

	// 5.1 Delete ApiKey Secret manually (if exists)
	if k8sModel.Spec.Source.ApiKey != nil && k8sModel.Spec.Source.ApiKey.Name != "" {
		apiKeySecret := &corev1.Secret{}
		apiKeySecretKey := ctrlclient.ObjectKey{
			Name:      k8sModel.Spec.Source.ApiKey.Name,
			Namespace: common.PrimusSafeNamespace,
		}
		if err := h.k8sClient.Get(ctx, apiKeySecretKey, apiKeySecret); err == nil {
			if err := h.k8sClient.Delete(ctx, apiKeySecret); err != nil && !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete apiKey secret", "secret", apiKeySecretKey.Name)
			} else {
				klog.InfoS("ApiKey secret deleted", "secret", apiKeySecretKey.Name, "model", modelId)
			}
		}
	}

	// 6. Delete K8s Model CR
	// The model controller will handle S3 and local path cleanup via finalizer
	if err := h.k8sClient.Delete(ctx, k8sModel); err != nil {
		if !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete model from K8s", "model", modelId)
			return nil, commonerrors.NewInternalError("failed to delete model: " + err.Error())
		}
	}

	klog.InfoS("Model deletion initiated", "model", modelId,
		"runningWorkloads", len(runningWorkloads),
		"stoppedWorkloads", len(stoppedWorkloads),
		"force", req.Force,
		"deleteAssociated", req.DeleteAssociated)

	return gin.H{
		"message":          "model deleted successfully",
		"id":               modelId,
		"deletedWorkloads": stoppedWorkloads,
	}, nil
}

// retryModel implements the logic to retry a failed model download.
func (h *Handler) retryModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	ctx := c.Request.Context()

	// Get the model from K8s
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound("model", modelId)
		}
		klog.ErrorS(err, "Failed to get model from K8s", "model", modelId)
		return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
	}

	// Check if model is in Failed phase
	if k8sModel.Status.Phase != v1.ModelPhaseFailed {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("model is not in Failed phase, current phase: %s. Only failed models can be retried", k8sModel.Status.Phase))
	}

	// Reset model status to Pending
	originalPhase := k8sModel.Status.Phase
	k8sModel.Status.Phase = v1.ModelPhasePending
	k8sModel.Status.Message = "Retry requested, re-downloading model..."

	if err := h.k8sClient.Status().Update(ctx, k8sModel); err != nil {
		klog.ErrorS(err, "Failed to update model status", "model", modelId)
		return nil, commonerrors.NewInternalError("failed to update model status: " + err.Error())
	}

	klog.InfoS("Model retry initiated", "model", modelId, "previousPhase", originalPhase, "newPhase", v1.ModelPhasePending)

	return gin.H{
		"message":       "model retry initiated successfully",
		"id":            modelId,
		"previousPhase": string(originalPhase),
		"currentPhase":  string(v1.ModelPhasePending),
	}, nil
}

// patchModel implements partial update of a model's mutable fields.
func (h *Handler) patchModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	var req PatchModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	// Check if any field is provided
	if req.ModelName == nil && req.DisplayName == nil && req.Description == nil {
		return nil, commonerrors.NewBadRequest("at least one field must be provided for update")
	}

	ctx := c.Request.Context()

	// Get the model from K8s
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound("model", modelId)
		}
		klog.ErrorS(err, "Failed to get model from K8s", "model", modelId)
		return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
	}

	// Apply updates using patch
	patch := ctrlclient.MergeFrom(k8sModel.DeepCopy())

	if req.ModelName != nil {
		k8sModel.Spec.Source.ModelName = *req.ModelName
	}
	if req.DisplayName != nil {
		k8sModel.Spec.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		k8sModel.Spec.Description = *req.Description
	}

	if err := h.k8sClient.Patch(ctx, k8sModel, patch); err != nil {
		klog.ErrorS(err, "Failed to patch model", "model", modelId)
		return nil, commonerrors.NewInternalError("failed to update model: " + err.Error())
	}

	klog.InfoS("Model patched successfully", "model", modelId,
		"modelName", req.ModelName,
		"displayName", req.DisplayName,
		"description", req.Description)

	return h.convertK8sModelToInfo(k8sModel), nil
}

// getModelWorkloads lists all workloads associated with a model via label selector.
func (h *Handler) getModelWorkloads(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	ctx := c.Request.Context()

	// Verify model exists
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound("model", modelId)
		}
		return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
	}

	// List workloads by label selector
	workloadList := &v1.WorkloadList{}
	labelSelector := ctrlclient.MatchingLabels{
		v1.SourceModelLabel: modelId,
	}
	if err := h.k8sClient.List(ctx, workloadList, labelSelector); err != nil {
		klog.ErrorS(err, "Failed to list workloads by label", "model", modelId)
		return nil, commonerrors.NewInternalError("failed to list workloads: " + err.Error())
	}

	// Convert to response format
	var items []AssociatedWorkload
	for _, w := range workloadList.Items {
		items = append(items, AssociatedWorkload{
			WorkloadID:  w.Name,
			DisplayName: w.Name, // Workload doesn't have DisplayName, use Name
			Workspace:   w.Namespace,
			Phase:       string(w.Status.Phase),
			CreatedAt:   w.CreationTimestamp.Format(time.RFC3339),
		})
	}

	return &ModelWorkloadsResponse{
		Total: len(items),
		Items: items,
	}, nil
}

// getWorkloadConfig generates a pre-filled workload configuration for deploying a model.
func (h *Handler) getWorkloadConfig(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	workspace := c.Query("workspace")
	if workspace == "" {
		return nil, commonerrors.NewBadRequest("workspace query parameter is required")
	}

	ctx := c.Request.Context()

	// Get the model
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound("model", modelId)
		}
		return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
	}

	// Only local models can be deployed as workloads
	if k8sModel.Spec.Source.AccessMode != v1.AccessModeLocal {
		return nil, commonerrors.NewBadRequest("only local models can be deployed as workloads")
	}

	// Check if model is ready
	if k8sModel.Status.Phase != v1.ModelPhaseReady {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("model is not ready, current phase: %s", k8sModel.Status.Phase))
	}

	// Get the local path for the specified workspace
	var modelPath string
	for _, lp := range k8sModel.Status.LocalPaths {
		if lp.Workspace == workspace && lp.Status == v1.LocalPathStatusReady {
			modelPath = lp.Path
			break
		}
	}

	if modelPath == "" {
		// Check if model is public (should be available in all workspaces)
		if k8sModel.IsPublic() {
			// For public models, construct the expected path
			modelPath = fmt.Sprintf("/wekafs/models/%s", k8sModel.GetSafeDisplayName())
		} else {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("model is not available in workspace %s", workspace))
		}
	}

	// Generate workload configuration
	// Pre-filled fields: DisplayName, Description, Labels, Env, ModelID, ModelName, ModelPath, AccessMode, MaxTokens, Workspace
	// User-provided fields: Image, EntryPoint, CPU, Memory, GPU (must be filled by frontend)
	config := WorkloadConfigResponse{
		DisplayName: fmt.Sprintf("%s-infer", k8sModel.Spec.DisplayName),
		Description: fmt.Sprintf("Inference service for %s", k8sModel.Spec.DisplayName),
		Labels: map[string]string{
			v1.SourceModelLabel: modelId,
		},
		Env: map[string]string{
			"MODEL_PATH": modelPath,
		},
		ModelID:    modelId,
		ModelName:  k8sModel.GetModelName(),
		ModelPath:  modelPath,
		AccessMode: string(k8sModel.Spec.Source.AccessMode),
		MaxTokens:  k8sModel.Spec.MaxTokens,
		Workspace:  workspace,
		// TODO: The following fields must be filled by the user in the frontend
		// Consider providing recommended values based on model type/size:
		// - Image: vllm/vllm-openai:latest, ghcr.io/huggingface/text-generation-inference:latest
		// - EntryPoint: python -m vllm.entrypoints.openai.api_server --model ${MODEL_PATH}
		// - CPU/Memory/GPU: based on model parameters (7B -> 8GPU, 70B -> 16GPU, etc.)
		Image:      "", // Required: user must provide container image
		EntryPoint: "", // Required: user must provide startup command
		CPU:        "", // Required: user must specify CPU request
		Memory:     "", // Required: user must specify memory request
		GPU:        "", // Required: user must specify GPU request
	}

	return config, nil
}

// findModelBySourceURL checks if a model with the given source URL and workspace already exists.
func (h *Handler) findModelBySourceURL(ctx context.Context, sourceURL string, workspace string) (*dbclient.Model, error) {
	// Check K8s directly for immediate consistency
	modelList := &v1.ModelList{}
	if err := h.k8sClient.List(ctx, modelList); err == nil {
		for _, m := range modelList.Items {
			if m.Spec.Source.URL == sourceURL && m.DeletionTimestamp == nil {
				// For local models, also check workspace
				if m.Spec.Source.AccessMode == v1.AccessModeLocal {
					// Public models (empty workspace) are considered duplicates regardless of workspace
					// Specific workspace models are duplicates only for the same workspace
					if m.Spec.Workspace == "" || m.Spec.Workspace == workspace || workspace == "" {
						return &dbclient.Model{
							ID:        m.Name,
							SourceURL: m.Spec.Source.URL,
							Phase:     string(m.Status.Phase),
							Workspace: m.Spec.Workspace,
						}, nil
					}
				} else {
					return &dbclient.Model{
						ID:        m.Name,
						SourceURL: m.Spec.Source.URL,
						Phase:     string(m.Status.Phase),
					}, nil
				}
			}
		}
	}

	// Also try with different URL formats for HuggingFace repos
	repoId := cleanRepoID(sourceURL)
	if repoId != sourceURL {
		fullURL := fmt.Sprintf("https://huggingface.co/%s", repoId)
		if fullURL != sourceURL {
			// Recursively check with full URL
			return h.findModelBySourceURL(ctx, fullURL, workspace)
		}
	}

	return nil, nil
}

// isFullURL checks if the input is a full URL (starts with http:// or https://)
func isFullURL(input string) bool {
	return len(input) > 7 && (input[:7] == "http://" || input[:8] == "https://")
}

// cvtDBModelToInfo converts database model to ModelInfo (for backward compatibility).
func cvtDBModelToInfo(dbModel *dbclient.Model) ModelInfo {
	includeUnmatched := dbModel.AccessMode == string(v1.AccessModeRemoteAPI)

	// Parse local paths from JSON
	var localPaths []LocalPathInfo
	if dbModel.LocalPaths != "" {
		var dbLocalPaths []dbclient.ModelLocalPathDB
		if err := json.Unmarshal([]byte(dbModel.LocalPaths), &dbLocalPaths); err == nil {
			for _, lp := range dbLocalPaths {
				localPaths = append(localPaths, LocalPathInfo{
					Workspace: lp.Workspace,
					Path:      lp.Path,
					Status:    lp.Status,
					Message:   lp.Message,
				})
			}
		}
	}

	return ModelInfo{
		ID:              dbModel.ID,
		DisplayName:     dbModel.DisplayName,
		Description:     dbModel.Description,
		Icon:            dbModel.Icon,
		Label:           dbModel.Label,
		Tags:            dbModel.Tags,
		CategorizedTags: CategorizeTagString(dbModel.Tags, includeUnmatched),
		MaxTokens:       dbModel.MaxTokens,
		Version:         dbModel.Version,
		SourceURL:       dbModel.SourceURL,
		AccessMode:      dbModel.AccessMode,
		ModelName:       dbModel.ModelName,
		Phase:           dbModel.Phase,
		Message:         dbModel.Message,
		Workspace:       dbModel.Workspace,
		S3Path:          dbModel.S3Path,
		LocalPaths:      localPaths,
		CreatedAt:       formatNullTime(dbModel.CreatedAt),
		UpdatedAt:       formatNullTime(dbModel.UpdatedAt),
		DeletionTime:    formatNullTime(dbModel.DeletionTime),
		IsDeleted:       dbModel.IsDeleted,
	}
}

// formatNullTime formats pq.NullTime to RFC3339 string.
func formatNullTime(nt pq.NullTime) string {
	if nt.Valid {
		return nt.Time.Format(time.RFC3339)
	}
	return ""
}
