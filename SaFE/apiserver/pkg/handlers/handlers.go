/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package handlers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	cdhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/cd-handlers"
	emailrelayhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/email-relay-handlers"
	githubworkflow "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/github-workflow"
	imagehandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/image-handlers"
	inferencexhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/inferencex"
	lenscompat "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/lens-compat"
	llmgateway "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/llm-gateway"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	model_handlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/model-handlers"
	optimizationhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/optimization"
	proxyhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/proxy-handlers"
	reshandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/scim"
	sshhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers"
	mcprouter "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/mcp/router"
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
	engine.Use(apiutils.Logger(), gin.Recovery(), middleware.HandleMetrics(), middleware.HandleTracing())
	engine.NoRoute(func(c *gin.Context) {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage(c.Request.RequestURI+" not found"))
	})
	if commonconfig.IsSSOEnable() || commonconfig.IsSSOLoginEnable() {
		if authority.NewSSOToken(mgr.GetClient()) == nil {
			return nil, commonerrors.NewInternalError("failed to new sso token")
		}
	}
	if authority.NewDefaultToken(mgr.GetClient()) == nil {
		return nil, commonerrors.NewInternalError("failed to new default token")
	}
	// Pluggable authentication: a no-op in the default builtin mode.
	if err := authority.InitExternalAuth(mgr.GetClient()); err != nil {
		return nil, commonerrors.NewInternalError("failed to initialize external authentication: " + err.Error())
	}
	if err := authority.CheckExternalDeployment(); err != nil {
		return nil, commonerrors.NewInternalError("failed to initialize external authentication: " + err.Error())
	}
	var scimToken *scim.Token
	if commonconfig.GetAuthMode() == commonconfig.AuthModeExternal && commonconfig.IsSCIMEnabled() {
		token, err := scim.EnsureToken(context.Background(), mgr.GetClient())
		if err != nil {
			return nil, commonerrors.NewInternalError("failed to initialize scim: " + err.Error())
		}
		scimToken = scim.NewToken(token)
		if commonconfig.ProviderClientEnabled() {
			scim.StartRegistration(context.Background(), mgr.GetClient(), scimToken)
		} else {
			klog.Info("auth.provider_client is off; the scim target is not registered")
		}
	} else if commonconfig.IsSCIMEnabled() {
		klog.Warning("auth.scim.enabled is set but auth.mode is not external; the scim endpoint is not served")
	}
	// Initialize internal auth for service-to-service authentication
	if _, err := authority.NewInternalAuth(mgr.GetClient()); err != nil {
		return nil, commonerrors.NewInternalError("failed to initialize internal auth: " + err.Error())
	}
	// Initialize API key authentication if database is enabled
	if commonconfig.IsDBEnable() {
		dbClient := dbclient.NewClient()
		if dbClient != nil {
			authority.NewApiKeyToken(dbClient, mgr.GetClient())
		}
	}
	routes := routeHandlers{scimToken: scimToken}

	// Proxy handlers are registered first to avoid route conflicts with resource handlers.
	var err error
	if routes.proxy, err = proxyhandlers.NewProxyHandler(); err != nil {
		return nil, err
	}

	// Dynamic lens-compat proxy: routes /lens/v1/* to the robust-analyzer of
	// the cluster named by ?cluster=<name>. Uses the shared robustclient that
	// is populated by resource-manager's Discovery controller, so adding a
	// new data cluster only requires a Cluster CR + the addon endpoint
	// annotation — no apiserver config or helm upgrade.
	routes.lens = lenscompat.NewHandler(model_handlers.GetRobustClient())

	if routes.resources, err = reshandler.NewHandler(mgr); err != nil {
		return nil, err
	}
	if routes.cd, err = cdhandlers.NewHandler(mgr); err != nil {
		return nil, err
	}
	if routes.image, err = imagehandlers.NewImageHandler(mgr); err != nil {
		return nil, err
	}
	if routes.ssh, err = InitSshHandlers(context.Background(), mgr); err != nil {
		return nil, err
	}
	routes.model = InitModelHandlers(context.Background(), mgr)

	// Initialize LLM Gateway handlers (if enabled and DB is available)
	if commonconfig.IsLLMGatewayEnable() && commonconfig.IsDBEnable() {
		llmDbClient := dbclient.NewClient()
		if llmDbClient != nil {
			llmHandler, llmErr := llmgateway.NewHandler(authority.NewAccessController(mgr.GetClient()), llmDbClient)
			if llmErr != nil {
				klog.ErrorS(llmErr, "failed to initialize LLM Gateway handler")
			} else {
				routes.llm = llmHandler
			}
		}
	}

	// Initialize email relay handlers (only when DB is enabled)
	if commonconfig.IsDBEnable() {
		emailRelayHandler, err := emailrelayhandlers.NewHandler()
		if err != nil {
			klog.Warningf("Email relay handler initialization skipped: %v", err)
		} else {
			routes.emailRelay = emailRelayHandler
		}
	}

	// GitHub Workflow CI/CD API
	routes.githubWorkflow = commonconfig.IsDBEnable()

	// InferenceX benchmark data proxy (no DB required)
	routes.inferencex = inferencexhandlers.NewHandler(24 * time.Hour)

	// Model Optimization (Hyperloom via PrimusClaw). Requires DB for task
	// persistence + event log. Routes register whenever DB is enabled; Claw base
	// URL from model_optimization.claw_base_url or derived https://<global host>/claw-api/v1;
	// auth: Bearer ak-... if sent, else per-user platform key (GetOrCreatePlatformKey, same
	// idea as Primus-Claw + /auth/verify), else optional secret claw_api_key for service use.
	if commonconfig.IsDBEnable() {
		optDBClient := dbclient.NewClient()
		if optDBClient != nil {
			optHandler, optErr := optimizationhandlers.NewHandler(mgr.GetClient(), optDBClient,
				authority.NewAccessController(mgr.GetClient()))
			if optErr != nil {
				klog.ErrorS(optErr, "failed to initialize Model Optimization handler")
			} else {
				routes.optimization = optHandler
				go optHandler.Start(context.Background())
			}
		}
	}

	// MCP (Model Context Protocol) server: opt-in via mcp.enabled (default true
	// in Helm chart). Mounts SSE + Streamable HTTP transports under the path
	// configured by mcp.base_path so AI clients can drive the apiserver.
	routes.mcp = commonconfig.IsMCPEnable()

	registerRoutes(engine, routes)
	return engine, nil
}

// routeHandlers are the handlers whose routes the apiserver serves; a nil one, or a false flag, is
// not served (what InitHttpHandlers leaves out when its dependency, such as the database, is not
// available). It is the one list of what the apiserver registers, so a test that registers all of
// it, with handlers that have nothing behind them, sees the same routes the apiserver serves.
type routeHandlers struct {
	proxy          *proxyhandlers.ProxyHandler
	lens           *lenscompat.Handler
	resources      *reshandler.Handler
	cd             *cdhandlers.Handler
	image          *imagehandlers.ImageHandler
	ssh            *sshhandler.SshHandler
	model          *model_handlers.Handler
	llm            *llmgateway.Handler
	emailRelay     *emailrelayhandlers.Handler
	githubWorkflow bool
	inferencex     *inferencexhandlers.Handler
	optimization   *optimizationhandlers.Handler
	mcp            bool
	// scimToken is set when the SCIM endpoint is served. Nil leaves the routes unregistered.
	scimToken *scim.Token
}

// registerRoutes registers the routes of the handlers on the engine, in the order that matters: the
// proxies first, so that they do not conflict with the resource routes.
func registerRoutes(engine *gin.Engine, h routeHandlers) {
	if h.proxy != nil {
		proxyhandlers.InitProxyRoutes(engine, h.proxy)
	}
	if h.lens != nil {
		h.lens.Register(engine)
	}
	if h.resources != nil {
		reshandler.InitCustomRouters(engine, h.resources)
	}
	if h.scimToken != nil && h.resources != nil {
		scim.Mount(engine, h.resources, h.scimToken)
	}
	if h.cd != nil {
		cdhandlers.InitCDRouters(engine, h.cd)
	}
	if h.image != nil {
		imagehandlers.InitImageRouter(engine, h.image)
	}
	if h.ssh != nil {
		sshhandler.InitWebShellRouters(engine, h.ssh)
	}
	if h.model != nil {
		model_handlers.InitInferenceRouters(engine, h.model)
	}
	if h.llm != nil {
		llmgateway.InitRoutes(engine, h.llm)
	}
	if h.emailRelay != nil {
		emailrelayhandlers.InitEmailRelayRouters(engine, h.emailRelay)
	}
	if h.githubWorkflow {
		githubworkflow.RegisterRoutes(engine.Group(common.PrimusRouterCustomRootPath))
	}
	if h.inferencex != nil {
		inferencexhandlers.InitInferenceXRouters(engine, h.inferencex)
	}
	if h.optimization != nil {
		optimizationhandlers.InitRoutes(engine, h.optimization)
	}
	if h.mcp {
		mcprouter.InitRoutes(engine)
	}
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
