package server

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/controller"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/router"
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
	err = clientsets.InitClientSets(ctx, cfg.MultiCluster, cfg.LoadK8SClient, cfg.LoadStorageClient)
	if err != nil {
		return err
	}
	if preInit != nil {
		err := preInit(ctx, cfg)
		if err != nil {
			return errors.NewError().WithCode(errors.CodeInitializeError).WithMessage("PreInit Error").WithError(err)
		}
	}
	ginEngine := gin.New()
	ginEngine.Use(gin.Recovery())
	err = router.InitRouter(ginEngine)
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
