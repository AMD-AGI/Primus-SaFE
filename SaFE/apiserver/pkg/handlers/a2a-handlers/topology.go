/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2ahandlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	sqrl "github.com/Masterminds/squirrel"

	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/a2a-handlers/view"
)

// GetTopology handles GET /api/v1/a2a/topology
func (h *Handler) GetTopology(c *gin.Context) {
	services, err := h.dbClient.ListActiveA2AServices(c.Request.Context())
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError(err.Error()))
		return
	}

	nodes := make([]view.ServiceView, 0, len(services))
	for _, svc := range services {
		nodes = append(nodes, toServiceView(svc))
	}

	logs, err := h.dbClient.SelectA2ACallLogs(c.Request.Context(), sqrl.Expr("1=1"), []string{"created_at DESC"}, 1000, 0)
	if err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError(err.Error()))
		return
	}

	edgeMap := make(map[string]*view.TopologyEdge)
	for _, log := range logs {
		key := log.CallerServiceName + "->" + log.TargetServiceName
		if e, ok := edgeMap[key]; ok {
			e.Count++
		} else {
			edgeMap[key] = &view.TopologyEdge{
				Caller: log.CallerServiceName,
				Target: log.TargetServiceName,
				Count:  1,
			}
		}
	}

	edges := make([]view.TopologyEdge, 0, len(edgeMap))
	for _, e := range edgeMap {
		edges = append(edges, *e)
	}

	c.JSON(http.StatusOK, view.TopologyResponse{Nodes: nodes, Edges: edges})
}
