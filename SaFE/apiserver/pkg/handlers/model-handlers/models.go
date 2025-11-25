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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// CreatePlaygroundModel handles the creation of a new playground model.
func (h *Handler) CreatePlaygroundModel(c *gin.Context) {
	handle(c, h.createPlaygroundModel)
}

// ListPlaygroundModels handles listing of playground models.
func (h *Handler) ListPlaygroundModels(c *gin.Context) {
	handle(c, h.listPlaygroundModels)
}

// GetPlaygroundModel handles getting a single playground model by ID.
func (h *Handler) GetPlaygroundModel(c *gin.Context) {
	handle(c, h.getPlaygroundModel)
}

// TogglePlaygroundModel handles enabling/disabling (start/stop) a model instance for the user.
func (h *Handler) TogglePlaygroundModel(c *gin.Context) {
	handle(c, h.togglePlaygroundModel)
}

// DeletePlaygroundModel handles the deletion of a playground model.
func (h *Handler) DeletePlaygroundModel(c *gin.Context) {
	handle(c, h.deletePlaygroundModel)
}

// createPlaygroundModel implements the model creation logic.
func (h *Handler) createPlaygroundModel(c *gin.Context) (interface{}, error) {
	var req CreatePlaygroundModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	ctx := context.Background()

	// Validate URL first
	if req.Source.URL == "" {
		return nil, commonerrors.NewBadRequest("model source url is required")
	}

	var (
		displayName string
		description string
		icon        string
		label       string
		tags        []string
	)

	// 0. Auto-fill metadata from Hugging Face
	// We assume if user provides a simple "org/repo" string as URL, it's HF.
	if strings.Contains(req.Source.URL, "huggingface.co") {
		if hfInfo, err := GetHFModelInfo(req.Source.URL); err == nil {
			displayName = hfInfo.DisplayName
			description = hfInfo.Description
			icon = hfInfo.Icon
			label = hfInfo.Label
			tags = hfInfo.Tags
			klog.InfoS("Auto-filled model metadata from Hugging Face", "model", hfInfo.DisplayName)
		} else {
			klog.ErrorS(err, "Failed to fetch metadata from Hugging Face", "url", req.Source.URL)
			return nil, commonerrors.NewBadRequest("Can't got model info")
		}
	} else {
		return nil, commonerrors.NewBadRequest("Only support Hugging Face Now.")
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

	// Handle Resources
	if req.Resources != nil {
		cpuInt := 0
		if req.Resources.CPU != "" {
			if q, err := resource.ParseQuantity(req.Resources.CPU); err == nil {
				cpuInt = int(q.Value()) // Returns int64, cast to int
			}
		}
		memInt := 0
		if req.Resources.Memory != "" {
			if q, err := resource.ParseQuantity(req.Resources.Memory); err == nil {
				memInt = int(q.Value() / (1024 * 1024 * 1024)) // Convert bytes to GiB
			}
		}

		k8sModel.Spec.Resource = v1.InferenceResource{
			Cpu:    cpuInt,
			Memory: memInt,
			Gpu:    req.Resources.GPU,
		}
	}

	if err := h.k8sClient.Create(ctx, k8sModel); err != nil {
		klog.ErrorS(err, "Failed to create Model CR")
		return nil, commonerrors.NewInternalError("failed to create model resource: " + err.Error())
	}

	return &CreateResponse{ID: name}, nil
}

// getPlaygroundModel implements the logic to get a single model by ID.
func (h *Handler) getPlaygroundModel(c *gin.Context) (interface{}, error) {
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

func parseListPlaygroundModelQuery(c *gin.Context) (*ListPlaygroundModelQuery, error) {
	query := &ListPlaygroundModelQuery{}
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

// listPlaygroundModels implements the model listing logic with aggregated inference status.
func (h *Handler) listPlaygroundModels(c *gin.Context) (interface{}, error) {
	// Parse query parameters for filtering
	queryArgs, err := parseListPlaygroundModelQuery(c)
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

		return &ListPlaygroundModelResponse{
			Total: total,
			Items: models,
		}, nil
	}

	return nil, commonerrors.NewInternalError("database client type mismatch")
}

// togglePlaygroundModel handles enabling/disabling an inference service for the model.
func (h *Handler) togglePlaygroundModel(c *gin.Context) (interface{}, error) {
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

	if req.Enabled {
		// Toggle ON
		if k8sModel.Status.InferenceID != "" {
			return gin.H{"message": "inference already exists", "inferenceId": k8sModel.Status.InferenceID}, nil
		}

		// Generate new inference ID
		infId := commonutils.GenerateName(modelId)

		// Determine ModelForm based on AccessMode
		var modelForm constvar.InferenceModelForm
		if k8sModel.IsRemoteAPI() {
			modelForm = constvar.InferenceModelFormAPI
		} else {
			modelForm = constvar.InferenceModelFormModelSquare
		}

		// Create Inference CRD
		inference := &v1.Inference{
			ObjectMeta: metav1.ObjectMeta{
				Name: infId,
				Labels: map[string]string{
					v1.InferenceIdLabel: infId,
					v1.UserIdLabel:      userId,
				},
				Annotations: map[string]string{
					v1.DisplayNameLabel: k8sModel.Spec.DisplayName,
				},
			},
			Spec: v1.InferenceSpec{
				DisplayName: k8sModel.Spec.DisplayName,
				Description: k8sModel.Spec.Description,
				UserID:      userId,
				UserName:    userName,
				ModelForm:   modelForm,
				ModelName:   modelId,
				Resource:    k8sModel.Spec.Resource,
				Config:      k8sModel.Spec.Config,
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
				_ = h.k8sClient.Status().Update(ctx, k8sModel)
				return gin.H{"message": "inference already deleted"}, nil
			}
			return nil, commonerrors.NewInternalError("failed to fetch inference for deletion: " + err.Error())
		}

		if err := h.k8sClient.Delete(ctx, k8sInf); err != nil {
			return nil, commonerrors.NewInternalError("failed to stop inference: " + err.Error())
		}

		// Clear Model Status InferenceID
		k8sModel.Status.InferenceID = ""
		if err := h.k8sClient.Status().Update(ctx, k8sModel); err != nil {
			klog.ErrorS(err, "Failed to clear model status inference ID", "model", modelId)
			return nil, commonerrors.NewInternalError("failed to Update models: " + err.Error())
		}

		return gin.H{"message": "inference stopped"}, nil
	}
}

// deletePlaygroundModel implements the model deletion logic.
func (h *Handler) deletePlaygroundModel(c *gin.Context) (interface{}, error) {
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
