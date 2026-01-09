// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/gin-gonic/gin"
)

func InitServer(ctx context.Context) error {
	return InitServerWithPreInitFunc(ctx, nil)
}

func InitServerWithPreInitFunc(ctx context.Context, preInit func(ctx context.Context, cfg *config.Config) error) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	// Initialize ClusterManager with appropriate options
	if cfg.IsControlPlaneEnabled() {
		// Control Plane enabled - initialize with Control Plane support
		log.Info("Control Plane enabled, initializing with database support...")
		cpConfig, err := clientsets.NewControlPlaneConfigFromEnv()
		if err != nil {
			return errors.NewError().WithCode(errors.CodeInitializeError).
				WithMessage("Failed to load Control Plane config from environment").WithError(err)
		}
		opts := &clientsets.InitOptions{
			LoadControlPlane:   true,
			ControlPlaneConfig: cpConfig,
			MultiCluster:       cfg.MultiCluster,
			LoadK8SClient:      cfg.LoadK8SClient,
			LoadStorageClient:  cfg.LoadStorageClient,
		}
		err = clientsets.InitClusterManagerWithOptions(ctx, opts)
		if err != nil {
			return errors.NewError().WithCode(errors.CodeInitializeError).
				WithMessage("Failed to initialize ClusterManager with Control Plane").WithError(err)
		}
		log.Info("ClusterManager initialized with Control Plane support")
	} else {
		// Control Plane disabled - use legacy initialization
		err = clientsets.InitClientSets(ctx, cfg.MultiCluster, cfg.LoadK8SClient, cfg.LoadStorageClient)
		if err != nil {
			return err
		}
	}

	if preInit != nil {
		err := preInit(ctx, cfg)
		if err != nil {
			return errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("PreInit Error").WithError(err)
		}
	}
	ginEngine := gin.New()
	ginEngine.Use(gin.Recovery())
	err = router.InitRouter(ginEngine, cfg)
	if err != nil {
		return err
	}
	err = controller.InitControllers(ctx, *cfg)
	if err != nil {
		return err
	}
	InitHealthServer(cfg.HttpPort + 1)
	err = ginEngine.Run(fmt.Sprintf(":%d", cfg.HttpPort))
	if err != nil {
		return err
	}
	return nil
}
