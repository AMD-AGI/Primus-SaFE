/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/constvar"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
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

// ToggleModel handles enabling/disabling (start/stop) a model instance for the user.
func (h *Handler) ToggleModel(c *gin.Context) {
	handle(c, h.toggleModel)
}

// DeleteModel handles the deletion of a playground model.
func (h *Handler) DeleteModel(c *gin.Context) {
	handle(c, h.deleteModel)
}

// RetryModel handles retrying a failed model download.
func (h *Handler) RetryModel(c *gin.Context) {
	handle(c, h.retryModel)
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
		// Check with normalized URL first
		existingModel, _ := h.findModelBySourceURL(ctx, normalizedURL)
		if existingModel == nil {
			// Also try with trailing slash (in case stored with slash)
			existingModel, _ = h.findModelBySourceURL(ctx, normalizedURL+"/")
		}

		// For local models, also check by repo_id to handle both URL and repo_id format
		if existingModel == nil {
			// Try to find by normalized repo_id (e.g., "microsoft/phi-2" from "https://huggingface.co/microsoft/phi-2")
			repoId := cleanRepoID(normalizedURL)
			if repoId != normalizedURL {
				existingModel, _ = h.findModelBySourceURL(ctx, repoId)
			}
			// Also try the full URL if user provided repo_id
			if existingModel == nil && !isFullURL(normalizedURL) {
				fullURL := fmt.Sprintf("https://huggingface.co/%s", normalizedURL)
				existingModel, _ = h.findModelBySourceURL(ctx, fullURL)
			}
		}

		if existingModel != nil {
			if existingModel.Phase == string(v1.ModelPhaseReady) {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("model with URL '%s' already exists and is ready (id: %s)", existingModel.SourceURL, existingModel.ID))
			} else if existingModel.Phase == string(v1.ModelPhasePulling) {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("model with URL '%s' is currently being downloaded (id: %s)", existingModel.SourceURL, existingModel.ID))
			} else if existingModel.Phase == string(v1.ModelPhasePending) {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("model with URL '%s' already exists and is pending (id: %s)", existingModel.SourceURL, existingModel.ID))
			} else if existingModel.Phase == string(v1.ModelPhaseFailed) {
				// If model exists but failed, user should use retry API instead of creating a new one
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
	)

	// 0. Handle metadata based on AccessMode
	if req.Source.AccessMode == string(v1.AccessModeLocal) {
		// Local mode: Auto-fill metadata from Hugging Face
		// Supports both full URL (https://huggingface.co/microsoft/phi-2) and repo_id format (microsoft/phi-2)
		if hfInfo, err := GetHFModelInfo(req.Source.URL); err == nil {
			displayName = hfInfo.DisplayName
			description = hfInfo.Description
			icon = hfInfo.Icon
			label = hfInfo.Label
			tags = hfInfo.Tags
			maxTokens = hfInfo.MaxTokens
			klog.InfoS("Auto-filled model metadata from Hugging Face", "model", hfInfo.DisplayName, "maxTokens", maxTokens)
		} else {
			klog.ErrorS(err, "Failed to fetch metadata from Hugging Face", "url", req.Source.URL)
			return nil, commonerrors.NewBadRequest("failed to fetch model info from Hugging Face: " + err.Error())
		}
	} else {
		// Remote API mode: Use user-provided metadata
		if req.DisplayName == "" {
			return nil, commonerrors.NewBadRequest("displayName is required for remote_api mode")
		}
		displayName = req.DisplayName
		description = req.Description // Optional
		icon = req.Icon               // Optional
		label = req.Label             // Optional
		tags = req.Tags               // Optional
		maxTokens = req.MaxTokens     // Optional
		klog.InfoS("Using user-provided metadata for remote API model", "displayName", displayName)
	}

	// Generate Name
	name := commonutils.GenerateName("model")

	// 1. Create K8s Model CR first
	k8sModel := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.PrimusSafeNamespace,
		},
		Spec: v1.ModelSpec{
			DisplayName: displayName,
			Description: description,
			Icon:        icon,
			Label:       label,
			Tags:        tags,
			MaxTokens:   maxTokens,
			Source: v1.ModelSource{
				URL:        normalizedURL, // Use normalized URL for consistent duplicate detection
				AccessMode: v1.AccessMode(req.Source.AccessMode),
			},
		},
	}

	if err := h.k8sClient.Create(ctx, k8sModel); err != nil {
		klog.ErrorS(err, "Failed to create Model CR")
		return nil, commonerrors.NewInternalError("failed to create model resource: " + err.Error())
	}

	// 2. If user provided a token, create a Secret with Model as owner
	if req.Source.Token != "" {
		tokenSecretName := name // Secret name: model-xxx
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tokenSecretName,
				Namespace: common.PrimusSafeNamespace,
				Labels: map[string]string{
					v1.ModelIdLabel: name, // Label for manual cleanup when Model is deleted
				},
			},
			StringData: map[string]string{
				"token": req.Source.Token, // Store plaintext token in Secret
			},
			Type: corev1.SecretTypeOpaque,
		}

		// Note: Cannot use OwnerReference here because Model is cluster-scoped
		// and Secret is namespace-scoped. Will delete manually in deleteModel.

		if err := h.k8sClient.Create(ctx, secret); err != nil {
			klog.ErrorS(err, "Failed to create token Secret")
			// Clean up: delete the model we just created
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

	return &CreateResponse{ID: name}, nil
}

// getModel implements the logic to get a single model by ID.
func (h *Handler) getModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	if client, ok := h.dbClient.(*dbclient.Client); ok {
		db, err := client.GetGormDB()
		if err != nil {
			return nil, commonerrors.NewInternalError("database connection failed")
		}

		// 1. Fetch the model
		var model dbclient.Model
		if err := db.Where("id = ?", modelId).First(&model).Error; err != nil {
			return nil, commonerrors.NewNotFound("playground model", modelId)
		}

		modelInfo := cvtDBModelToInfo(&model)

		// 2. If model has an inference, fetch WorkloadID from Inference CR
		if model.InferenceID != "" {
			inference := &v1.Inference{}
			if err := h.k8sClient.Get(c.Request.Context(), ctrlclient.ObjectKey{Name: model.InferenceID}, inference); err == nil {
				modelInfo.WorkloadID = inference.Spec.Instance.WorkloadID
			}
		}

		return modelInfo, nil
	}

	// Fallback to K8s API
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(c.Request.Context(), ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		return nil, commonerrors.NewNotFound("playground model", modelId)
	}

	// Convert K8s Model to response format
	return h.convertK8sModelToResponse(k8sModel), nil
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

// listModels implements the model listing logic with aggregated inference status.
func (h *Handler) listModels(c *gin.Context) (interface{}, error) {
	// Parse query parameters for filtering
	queryArgs, err := parseListModelQuery(c)
	if err != nil {
		return nil, err
	}

	if client, ok := h.dbClient.(*dbclient.Client); ok {
		db, err := client.GetGormDB()
		if err != nil {
			return nil, commonerrors.NewInternalError("database connection failed")
		}

		// Build query
		query := db.Model(&dbclient.Model{})

		// Default filter: exclude deleted models
		query = query.Where("is_deleted = ?", false)

		if queryArgs.InferenceStatus != "" {
			query = query.Where("inference_phase = ?", queryArgs.InferenceStatus)
		}
		if queryArgs.AccessMode != "" {
			query = query.Where("access_mode = ?", queryArgs.AccessMode)
		}

		// 1. Count total
		var total int64
		if err := query.Count(&total).Error; err != nil {
			return nil, commonerrors.NewInternalError("failed to count models")
		}

		// 2. Fetch models with pagination
		var models []dbclient.Model
		if err := query.Limit(queryArgs.Limit).Offset(queryArgs.Offset).Find(&models).Error; err != nil {
			return nil, commonerrors.NewInternalError("failed to fetch models")
		}

		// Convert to ModelInfo
		items := make([]ModelInfo, len(models))
		for i, model := range models {
			items[i] = cvtDBModelToInfo(&model)
		}

		return &ListModelResponse{
			Total: total,
			Items: items,
		}, nil
	}

	// Fallback to K8s API
	k8sModelList := &v1.ModelList{}
	if err := h.k8sClient.List(c.Request.Context(), k8sModelList); err != nil {
		return nil, commonerrors.NewInternalError("failed to list models from K8s: " + err.Error())
	}

	// Filter and convert models
	var items []interface{}
	for _, k8sModel := range k8sModelList.Items {
		// Apply filters
		if queryArgs.AccessMode != "" && string(k8sModel.Spec.Source.AccessMode) != queryArgs.AccessMode {
			continue
		}
		if queryArgs.InferenceStatus != "" && k8sModel.Status.InferencePhase != queryArgs.InferenceStatus {
			continue
		}
		items = append(items, h.convertK8sModelToResponse(&k8sModel))
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

	return gin.H{
		"total": total,
		"items": items[start:end],
	}, nil
}

// convertK8sModelToResponse converts a K8s Model CR to API response format
func (h *Handler) convertK8sModelToResponse(k8sModel *v1.Model) gin.H {
	return gin.H{
		"id":             k8sModel.Name,
		"displayName":    k8sModel.Spec.DisplayName,
		"description":    k8sModel.Spec.Description,
		"icon":           k8sModel.Spec.Icon,
		"label":          k8sModel.Spec.Label,
		"tags":           k8sModel.Spec.Tags,
		"maxTokens":      k8sModel.Spec.MaxTokens,
		"accessMode":     k8sModel.Spec.Source.AccessMode,
		"url":            k8sModel.Spec.Source.URL,
		"phase":          k8sModel.Status.Phase,
		"inferenceId":    k8sModel.Status.InferenceID,
		"inferencePhase": k8sModel.Status.InferencePhase,
		"creationTime":   k8sModel.CreationTimestamp.Time,
	}
}

// toggleModel handles enabling/disabling an inference service for the model.
func (h *Handler) toggleModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	var req ToggleModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	userId := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	ctx := c.Request.Context()

	// Fetch Model Info first to check status
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		return nil, commonerrors.NewNotFound("playground model", modelId)
	}

	if req.Enabled {
		// Toggle ON
		if k8sModel.Status.InferenceID != "" {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("inference already exists, inferenceId: %s", k8sModel.Status.InferenceID))
		}

		// Generate new inference ID
		infId := commonutils.GenerateName(modelId)

		// Normalize displayName for K8s label (must be lowercase alphanumeric, '-', max 45 chars)
		normalizedDisplayName := stringutil.NormalizeForDNS(k8sModel.Spec.DisplayName)

		var inference *v1.Inference

		if k8sModel.IsRemoteAPI() {
			// Remote API mode: create API-type inference
			if req.Instance == nil || req.Instance.ApiKey == "" {
				return nil, commonerrors.NewBadRequest("instance.apiKey is required for remote_api model")
			}

			apiKeySecretName := infId // Secret name same as inference ID

			// 1. Create Inference first (without ApiKey reference)
			inference = &v1.Inference{
				ObjectMeta: metav1.ObjectMeta{
					Name: infId,
					Labels: map[string]string{
						v1.InferenceIdLabel: infId,
						v1.UserIdLabel:      userId,
						v1.DisplayNameLabel: normalizedDisplayName,
					},
				},
				Spec: v1.InferenceSpec{
					DisplayName: k8sModel.Spec.DisplayName,
					Description: k8sModel.Spec.Description,
					UserID:      userId,
					UserName:    userName,
					ModelForm:   constvar.InferenceModelFormAPI,
					ModelName:   modelId,
					Instance: v1.InferenceInstance{
						ApiKey:  &corev1.LocalObjectReference{Name: apiKeySecretName},
						BaseUrl: k8sModel.Spec.Source.URL,
						Model:   req.Instance.Model, // Optional: model name for API calls
					},
				},
				Status: v1.InferenceStatus{
					Phase:      constvar.InferencePhaseRunning, // Remote API is immediately ready
					UpdateTime: &metav1.Time{Time: time.Now().UTC()},
				},
			}

			// Set Model as owner of Inference for automatic cascade deletion
			if err := controllerutil.SetControllerReference(k8sModel, inference, h.k8sClient.Scheme()); err != nil {
				klog.ErrorS(err, "Failed to set owner reference", "model", modelId, "inference", infId)
				return nil, commonerrors.NewInternalError("failed to set owner reference: " + err.Error())
			}

			if err := h.k8sClient.Create(ctx, inference); err != nil {
				klog.ErrorS(err, "Failed to create inference", "id", infId)
				return nil, commonerrors.NewInternalError("failed to start inference: " + err.Error())
			}

			// Update Status separately (Status is a subresource, not saved by Create)
			inference.Status.Phase = constvar.InferencePhaseRunning
			inference.Status.Message = "Inference service is running"
			inference.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
			if err := h.k8sClient.Status().Update(ctx, inference); err != nil {
				klog.ErrorS(err, "Failed to update inference status to Running", "inference", infId)
				// Don't fail the whole operation, controller will reconcile
			}

			// 2. Create ApiKey Secret (without OwnerReference - will be deleted manually)
			// Note: Cannot use OwnerReference here because Inference is cluster-scoped
			// and Secret is namespace-scoped.
			apiKeySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      apiKeySecretName,
					Namespace: common.PrimusSafeNamespace,
					Labels: map[string]string{
						v1.InferenceIdLabel: infId,
						v1.UserIdLabel:      userId,
					},
				},
				Type: corev1.SecretTypeOpaque,
				StringData: map[string]string{
					"apiKey": req.Instance.ApiKey,
				},
			}

			if err := h.k8sClient.Create(ctx, apiKeySecret); err != nil && !errors.IsAlreadyExists(err) {
				klog.ErrorS(err, "Failed to create API key secret", "secret", apiKeySecretName)
				_ = h.k8sClient.Delete(ctx, inference)
				return nil, commonerrors.NewInternalError("failed to create API key secret: " + err.Error())
			}

			// Update Model Status with InferenceID
			k8sModel.Status.InferenceID = infId
			k8sModel.Status.InferencePhase = string(inference.Status.Phase)
			if err := h.k8sClient.Status().Update(ctx, k8sModel); err != nil {
				klog.ErrorS(err, "Failed to update model status with inference ID", "model", modelId)
			}

			klog.InfoS("Toggle ON (remote_api): inference created", "model", modelId, "inference", infId)
			return gin.H{"message": "inference started", "inferenceId": infId}, nil
		} else {
			// Local mode: create ModelSquare-type inference
			if req.Resource == nil || req.Config == nil {
				return nil, commonerrors.NewBadRequest("resource and config are required for local model inference")
			}
			var missingFields []string
			if req.Resource.Workspace == "" {
				missingFields = append(missingFields, "workspace")
			}
			if req.Resource.Replica <= 0 {
				missingFields = append(missingFields, "replica")
			}
			if req.Config.Image == "" {
				missingFields = append(missingFields, "image")
			}
			if req.Config.EntryPoint == "" {
				missingFields = append(missingFields, "entryPoint")
			}
			if len(missingFields) > 0 {
				return nil, commonerrors.NewBadRequest(fmt.Sprintf("missing required fields for inference: %v", missingFields))
			}

			// Build Resource from request
			inferenceResource := v1.InferenceResource{
				Workspace: req.Resource.Workspace,
				Replica:   req.Resource.Replica,
				Cpu:       req.Resource.CPU,
				Memory:    req.Resource.Memory,
				Gpu:       req.Resource.GPU,
			}

			// Build Config from request
			inferenceConfig := v1.InferenceConfig{
				Image:      req.Config.Image,
				EntryPoint: req.Config.EntryPoint,
				ModelPath:  req.Config.ModelPath,
			}

			inference = &v1.Inference{
				ObjectMeta: metav1.ObjectMeta{
					Name: infId,
					Labels: map[string]string{
						v1.InferenceIdLabel: infId,
						v1.UserIdLabel:      userId,
						v1.DisplayNameLabel: normalizedDisplayName,
					},
				},
				Spec: v1.InferenceSpec{
					DisplayName: k8sModel.Spec.DisplayName,
					Description: k8sModel.Spec.Description,
					UserID:      userId,
					UserName:    userName,
					ModelForm:   constvar.InferenceModelFormModelSquare,
					ModelName:   modelId,
					Resource:    inferenceResource,
					Config:      inferenceConfig,
				},
				Status: v1.InferenceStatus{
					Phase:      constvar.InferencePhasePending,
					UpdateTime: &metav1.Time{Time: time.Now().UTC()},
				},
			}
		}

		// Set Model as owner of Inference for automatic cascade deletion
		if err := controllerutil.SetControllerReference(k8sModel, inference, h.k8sClient.Scheme()); err != nil {
			klog.ErrorS(err, "Failed to set owner reference", "model", modelId, "inference", infId)
			return nil, commonerrors.NewInternalError("failed to set owner reference: " + err.Error())
		}

		if err := h.k8sClient.Create(ctx, inference); err != nil {
			klog.ErrorS(err, "Failed to create inference", "id", infId)
			return nil, commonerrors.NewInternalError("failed to start inference: " + err.Error())
		}

		// Update Model Status with InferenceID
		k8sModel.Status.InferenceID = infId
		k8sModel.Status.InferencePhase = string(inference.Status.Phase)
		if err := h.k8sClient.Status().Update(ctx, k8sModel); err != nil {
			klog.ErrorS(err, "Failed to update model status with inference ID", "model", modelId)
		}

		return gin.H{"message": "inference started", "inferenceId": infId}, nil

	} else {
		// Toggle OFF
		if k8sModel.Status.InferenceID == "" {
			return nil, commonerrors.NewBadRequest("inference not found or already stopped")
		}

		infId := k8sModel.Status.InferenceID

		// Delete Inference
		k8sInf := &v1.Inference{}
		if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: infId}, k8sInf); err != nil {
			if errors.IsNotFound(err) {
				// Inference already gone, just clean up status
				k8sModel.Status.InferenceID = ""
				k8sModel.Status.InferencePhase = ""
				_ = h.k8sClient.Status().Update(ctx, k8sModel)
				return nil, commonerrors.NewBadRequest("inference already deleted")
			}
			return nil, commonerrors.NewInternalError("failed to fetch inference for deletion: " + err.Error())
		}

		if err := h.k8sClient.Delete(ctx, k8sInf); err != nil {
			return nil, commonerrors.NewInternalError("failed to stop inference: " + err.Error())
		}

		// Clear Model Status InferenceID and InferencePhase
		k8sModel.Status.InferenceID = ""
		k8sModel.Status.InferencePhase = ""
		if err := h.k8sClient.Status().Update(ctx, k8sModel); err != nil {
			klog.ErrorS(err, "Failed to clear model status inference ID", "model", modelId)
			return nil, commonerrors.NewInternalError("failed to Update models: " + err.Error())
		}

		return gin.H{"message": "inference stopped"}, nil
	}
}

// deleteModel implements the model deletion logic.
// Note: Inference will be automatically deleted by Kubernetes garbage collection (OwnerReference)
// Token Secret must be deleted manually (cluster-scoped owner cannot own namespace-scoped resource)
func (h *Handler) deleteModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	ctx := c.Request.Context()

	// Check if model exists in K8s
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		if errors.IsNotFound(err) {
			// Model not found in K8s, try to delete from DB only
			klog.InfoS("Model not found in K8s, attempting DB deletion only", "model", modelId)
		} else {
			klog.ErrorS(err, "Failed to get model from K8s", "model", modelId)
			return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
		}
	} else {
		// 1. Delete Token Secret manually (if exists)
		// Token Secret cannot be deleted via OwnerReference because Model is cluster-scoped
		// and Secret is namespace-scoped
		if k8sModel.Spec.Source.Token != nil && k8sModel.Spec.Source.Token.Name != "" {
			tokenSecret := &corev1.Secret{}
			tokenSecretKey := ctrlclient.ObjectKey{
				Name:      k8sModel.Spec.Source.Token.Name,
				Namespace: common.PrimusSafeNamespace,
			}
			if err := h.k8sClient.Get(ctx, tokenSecretKey, tokenSecret); err != nil {
				if !errors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to get token secret", "secret", tokenSecretKey.Name)
				}
			} else {
				if err := h.k8sClient.Delete(ctx, tokenSecret); err != nil && !errors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete token secret", "secret", tokenSecretKey.Name)
				} else {
					klog.InfoS("Token secret deleted", "secret", tokenSecretKey.Name, "model", modelId)
				}
			}
		}

		// 2. Delete K8s Model CR
		// Inference will be automatically deleted via OwnerReference (both are cluster-scoped)
		if err := h.k8sClient.Delete(ctx, k8sModel); err != nil {
			if !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete model from K8s", "model", modelId)
				return nil, commonerrors.NewInternalError("failed to delete model: " + err.Error())
			}
		}
		klog.InfoS("Model deleted from K8s (cascade: Inference; manual: Token Secret)", "model", modelId)
	}

	return gin.H{"message": "model deleted successfully", "id": modelId}, nil
}

// retryModel implements the logic to retry a failed model download.
// It resets the model phase from Failed to Pending, allowing the controller to restart the download.
func (h *Handler) retryModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

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

	// 2. Check if model is in Failed phase
	if k8sModel.Status.Phase != v1.ModelPhaseFailed {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("model is not in Failed phase, current phase: %s. Only failed models can be retried", k8sModel.Status.Phase))
	}

	// 3. Reset model status to Pending
	// The model controller will detect this change and restart the download process
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

// findModelBySourceURL checks if a model with the given source URL already exists.
// Checks both K8s and database (if available). Returns the existing model if found, nil otherwise.
func (h *Handler) findModelBySourceURL(ctx context.Context, sourceURL string) (*dbclient.Model, error) {
	// First, check K8s directly for immediate consistency (no sync delay)
	// This is critical for detecting duplicates when user creates models quickly
	modelList := &v1.ModelList{}
	if err := h.k8sClient.List(ctx, modelList); err == nil {
		for _, m := range modelList.Items {
			if m.Spec.Source.URL == sourceURL && m.DeletionTimestamp == nil {
				// Found in K8s, convert to dbclient.Model for consistent return type
				return &dbclient.Model{
					ID:        m.Name,
					SourceURL: m.Spec.Source.URL,
					Phase:     string(m.Status.Phase),
				}, nil
			}
		}
	}

	// Also check database for models that might not be in K8s yet or soft-deleted
	// Skip if database is not enabled
	client, ok := h.dbClient.(*dbclient.Client)
	if !ok {
		// No database client, return nil (not found in K8s)
		return nil, nil
	}

	db, err := client.GetGormDB()
	if err != nil {
		return nil, err
	}

	var model dbclient.Model
	// Search for non-deleted model with the same source URL
	if err := db.Where("source_url = ? AND is_deleted = ?", sourceURL, false).First(&model).Error; err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		// For gorm, check if it's a "record not found" error
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, err
	}

	return &model, nil
}

// isFullURL checks if the input is a full URL (starts with http:// or https://)
func isFullURL(input string) bool {
	return len(input) > 7 && (input[:7] == "http://" || input[:8] == "https://")
}

// cvtDBModelToInfo converts database model to ModelInfo.
func cvtDBModelToInfo(dbModel *dbclient.Model) ModelInfo {
	// Determine whether to include unmatched tags based on access mode:
	// - remote_api: include all tags (user-defined, may have custom tags)
	// - local: only include matched tags (filtered from HuggingFace)
	includeUnmatched := dbModel.AccessMode == string(v1.AccessModeRemoteAPI)

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
		Phase:           dbModel.Phase,
		Message:         dbModel.Message,
		InferenceID:     dbModel.InferenceID,
		InferencePhase:  dbModel.InferencePhase,
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
