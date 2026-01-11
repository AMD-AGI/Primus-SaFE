// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/matcher"
	"github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/oidc"
	"github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/reconciler"
	"github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/scheduler"
	"github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/service"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	safeclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

// schemes contains app-specific types to register
// Note: corev1 types are already registered by default in controller.GetScheme()
var schemes = &runtime.SchemeBuilder{
	primusSafeV1.AddToScheme,
}

const (
	// SaFE database configuration related constants
	safeNamespace         = "primus-safe"
	safeSecretName        = "primus-safe-pguser-primus-safe"
	defaultSSLMode        = "require"
	defaultMaxOpenConns   = 100
	defaultMaxIdleConns   = 10
	defaultConnectTimeout = 10
)

var (
	globalScheduler *scheduler.Scheduler
	globalMgr       ctrl.Manager
)

func Init(ctx context.Context, cfg *config.Config) error {
	// Enable Jaeger tracer
	err := trace.InitTracer("primus-safe-adapter")
	if err != nil {
		log.Errorf("Failed to init tracer: %v", err)
		// Don't block startup, degrade to non-tracing mode
	} else {
		log.Info("Jaeger tracer initialized successfully for adapter service")
	}

	// Register cleanup function
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
		// Stop scheduler when context is done
		if globalScheduler != nil {
			globalScheduler.Stop()
		}
	}()

	if err := RegisterController(ctx); err != nil {
		return err
	}
	matcher.InitWorkloadMatcher(ctx)

	// Initialize database client and scheduled tasks
	if err := initScheduledTasks(ctx, cfg); err != nil {
		log.Errorf("Failed to initialize scheduled tasks: %v", err)
		// Don't block startup, continue without scheduled tasks
	} else {
		log.Info("Scheduled tasks initialized successfully")
	}

	return nil
}

func RegisterController(ctx context.Context) error {
	err := controller.RegisterScheme(schemes)
	if err != nil {
		return err
	}
	workloadReconciler := &reconciler.WorkloadReconciler{}
	err = workloadReconciler.Init(ctx)
	if err != nil {
		return err
	}
	controller.RegisterReconciler(workloadReconciler)

	// Register WorkspaceReconciler
	workspaceReconciler := &reconciler.WorkspaceReconciler{}
	err = workspaceReconciler.Init(ctx)
	if err != nil {
		return err
	}
	controller.RegisterReconciler(workspaceReconciler)

	return nil
}

// initScheduledTasks initializes scheduled tasks
func initScheduledTasks(ctx context.Context, cfg *config.Config) error {
	// Get k8s client from ClusterManager
	clusterManager := clientsets.GetClusterManager()
	if clusterManager == nil {
		log.Error("Failed to get ClusterManager")
		return fmt.Errorf("failed to get ClusterManager")
	}

	currentCluster := clusterManager.GetCurrentClusterClients()
	if currentCluster.K8SClientSet == nil || currentCluster.K8SClientSet.ControllerRuntimeClient == nil {
		log.Error("Failed to get K8S client from ClusterManager")
		return fmt.Errorf("failed to get K8S client from ClusterManager")
	}

	k8sClient := currentCluster.K8SClientSet.ControllerRuntimeClient

	// Read database configuration from Secret
	dbConfig, err := readDBConfigFromSecret(ctx, k8sClient)
	if err != nil {
		log.Errorf("Failed to read database config from Secret: %v", err)
		return fmt.Errorf("failed to read database config from Secret: %w", err)
	}

	// Initialize SaFE database client with the config from Secret
	safeDBClient, err := safeclient.NewClientWithConfig(dbConfig)
	if err != nil {
		log.Errorf("Failed to initialize SaFE database client: %v", err)
		return fmt.Errorf("failed to initialize SaFE database client: %w", err)
	}

	// Get GORM DB instance from SaFE client
	safeDB, err := safeDBClient.GetGormDB()
	if err != nil {
		log.Errorf("Failed to get GORM DB from SaFE client: %v", err)
		return fmt.Errorf("failed to get GORM DB from SaFE client: %w", err)
	}

	// Create workload stats service
	workloadStatsService := service.NewWorkloadStatsService(k8sClient, safeDB)

	// Create node stats service
	nodeStatsService := service.NewNodeStatsService(safeDB)

	// Create namespace sync service
	namespaceSyncService := service.NewNamespaceSyncService(k8sClient)

	// Create and configure scheduler
	globalScheduler = scheduler.NewScheduler()

	// Add workload stats collection task (runs every 30 seconds)
	globalScheduler.AddTask(workloadStatsService, 30*time.Second)

	// Add node stats collection task (runs every 60 seconds)
	globalScheduler.AddTask(nodeStatsService, 60*time.Second)

	// Add namespace sync task (runs every 60 seconds)
	globalScheduler.AddTask(namespaceSyncService, 60*time.Second)

	// Initialize Token and User sync tasks if Control Plane DB is available
	if err := initSyncTasks(clusterManager, safeDB, k8sClient); err != nil {
		log.Warnf("Token/User sync tasks not initialized: %v", err)
		// Don't fail startup, token sync is optional
	}

	// Start scheduler in background
	go globalScheduler.Start(ctx)

	log.Info("Scheduler started with workload stats (30s), node stats (60s), and namespace sync (60s) tasks")
	return nil
}

// initSyncTasks initializes sync tasks (user sync only, token sync removed)
func initSyncTasks(clusterManager *clientsets.ClusterManager, safeDB *gorm.DB, k8sClient client.Client) error {
	// Initialize session validator with SaFE DB for direct validation
	// This validates sessions directly against SaFE DB - no token sync needed
	if err := initSessionValidator(safeDB); err != nil {
		log.Warnf("Session validator not initialized: %v", err)
		// Don't fail, continue with other tasks
	}

	// User sync and auto-registration require Control Plane DB
	if clusterManager.IsControlPlaneEnabled() {
		lensDB := clusterManager.GetControlPlaneDB()
		if lensDB != nil {
			// Auto-register adapter with Lens (read config from DB and enable safe mode)
			autoRegister := service.NewAutoRegisterService(lensDB)
			if err := autoRegister.Register(context.Background()); err != nil {
				log.Warnf("Auto-registration failed: %v", err)
				// Don't fail startup, registration is optional
			}

			// Create user sync service (sync users from SaFE CRD to Lens)
			userSyncService := service.NewUserSyncService(k8sClient, lensDB)

			// Add user sync task (runs every 5 seconds for fast user info sync)
			globalScheduler.AddTask(userSyncService, 5*time.Second)

			log.Info("User sync task added to scheduler (5s interval)")
		} else {
			log.Info("Control Plane DB not available, user sync and auto-registration disabled")
		}
	} else {
		log.Info("Control Plane not enabled, user sync and auto-registration disabled")
	}

	return nil
}

// initSessionValidator initializes the SaFE session validator and registers routes
// This is a simplified service that only provides session validation
// No full OIDC provider functionality - just /validate endpoint for Lens API
func initSessionValidator(safeDB *gorm.DB) error {
	// Create session validator with SaFE DB
	validator := oidc.NewSafeValidator(safeDB, nil)

	// Register session validation routes
	router.RegisterGroup(func(group *gin.RouterGroup) error {
		// POST /validate - validate SaFE session
		group.POST("/validate", func(c *gin.Context) {
			var req oidc.ValidateRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(400, oidc.ValidateResponse{
					Valid: false,
					Error: "invalid request: session_id is required",
				})
				return
			}

			userInfo, err := validator.ValidateSafeSession(c.Request.Context(), req.SessionID)
			if err != nil {
				log.Debugf("Session validation failed: %v", err)
				c.JSON(401, oidc.ValidateResponse{
					Valid: false,
					Error: err.Error(),
				})
				return
			}

			c.JSON(200, oidc.ValidateResponse{
				Valid:   true,
				UserID:  userInfo.ID,
				Name:    userInfo.Username,
				Email:   userInfo.Email,
				IsAdmin: userInfo.IsAdmin,
			})
		})

		// GET /health - health check
		group.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "healthy"})
		})

		log.Info("Session validator routes registered: /validate, /health")
		return nil
	})

	log.Info("SaFE session validator initialized (direct SaFE DB validation)")
	return nil
}

// readDBConfigFromSecret reads database configuration from Kubernetes Secret
func readDBConfigFromSecret(ctx context.Context, k8sClient client.Client) (*utils.DBConfig, error) {
	// Read Secret from Kubernetes
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: safeNamespace,
		Name:      safeSecretName,
	}

	err := k8sClient.Get(ctx, secretKey, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret %s/%s: %w", safeNamespace, safeSecretName, err)
	}

	// Decode base64 encoded data
	dbname, err := decodeSecretData(secret.Data, "dbname")
	if err != nil {
		return nil, fmt.Errorf("failed to decode dbname: %w", err)
	}

	host, err := decodeSecretData(secret.Data, "host")
	if err != nil {
		return nil, fmt.Errorf("failed to decode host: %w", err)
	}

	password, err := decodeSecretData(secret.Data, "password")
	if err != nil {
		return nil, fmt.Errorf("failed to decode password: %w", err)
	}

	portStr, err := decodeSecretData(secret.Data, "port")
	if err != nil {
		return nil, fmt.Errorf("failed to decode port: %w", err)
	}

	user, err := decodeSecretData(secret.Data, "user")
	if err != nil {
		return nil, fmt.Errorf("failed to decode user: %w", err)
	}

	// Convert port string to int
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse port %s: %w", portStr, err)
	}

	// Build DBConfig
	dbConfig := &utils.DBConfig{
		DBName:         dbname,
		Username:       user,
		Password:       password,
		Host:           host,
		Port:           port,
		SSLMode:        defaultSSLMode,
		MaxOpenConns:   defaultMaxOpenConns,
		MaxIdleConns:   defaultMaxIdleConns,
		MaxLifetime:    time.Hour,
		MaxIdleTime:    30 * time.Minute,
		ConnectTimeout: defaultConnectTimeout,
		RequestTimeout: 30 * time.Second,
	}

	log.Infof("Database config loaded from Secret: host=%s, port=%d, dbname=%s, user=%s",
		host, port, dbname, user)

	return dbConfig, nil
}

// decodeSecretData decodes base64 data from Secret
func decodeSecretData(data map[string][]byte, key string) (string, error) {
	encodedValue, exists := data[key]
	if !exists {
		return "", fmt.Errorf("key %s not found in Secret data", key)
	}

	// Secret data is already decoded by Kubernetes client, just convert to string
	return string(encodedValue), nil
}
