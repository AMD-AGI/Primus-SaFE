// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/embedding"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/runner"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/service"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/storage"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Server represents the tools server
type Server struct {
	config        *config.Config
	db            *gorm.DB
	httpServer    *http.Server
	facade        *database.ToolFacade
	toolsetFacade *database.ToolsetFacade
	runner        *runner.Runner
	storage       storage.Storage
	embedding     *embedding.Service
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

	// Create facades
	facade := database.NewToolFacade(db)
	toolsetFacade := database.NewToolsetFacade(db)

	// Create storage
	storageService, err := storage.NewStorage(cfg.Storage)
	if err != nil {
		fmt.Printf("Warning: Failed to create storage service: %v\n", err)
	}

	// Create runner (if enabled)
	var toolRunner *runner.Runner
	if cfg.Runner.Enabled {
		backend := runner.NewHTTPBackend(runner.HTTPBackendConfig{
			BaseURL: cfg.Runner.BaseURL,
		})
		toolRunner = runner.NewRunner(backend)
		fmt.Printf("Runner enabled: %s\n", cfg.Runner.BaseURL)
	}

	// Create embedding service (if enabled)
	var embeddingSvc *embedding.Service
	if cfg.Embedding.Enabled {
		embeddingSvc = embedding.NewService(embedding.Config{
			Enabled:   cfg.Embedding.Enabled,
			BaseURL:   cfg.Embedding.BaseURL,
			APIKey:    cfg.Embedding.APIKey,
			Model:     cfg.Embedding.Model,
			Dimension: cfg.Embedding.Dimension,
		})
		fmt.Printf("Embedding enabled: model=%s, dimension=%d\n", cfg.Embedding.Model, cfg.Embedding.Dimension)
	}

	return &Server{
		config:        cfg,
		db:            db,
		facade:        facade,
		toolsetFacade: toolsetFacade,
		runner:        toolRunner,
		storage:       storageService,
		embedding:     embeddingSvc,
	}, nil
}

// Start starts the server
func (s *Server) Start() error {
	// Setup HTTP server
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger()) // Add request logging
	router.Use(gin.Recovery())

	// Create services
	toolSvc := service.NewToolService(s.facade, s.storage, s.embedding)
	searchSvc := service.NewSearchService(s.facade, s.embedding, s.config.Search.ScoreThreshold)
	importSvc := service.NewImportService(s.facade, s.storage, s.embedding)
	runSvc := service.NewRunService(s.facade, s.runner, s.storage)
	toolsetSvc := service.NewToolsetService(s.toolsetFacade, s.facade, s.embedding, s.config.Search.ScoreThreshold)

	// Create handler and register routes
	handler := api.NewHandler(toolSvc, searchSvc, importSvc, runSvc, toolsetSvc)
	api.RegisterRoutes(router, handler)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Server.Port),
		Handler: router,
	}

	fmt.Printf("Tools API listening on port %d\n", s.config.Server.Port)
	return s.httpServer.ListenAndServe()
}

// Stop stops the server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
