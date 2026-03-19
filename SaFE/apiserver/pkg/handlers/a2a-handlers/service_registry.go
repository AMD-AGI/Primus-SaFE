/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2ahandlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/a2a-handlers/view"
)

func toServiceView(svc *dbclient.A2AServiceRegistry) view.ServiceView {
	v := view.ServiceView{
		Id:              svc.Id,
		ServiceName:     svc.ServiceName,
		DisplayName:     svc.DisplayName,
		Description:     svc.Description,
		Endpoint:        svc.Endpoint,
		A2APathPrefix:   svc.A2APathPrefix,
		A2AHealth:       svc.A2AHealth,
		DiscoverySource: svc.DiscoverySource,
		Status:          svc.Status,
	}
	if svc.WorkloadId.Valid {
		v.WorkloadId = svc.WorkloadId.String
	}
	if svc.A2AAgentCard.Valid {
		v.A2AAgentCard = svc.A2AAgentCard.String
	}
	if svc.A2ASkills.Valid {
		v.A2ASkills = svc.A2ASkills.String
	}
	if svc.A2ALastSeen.Valid {
		t := svc.A2ALastSeen.Time
		v.A2ALastSeen = &t
	}
	if svc.K8sNamespace.Valid {
		v.K8sNamespace = svc.K8sNamespace.String
	}
	if svc.K8sService.Valid {
		v.K8sService = svc.K8sService.String
	}
	if svc.CreatedBy.Valid {
		v.CreatedBy = svc.CreatedBy.String
	}
	if svc.CreatedAt.Valid {
		t := svc.CreatedAt.Time
		v.CreatedAt = &t
	}
	if svc.UpdatedAt.Valid {
		t := svc.UpdatedAt.Time
		v.UpdatedAt = &t
	}
	return v
}

// CreateService handles POST /api/v1/a2a/services
func (h *Handler) CreateService(c *gin.Context) {
	var req view.CreateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest(err.Error()))
		return
	}

	prefix := req.A2APathPrefix
	if prefix == "" {
		prefix = "/a2a"
	}

	svc := &dbclient.A2AServiceRegistry{
		ServiceName:     req.ServiceName,
		DisplayName:     req.DisplayName,
		Description:     req.Description,
		Endpoint:        req.Endpoint,
		A2APathPrefix:   prefix,
		DiscoverySource: "manual",
		Status:          "active",
		A2AHealth:       "unknown",
	}
	if req.WorkloadId != "" {
		svc.WorkloadId = sql.NullString{String: req.WorkloadId, Valid: true}
	}

	if err := h.dbClient.UpsertA2AService(c.Request.Context(), svc); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError(err.Error()))
		return
	}
	c.JSON(http.StatusCreated, toServiceView(svc))
}

// ListServices handles GET /api/v1/a2a/services
func (h *Handler) ListServices(c *gin.Context) {
	status := c.DefaultQuery("status", "active")
	limitStr := c.DefaultQuery("limit", "100")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit <= 0 {
		limit = 100
	}

	query := sqrl.Eq{"status": status}
	services, err := h.dbClient.SelectA2AServices(c.Request.Context(), query, []string{"service_name ASC"}, limit, offset)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError(err.Error()))
		return
	}
	total, _ := h.dbClient.CountA2AServices(c.Request.Context(), query)

	views := make([]view.ServiceView, 0, len(services))
	for _, svc := range services {
		views = append(views, toServiceView(svc))
	}
	c.JSON(http.StatusOK, gin.H{"data": views, "total": total})
}

// GetService handles GET /api/v1/a2a/services/:serviceName
func (h *Handler) GetService(c *gin.Context) {
	serviceName := c.Param("serviceName")
	svc, err := h.dbClient.GetA2AService(c.Request.Context(), serviceName)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("A2A service not found"))
		return
	}
	c.JSON(http.StatusOK, toServiceView(svc))
}

// UpdateService handles PATCH /api/v1/a2a/services/:serviceName
func (h *Handler) UpdateService(c *gin.Context) {
	serviceName := c.Param("serviceName")
	svc, err := h.dbClient.GetA2AService(c.Request.Context(), serviceName)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("A2A service not found"))
		return
	}

	var req view.UpdateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest(err.Error()))
		return
	}

	if req.DisplayName != nil {
		svc.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		svc.Description = *req.Description
	}
	if req.Endpoint != nil {
		svc.Endpoint = *req.Endpoint
	}
	if req.A2APathPrefix != nil {
		svc.A2APathPrefix = *req.A2APathPrefix
	}
	if req.Status != nil {
		svc.Status = *req.Status
	}
	svc.UpdatedAt = pq.NullTime{Time: time.Now().UTC(), Valid: true}

	if err := h.dbClient.UpsertA2AService(c.Request.Context(), svc); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, toServiceView(svc))
}

// DeleteService handles DELETE /api/v1/a2a/services/:serviceName
func (h *Handler) DeleteService(c *gin.Context) {
	serviceName := c.Param("serviceName")
	if err := h.dbClient.SetA2AServiceDeleted(c.Request.Context(), serviceName); err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
