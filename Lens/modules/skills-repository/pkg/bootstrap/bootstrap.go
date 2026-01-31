// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/discovery"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/registry"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Server represents the skills repository server
type Server struct {
	config     *config.Config
	db         *gorm.DB
	httpServer *http.Server
	registry   *registry.SkillsRegistry
	discovery  *discovery.SkillsDiscovery
	embedder   embedding.Embedder
}

// NewServer creates a new Server instance
func NewServer() (*Server, error) {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Connect to database
	db, err := connectDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize control plane facade
	database.InitControlPlaneFacade(db)

	// Create embedder
	embedder := embedding.NewOpenAIEmbedder(cfg.Embedding)

	// Create registry
	reg := registry.NewSkillsRegistry(database.GetControlPlaneFacade(), embedder)

	// Create discovery
	disc, err := discovery.NewSkillsDiscovery(cfg.Discovery, reg)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery: %w", err)
	}

	return &Server{
		config:    cfg,
		db:        db,
		registry:  reg,
		discovery: disc,
		embedder:  embedder,
	}, nil
}

// Start starts the server
func (s *Server) Start() error {
	// Start discovery
	ctx := context.Background()
	if err := s.discovery.Start(ctx); err != nil {
		log.Warnf("Failed to start discovery: %v", err)
	}

	// Setup HTTP server
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Register API routes
	apiHandler := api.NewHandler(s.registry, s.embedder)
	api.RegisterRoutes(router, apiHandler)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Server.Port),
		Handler: router,
	}

	log.Infof("Skills Repository listening on port %d", s.config.Server.Port)
	return s.httpServer.ListenAndServe()
}

// Stop stops the server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop discovery
	s.discovery.Stop()

	// Shutdown HTTP server
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func connectDatabase(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}
