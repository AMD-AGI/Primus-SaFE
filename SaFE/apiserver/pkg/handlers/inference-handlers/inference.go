/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package inference_handlers

import (
	"database/sql"
	"fmt"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lib/pq"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/constvar"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

// CreateInference handles the creation of a new inference service.
func (h *Handler) CreateInference(c *gin.Context) {
	handle(c, h.createInference)
}

// ListInference handles listing inference services with filtering and pagination.
func (h *Handler) ListInference(c *gin.Context) {
	handle(c, h.listInference)
}

// GetInference retrieves detailed information about a specific inference service.
func (h *Handler) GetInference(c *gin.Context) {
	handle(c, h.getInference)
}

// DeleteInference handles deletion of a single inference service.
func (h *Handler) DeleteInference(c *gin.Context) {
	handle(c, h.deleteInference)
}

// PatchInference handles partial updates to an inference service.
func (h *Handler) PatchInference(c *gin.Context) {
	handle(c, h.patchInference)
}

// createInference implements the inference creation logic.
func (h *Handler) createInference(c *gin.Context) (interface{}, error) {
	req := &CreateInferenceRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	userId := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Generate inference ID (similar to workload naming)
	inferenceId := commonutils.GenerateName(fmt.Sprintf("inf-%s", req.DisplayName))

	// Create inference object
	inference := &v1.Inference{
		ObjectMeta: metav1.ObjectMeta{
			Name: inferenceId,
			Labels: map[string]string{
				v1.InferenceIdLabel: inferenceId,
				v1.UserIdLabel:      userId,
			},
			Annotations: map[string]string{
				v1.DisplayNameLabel: req.DisplayName,
			},
		},
		Spec: v1.InferenceSpec{
			DisplayName: req.DisplayName,
			Description: req.Description,
			UserID:      userId,
			UserName:    userName,
			ModelForm:   constvar.InferenceModelForm(req.ModelForm),
			ModelName:   req.ModelName,
			Instance:    req.Instance,
			Resource:    req.Resource,
			Config:      req.Config,
		},
		Status: v1.InferenceStatus{
			Phase:      constvar.InferencePhasePending,
			UpdateTime: &metav1.Time{Time: time.Now().UTC()},
		},
	}

	// Create in etcd
	if err := h.k8sClient.Create(c.Request.Context(), inference); err != nil {
		klog.ErrorS(err, "failed to create inference", "inference", inferenceId)
		return nil, err
	}

	klog.Infof("created inference: %s, user: %s/%s, model: %s", inferenceId, userName, userId, req.ModelName)
	return &CreateInferenceResponse{InferenceId: inferenceId}, nil
}

// listInference implements the inference listing logic.
func (h *Handler) listInference(c *gin.Context) (interface{}, error) {
	query, err := parseListInferenceQuery(c)
	if err != nil {
		return nil, err
	}

	// Build database query
	dbTags := dbclient.GetInferenceFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
	}

	// Filter by userId if provided (optional)
	if query.UserId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "UserId"): query.UserId})
	}

	if query.ModelForm != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "ModelForm"): query.ModelForm})
	}
	if query.Phase != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Phase"): query.Phase})
	}

	orderBy := []string{fmt.Sprintf("%s DESC", dbclient.GetFieldTag(dbTags, "CreationTime"))}

	ctx := c.Request.Context()
	inferences, err := h.dbClient.SelectInferences(ctx, dbSql, orderBy, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}

	count, err := h.dbClient.CountInferences(ctx, dbSql)
	if err != nil {
		return nil, err
	}

	items := make([]InferenceInfo, 0, len(inferences))
	for _, inf := range inferences {
		items = append(items, cvtDBInferenceToInfo(inf))
	}

	return &ListInferenceResponse{
		Total: count,
		Items: items,
	}, nil
}

// getInference implements the inference retrieval logic.
func (h *Handler) getInference(c *gin.Context) (interface{}, error) {
	inferenceId := c.Param("id")
	if inferenceId == "" {
		return nil, commonerrors.NewBadRequest("inference id is required")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Get from database
	dbInference, err := h.dbClient.GetInference(c.Request.Context(), inferenceId)
	if err != nil {
		return nil, err
	}

	// Check ownership
	if dbInference.UserId != userId {
		return nil, commonerrors.NewForbidden("not authorized to access this inference")
	}

	return cvtDBInferenceToDetail(dbInference), nil
}

// deleteInference implements the inference deletion logic.
func (h *Handler) deleteInference(c *gin.Context) (interface{}, error) {
	inferenceId := c.Param("id")
	if inferenceId == "" {
		return nil, commonerrors.NewBadRequest("inference id is required")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Get inference
	inference := &v1.Inference{}
	err := h.k8sClient.Get(c.Request.Context(), client.ObjectKey{Name: inferenceId}, inference)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound(v1.InferenceKind, inferenceId)
		}
		return nil, err
	}

	// TODO: Check ownership - currently commented out for development
	// if inference.Spec.UserID != userId {
	// 	return nil, commonerrors.NewForbidden("not authorized to delete this inference")
	// }

	// Delete from etcd (resource-manager exporter will auto-sync to database)
	if err := h.k8sClient.Delete(c.Request.Context(), inference); err != nil {
		return nil, err
	}

	klog.Infof("deleted inference: %s, user: %s", inferenceId, userId)
	return gin.H{"message": "inference deleted successfully"}, nil
}

// patchInference implements the inference update logic.
func (h *Handler) patchInference(c *gin.Context) (interface{}, error) {
	inferenceId := c.Param("id")
	if inferenceId == "" {
		return nil, commonerrors.NewBadRequest("inference id is required")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	req := &PatchInferenceRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	// Get inference
	inference := &v1.Inference{}
	err := h.k8sClient.Get(c.Request.Context(), client.ObjectKey{Name: inferenceId}, inference)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewNotFound(v1.InferenceKind, inferenceId)
		}
		return nil, err
	}

	// TODO: Check ownership - currently commented out for development
	// if inference.Spec.UserID != userId {
	// 	return nil, commonerrors.NewForbidden("not authorized to update this inference")
	// }

	// Update fields
	originalInference := inference.DeepCopy()
	if req.DisplayName != nil {
		inference.Spec.DisplayName = *req.DisplayName
		if inference.Annotations == nil {
			inference.Annotations = make(map[string]string)
		}
		inference.Annotations[v1.DisplayNameLabel] = *req.DisplayName
	}
	if req.Description != nil {
		inference.Spec.Description = *req.Description
	}
	if req.Instance != nil {
		// Only allow Instance modification if the inference is from API (not managed by controller)
		if !inference.IsFromAPI() {
			return nil, commonerrors.NewBadRequest("cannot modify instance for controller-managed inference")
		}
		inference.Spec.Instance = *req.Instance
	}

	// Patch in etcd
	if err := h.k8sClient.Patch(c.Request.Context(), inference, client.MergeFrom(originalInference)); err != nil {
		return nil, err
	}

	klog.Infof("updated inference: %s, user: %s", inferenceId, userId)
	return gin.H{"message": "inference updated successfully"}, nil
}

// parseListInferenceQuery parses query parameters for listing inferences.
func parseListInferenceQuery(c *gin.Context) (*ListInferenceQuery, error) {
	query := &ListInferenceQuery{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}

	// Set default values
	if query.Limit <= 0 {
		query.Limit = 100 // Default limit
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	return query, nil
}

// cvtDBInferenceToInfo converts database inference to InferenceInfo.
func cvtDBInferenceToInfo(dbInf *dbclient.Inference) InferenceInfo {
	info := InferenceInfo{
		InferenceId: dbInf.InferenceId,
		DisplayName: dbInf.DisplayName,
		ModelForm:   dbInf.ModelForm,
		ModelName:   dbInf.ModelName,
		Phase:       getString(dbInf.Phase),
		Message:     getString(dbInf.Message),
		CreatedAt:   getTime(dbInf.CreationTime),
		UpdatedAt:   getTime(dbInf.UpdateTime),
	}
	return info
}

// cvtDBInferenceToDetail converts database inference to InferenceDetail.
func cvtDBInferenceToDetail(dbInf *dbclient.Inference) *InferenceDetail {
	detail := &InferenceDetail{
		InferenceId: dbInf.InferenceId,
		DisplayName: dbInf.DisplayName,
		Description: getString(dbInf.Description),
		UserId:      dbInf.UserId,
		UserName:    getString(dbInf.UserName),
		ModelForm:   dbInf.ModelForm,
		ModelName:   dbInf.ModelName,
		Phase:       getString(dbInf.Phase),
		Message:     getString(dbInf.Message),
		CreatedAt:   getTime(dbInf.CreationTime),
		UpdatedAt:   getTime(dbInf.UpdateTime),
	}

	// Parse JSON fields
	if dbInf.Instance.Valid {
		var instance v1.InferenceInstance
		if err := jsonutils.Unmarshal([]byte(dbInf.Instance.String), &instance); err == nil {
			detail.Instance = instance
		}
	}
	if dbInf.Resource.Valid {
		var resource v1.InferenceResource
		if err := jsonutils.Unmarshal([]byte(dbInf.Resource.String), &resource); err == nil {
			detail.Resource = resource
		}
	}
	if dbInf.Events.Valid {
		var events []v1.InferenceEvent
		if err := jsonutils.Unmarshal([]byte(dbInf.Events.String), &events); err == nil {
			detail.Events = events
		}
	}

	return detail
}

// getString safely extracts string from sql.NullString.
func getString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// getTime safely extracts time from pq.NullTime.
func getTime(nt pq.NullTime) time.Time {
	if nt.Valid {
		return nt.Time
	}
	return time.Time{}
}
