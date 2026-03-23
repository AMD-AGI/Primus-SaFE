/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package handlers

import (
	"context"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	cdhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/cd-handlers"
	emailrelayhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/email-relay-handlers"
	githubworkflow "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/github-workflow"
	imagehandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/image-handlers"
	llmgateway "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/llm-gateway"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	model_handlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/model-handlers"
	a2ahandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/a2a-handlers"
	proxyhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/proxy-handlers"
	reshandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources"
	sshhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/a2a"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commons3 "github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
)

// InitHttpHandlers initializes the HTTP handlers for the API server.
// It creates a new Gin engine, sets up middleware including logging, recovery, and CORS,
// initializes custom API routes.
// Returns the configured Gin engine or an error if initialization fails.
func InitHttpHandlers(_ context.Context, mgr ctrlruntime.Manager) (*gin.Engine, error) {
	engine := gin.New()
	engine.Use(apiutils.Logger(), gin.Recovery(), middleware.HandleTracing())
	engine.NoRoute(func(c *gin.Context) {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage(c.Request.RequestURI+" not found"))
	})
	if commonconfig.IsSSOEnable() {
		if authority.NewSSOToken(mgr.GetClient()) == nil {
			return nil, commonerrors.NewInternalError("failed to new sso token")
		}
	}
	if authority.NewDefaultToken(mgr.GetClient()) == nil {
		return nil, commonerrors.NewInternalError("failed to new default token")
	}
	// Initialize internal auth for service-to-service authentication
	if _, err := authority.NewInternalAuth(mgr.GetClient()); err != nil {
		return nil, commonerrors.NewInternalError("failed to initialize internal auth: " + err.Error())
	}
	// Initialize API key authentication if database is enabled
	if commonconfig.IsDBEnable() {
		dbClient := dbclient.NewClient()
		if dbClient != nil {
			authority.NewApiKeyToken(dbClient)
		}
	}
	// Initialize proxy handlers first to avoid route conflicts with resource handlers
	proxyHandler, err := proxyhandlers.NewProxyHandler()
	if err != nil {
		return nil, err
	}
	proxyhandlers.InitProxyRoutes(engine, proxyHandler)

	customHandler, err := reshandler.NewHandler(mgr)
	if err != nil {
		return nil, err
	}
	reshandler.InitCustomRouters(engine, customHandler)
	cdHandler, err := cdhandlers.NewHandler(mgr)
	if err != nil {
		return nil, err
	}
	cdhandlers.InitCDRouters(engine, cdHandler)
	imageHandler, err := imagehandlers.NewImageHandler(mgr)
	if err != nil {
		return nil, err
	}
	imagehandlers.InitImageRouter(engine, imageHandler)
	sshHandler, err := InitSshHandlers(context.Background(), mgr)
	if err != nil {
		return nil, err
	}
	sshhandler.InitWebShellRouters(engine, sshHandler)
	modelHandler := InitModelHandlers(context.Background(), mgr)
	model_handlers.InitInferenceRouters(engine, modelHandler)

	// Initialize A2A handlers if database is enabled
	if commonconfig.IsDBEnable() {
		a2aDbClient := dbclient.NewClient()
		if a2aDbClient != nil {
			a2aHandler := a2ahandlers.NewHandler(a2aDbClient)
			a2ahandlers.InitA2ARouters(engine, a2aHandler)

			if commonconfig.IsA2AScannerEnable() {
				scanner := a2a.NewScanner(mgr.GetClient(), a2aDbClient)
				go scanner.Start(context.Background())
			}
		}
	}

	// Initialize LLM Gateway handlers (if enabled and DB is available)
	if commonconfig.IsLLMGatewayEnable() && commonconfig.IsDBEnable() {
		llmDbClient := dbclient.NewClient()
		if llmDbClient != nil {
			llmHandler, llmErr := llmgateway.NewHandler(authority.NewAccessController(mgr.GetClient()), llmDbClient)
			if llmErr != nil {
				klog.ErrorS(llmErr, "failed to initialize LLM Gateway handler")
			} else {
				llmgateway.InitRoutes(engine, llmHandler)
			}
		}
	}

	// Initialize email relay handlers (only when DB is enabled)
	if commonconfig.IsDBEnable() {
		emailRelayHandler, err := emailrelayhandlers.NewHandler()
		if err != nil {
			klog.Warningf("Email relay handler initialization skipped: %v", err)
		} else {
			emailrelayhandlers.InitEmailRelayRouters(engine, emailRelayHandler)
		}
	}

	// GitHub Workflow CI/CD API
	if commonconfig.IsDBEnable() {
		githubworkflow.RegisterRoutes(engine.Group(common.PrimusRouterCustomRootPath))
	}

	return engine, nil
}

// InitModelHandlers initializes the model handlers for the API server.
// It creates and returns a new model handler instance configured with the provided manager.
// If database is not enabled, dbClient will be nil and handlers will use K8s API only.
// If S3 is enabled, s3Client will be initialized for dataset operations.
func InitModelHandlers(ctx context.Context, mgr ctrlruntime.Manager) *model_handlers.Handler {
	var dbClient dbclient.Interface
	if commonconfig.IsDBEnable() {
		dbClient = dbclient.NewClient()
	}
	accessController := authority.NewAccessController(mgr.GetClient())

	// Initialize S3 client for dataset operations if S3 is enabled
	var s3Client commons3.Interface
	if commonconfig.IsS3Enable() && commonconfig.IsDBEnable() {
		var err error
		s3Client, err = commons3.NewClient(ctx, commons3.Option{})
		if err != nil {
			klog.ErrorS(err, "failed to initialize S3 client for dataset operations, dataset features will be disabled")
		}
	}

	if s3Client != nil {
		return model_handlers.NewHandlerWithS3(mgr.GetClient(), dbClient, s3Client, accessController)
	}
	return model_handlers.NewHandler(mgr.GetClient(), dbClient, accessController)
}

// InitSshHandlers initializes the SSH handlers for the API server.
// It creates and returns a new SSH handler instance configured with the provided manager.
// Returns the SSH handler or an error if initialization fails.
func InitSshHandlers(ctx context.Context, mgr ctrlruntime.Manager) (*sshhandler.SshHandler, error) {
	return sshhandler.NewSshHandler(ctx, mgr)
}
