/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/AMD-AIG-AIMA/SAFE/a2a-gateway/pkg/auth"
	"github.com/AMD-AIG-AIMA/SAFE/a2a-gateway/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/a2a-gateway/pkg/proxy"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// Server is the A2A Gateway HTTP server.
type Server struct {
	cfg    *config.Config
	engine *gin.Engine
}

// New creates a new gateway server.
func New(cfg *config.Config) (*Server, error) {
	db := dbclient.NewClient()
	if db == nil {
		return nil, fmt.Errorf("failed to initialize database client")
	}

	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())

	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	proxyHandler := proxy.NewHandler(db)

	a2a := engine.Group("/a2a")
	a2a.Use(auth.ApiKeyMiddleware(db))
	a2a.POST("/invoke/:target", proxyHandler.Invoke)
	a2a.POST("/invoke/:target/:skill", proxyHandler.Invoke)
	a2a.GET("/agents", proxyHandler.ListAgents)

	return &Server{cfg: cfg, engine: engine}, nil
}

// Run starts both the main server and the metrics server.
func (s *Server) Run() error {
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		addr := fmt.Sprintf(":%d", s.cfg.MetricsPort)
		if err := http.ListenAndServe(addr, mux); err != nil {
			panic(fmt.Sprintf("metrics server error: %v", err))
		}
	}()

	return s.engine.Run(fmt.Sprintf(":%d", s.cfg.ServerPort))
}
