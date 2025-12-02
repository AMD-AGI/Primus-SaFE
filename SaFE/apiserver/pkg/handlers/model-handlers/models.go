/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/constvar"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
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

	// Check if model with same URL already exists
	// Also check by repo_id to handle both URL and repo_id format
	existingModel, _ := h.findModelBySourceURL(ctx, req.Source.URL)
	if existingModel == nil {
		// Try to find by normalized repo_id (e.g., "microsoft/phi-2" from "https://huggingface.co/microsoft/phi-2")
		repoId := cleanRepoID(req.Source.URL)
		if repoId != req.Source.URL {
			existingModel, _ = h.findModelBySourceURL(ctx, repoId)
		}
		// Also try the full URL if user provided repo_id
		if existingModel == nil && !isFullURL(req.Source.URL) {
			fullURL := fmt.Sprintf("https://huggingface.co/%s", req.Source.URL)
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
		}
		// If model exists but failed, allow re-creation
	}

	var (
		displayName string
		description string
		icon        string
		label       string
		tags        []string
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
			klog.InfoS("Auto-filled model metadata from Hugging Face", "model", hfInfo.DisplayName)
		} else {
			klog.ErrorS(err, "Failed to fetch metadata from Hugging Face", "url", req.Source.URL)
			return nil, commonerrors.NewBadRequest("failed to fetch model info from Hugging Face: " + err.Error())
		}
	} else {
		// Remote API mode: Use user-provided metadata
		if req.DisplayName == "" {
			return nil, commonerrors.NewBadRequest("displayName is required for remote_api mode")
		}
		if req.Label == "" {
			return nil, commonerrors.NewBadRequest("label is required for remote_api mode")
		}
		if req.Description == "" {
			return nil, commonerrors.NewBadRequest("description is required for remote_api mode")
		}
		displayName = req.DisplayName
		description = req.Description
		icon = req.Icon
		label = req.Label
		tags = req.Tags
		klog.InfoS("Using user-provided metadata for remote API model", "displayName", displayName)
	}

	// Generate Name
	name := commonutils.GenerateName("model")

	// 1. If user provided a token, create a Secret first
	var tokenSecretName string
	if req.Source.Token != "" {
		tokenSecretName = name // Secret name: model-xxx
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tokenSecretName,
				Namespace: common.PrimusSafeNamespace,
			},
			StringData: map[string]string{
				"token": req.Source.Token, // Store plaintext token in Secret
			},
			Type: corev1.SecretTypeOpaque,
		}
		if err := h.k8sClient.Create(ctx, secret); err != nil {
			klog.ErrorS(err, "Failed to create token Secret")
			return nil, commonerrors.NewInternalError("failed to create token secret: " + err.Error())
		}
		klog.Infof("Created token Secret: %s", tokenSecretName)
	}

	// 2. Create K8s CR
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
			Source: v1.ModelSource{
				URL:        req.Source.URL,
				AccessMode: v1.AccessMode(req.Source.AccessMode),
			},
		},
	}

	// Handle DownloadTarget (optional)
	if req.DownloadTarget != nil {
		k8sModel.Spec.DownloadTarget = &v1.DownloadTarget{
			Type:      v1.DownloadType(req.DownloadTarget.Type),
			LocalPath: req.DownloadTarget.LocalPath,
		}
		// Handle S3 Config
		if req.DownloadTarget.Type == string(v1.DownloadTypeS3) && req.DownloadTarget.S3Config != nil {
			k8sModel.Spec.DownloadTarget.S3Config = &v1.S3TargetConfig{
				Endpoint:        req.DownloadTarget.S3Config.Endpoint,
				Bucket:          req.DownloadTarget.S3Config.Bucket,
				Region:          req.DownloadTarget.S3Config.Region,
				AccessKeyID:     req.DownloadTarget.S3Config.AccessKeyID,
				SecretAccessKey: req.DownloadTarget.S3Config.SecretAccessKey,
			}
		}
	}

	// Reference the Secret we just created
	if tokenSecretName != "" {
		k8sModel.Spec.Source.Token = &corev1.LocalObjectReference{
			Name: tokenSecretName,
		}
	}

	if err := h.k8sClient.Create(ctx, k8sModel); err != nil {
		klog.ErrorS(err, "Failed to create Model CR")
		return nil, commonerrors.NewInternalError("failed to create model resource: " + err.Error())
	}

	// For remote_api mode, automatically create Inference CR
	if req.Source.AccessMode == string(v1.AccessModeRemoteAPI) {
		userId := c.GetString("userId")
		userName := c.GetString("userName")

		infId := commonutils.GenerateName(name)

		// Normalize displayName for K8s label (must be lowercase alphanumeric, '-', max 45 chars)
		normalizedDisplayName := stringutil.NormalizeForDNS(displayName)

		inference := &v1.Inference{
			ObjectMeta: metav1.ObjectMeta{
				Name: infId,
				Labels: map[string]string{
					v1.InferenceIdLabel: infId,
					v1.UserIdLabel:      userId,
					v1.DisplayNameLabel: normalizedDisplayName,
				},
			},
			Spec: v1.InferenceSpec{
				DisplayName: displayName,
				Description: description,
				UserID:      userId,
				UserName:    userName,
				ModelForm:   constvar.InferenceModelFormAPI,
				ModelName:   name,
				Instance: v1.InferenceInstance{
					BaseUrl: req.Source.URL,
					ApiKey:  req.Source.ApiKey,
				},
				// Resource and Config are empty for remote_api mode
			},
			Status: v1.InferenceStatus{
				Phase:      constvar.InferencePhaseRunning, // Remote API is immediately ready
				UpdateTime: &metav1.Time{Time: time.Now().UTC()},
			},
		}

		if err := h.k8sClient.Create(ctx, inference); err != nil {
			klog.ErrorS(err, "Failed to create Inference for remote API model", "model", name)
			// Don't fail the model creation, just log the error
		} else {
			klog.Infof("Created Inference %s for remote API model %s", infId, name)

			// Update Model status with inference ID
			k8sModel.Status.InferenceID = infId
			k8sModel.Status.InferencePhase = string(constvar.InferencePhaseRunning)
			k8sModel.Status.Phase = v1.ModelPhaseReady // Remote API models are immediately ready
			if err := h.k8sClient.Status().Update(ctx, k8sModel); err != nil {
				klog.ErrorS(err, "Failed to update Model status with inference ID", "model", name)
			}
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

		return model, nil
	}

	return nil, commonerrors.NewInternalError("database client type mismatch")
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

		return &ListModelResponse{
			Total: total,
			Items: models,
		}, nil
	}

	return nil, commonerrors.NewInternalError("database client type mismatch")
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
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId, Namespace: common.PrimusSafeNamespace}, k8sModel); err != nil {
		return nil, commonerrors.NewNotFound("playground model", modelId)
	}

	// For remote_api mode, toggle is not needed - inference is always available
	if k8sModel.IsRemoteAPI() {
		if k8sModel.Status.InferenceID != "" {
			return gin.H{
				"message":     "remote API model is always available, toggle is not needed",
				"inferenceId": k8sModel.Status.InferenceID,
			}, nil
		}
		return gin.H{"message": "remote API model has no inference, please check model status"}, nil
	}

	if req.Enabled {
		// Toggle ON
		if k8sModel.Status.InferenceID != "" {
			return gin.H{"message": "inference already exists", "inferenceId": k8sModel.Status.InferenceID}, nil
		}

		// At this point, we know it's a local model (remote_api already returned above)
		modelForm := constvar.InferenceModelFormModelSquare

		// Validate required fields for local model inference
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
		var inferenceResource v1.InferenceResource
		if req.Resource != nil {
			inferenceResource = v1.InferenceResource{
				Workspace: req.Resource.Workspace,
				Replica:   req.Resource.Replica,
				Cpu:       req.Resource.CPU,
				Memory:    req.Resource.Memory,
				Gpu:       req.Resource.GPU,
			}
		}

		// Build Config from request
		var inferenceConfig v1.InferenceConfig
		if req.Config != nil {
			inferenceConfig = v1.InferenceConfig{
				Image:      req.Config.Image,
				EntryPoint: req.Config.EntryPoint,
				ModelPath:  req.Config.ModelPath,
			}
		}

		// Generate new inference ID
		infId := commonutils.GenerateName(modelId)

		// Normalize displayName for K8s label (must be lowercase alphanumeric, '-', max 45 chars)
		normalizedDisplayName := stringutil.NormalizeForDNS(k8sModel.Spec.DisplayName)

		// Create Inference CRD
		inference := &v1.Inference{
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
				ModelForm:   modelForm,
				ModelName:   modelId,
				Resource:    inferenceResource,
				Config:      inferenceConfig,
			},
			Status: v1.InferenceStatus{
				Phase:      constvar.InferencePhasePending,
				UpdateTime: &metav1.Time{Time: time.Now().UTC()},
			},
		}

		if err := h.k8sClient.Create(ctx, inference); err != nil {
			klog.ErrorS(err, "Failed to create inference", "id", infId)
			return nil, commonerrors.NewInternalError("failed to start inference: " + err.Error())
		}

		// Update Model Status with InferenceID
		k8sModel.Status.InferenceID = infId
		if err := h.k8sClient.Status().Update(ctx, k8sModel); err != nil {
			klog.ErrorS(err, "Failed to update model status with inference ID", "model", modelId)
		}

		return gin.H{"message": "inference started", "inferenceId": infId}, nil

	} else {
		// Toggle OFF
		if k8sModel.Status.InferenceID == "" {
			return gin.H{"message": "inference not found or already stopped"}, nil
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
				return gin.H{"message": "inference already deleted"}, nil
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
func (h *Handler) deleteModel(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	ctx := c.Request.Context()

	var tokenSecretName string

	// 1. Check if model exists in K8s and get token secret name
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId, Namespace: common.PrimusSafeNamespace}, k8sModel); err != nil {
		if errors.IsNotFound(err) {
			// Model not found in K8s, try to delete from DB only
			klog.InfoS("Model not found in K8s, attempting DB deletion only", "model", modelId)
		} else {
			klog.ErrorS(err, "Failed to get model from K8s", "model", modelId)
			return nil, commonerrors.NewInternalError("failed to fetch model: " + err.Error())
		}
	} else {
		// Get token secret name from Model spec
		if k8sModel.Spec.Source.Token != nil {
			tokenSecretName = k8sModel.Spec.Source.Token.Name
		}

		// 2. Delete associated Secret if exists
		if tokenSecretName != "" {
			secret := &corev1.Secret{}
			if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: tokenSecretName, Namespace: common.PrimusSafeNamespace}, secret); err != nil {
				if !errors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to get token secret", "secret", tokenSecretName)
				}
			} else {
				if err := h.k8sClient.Delete(ctx, secret); err != nil {
					if !errors.IsNotFound(err) {
						klog.ErrorS(err, "Failed to delete token secret", "secret", tokenSecretName)
						// Continue with model deletion even if secret deletion fails
					}
				} else {
					klog.InfoS("Token secret deleted", "secret", tokenSecretName)
				}
			}
		}

		// 3. Delete K8s Model CR (will cascade delete Job via OwnerReference)
		if err := h.k8sClient.Delete(ctx, k8sModel); err != nil {
			if !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete model from K8s", "model", modelId)
				return nil, commonerrors.NewInternalError("failed to delete model: " + err.Error())
			}
		}
		klog.InfoS("Model deleted from K8s", "model", modelId)
	}

	return gin.H{"message": "model deleted successfully", "id": modelId}, nil
}

// findModelBySourceURL checks if a model with the given source URL already exists in the database.
// Returns the existing model if found, nil otherwise.
func (h *Handler) findModelBySourceURL(ctx context.Context, sourceURL string) (*dbclient.Model, error) {
	client, ok := h.dbClient.(*dbclient.Client)
	if !ok {
		return nil, fmt.Errorf("database client type mismatch")
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
