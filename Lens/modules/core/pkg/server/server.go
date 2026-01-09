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

	// Step 1: Initialize K8s client first (needed for reading secrets)
	err = clientsets.InitClientSets(ctx, cfg.MultiCluster, cfg.LoadK8SClient, cfg.LoadStorageClient)
	if err != nil {
		return err
	}

	// Step 2: If Control Plane is enabled, read DB config from Secret and initialize
	if cfg.IsControlPlaneEnabled() {
		log.Info("Control Plane enabled, reading database config from Secret...")

		cm := clientsets.GetClusterManager()
		cc := cm.GetCurrentClusterClients()
		if cc == nil || cc.K8SClientSet == nil || cc.K8SClientSet.ControllerRuntimeClient == nil {
			return errors.NewError().WithCode(errors.CodeInitializeError).
				WithMessage("K8s client not available, cannot read Control Plane secret")
		}

		// Get secret name and namespace from config (with defaults)
		secretName := ""
		secretNamespace := ""
		if cfg.ControlPlane != nil {
			secretName = cfg.ControlPlane.SecretName
			secretNamespace = cfg.ControlPlane.SecretNamespace
		}

		// Read DB config from K8s Secret
		cpConfig, err := clientsets.NewControlPlaneConfigFromSecret(
			ctx,
			cc.K8SClientSet.ControllerRuntimeClient,
			secretName,
			secretNamespace,
		)
		if err != nil {
			return errors.NewError().WithCode(errors.CodeInitializeError).
				WithMessage("Failed to read Control Plane config from Secret").WithError(err)
		}

		// Initialize Control Plane with the config from Secret
		if err := cm.InitControlPlane(ctx, cpConfig); err != nil {
			return errors.NewError().WithCode(errors.CodeInitializeError).
				WithMessage("Failed to initialize Control Plane").WithError(err)
		}
		log.Info("Control Plane initialized successfully from Secret")
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
