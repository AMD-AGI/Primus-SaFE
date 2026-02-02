// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/AMD-AGI/Primus-SaFE/Lens/tools-repository/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/tools-repository/pkg/registry"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func main() {
	// Get database URL from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/tools_repository?sslmode=disable"
	}

	// Connect to database
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Connected to database")

	// Create registry
	reg := registry.NewToolsRegistry(db)

	// Create API handler
	handler := api.NewHandler(reg)

	// Create Gin router
	router := gin.Default()

	// Register routes
	handler.RegisterRoutes(router)

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("Starting Tools Repository server on port %s", port)

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
