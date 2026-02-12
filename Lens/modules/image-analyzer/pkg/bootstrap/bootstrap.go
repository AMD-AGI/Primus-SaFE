// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/image-analyzer/pkg/analyzer"
)

// Init is the pre-init function for the image-analyzer service.
// It starts the background worker that processes pending image analysis requests.
func Init(ctx context.Context, cfg *config.Config) error {
	log.Info("Initializing Image Analyzer service...")

	worker, err := analyzer.NewWorker()
	if err != nil {
		return err
	}

	// Start the worker in a background goroutine
	go func() {
		log.Info("Image Analyzer worker started")
		worker.Run(ctx)
		log.Info("Image Analyzer worker stopped")
	}()

	return nil
}
