/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dataset_handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commons3 "github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
)

// Handler handles HTTP requests for dataset resources.
type Handler struct {
	client.Client
	dbClient         dbclient.Interface
	s3Client         commons3.Interface
	accessController *authority.AccessController
}

// NewHandler creates a new dataset handler instance.
// It initializes all required clients including:
// - k8sClient: Kubernetes client for workspace and OpsJob operations
// - dbClient: Database client for dataset metadata storage
// - s3Client: S3 client for dataset file storage
// - accessController: AccessController for access control
// Returns nil if database is not enabled.
func NewHandler(ctx context.Context, mgr ctrlruntime.Manager) (*Handler, error) {
	if !commonconfig.IsDBEnable() {
		return nil, nil
	}

	dbClient := dbclient.NewClient()
	if dbClient == nil {
		return nil, fmt.Errorf("failed to new db client")
	}

	s3Client, err := commons3.NewClient(ctx, commons3.Option{})
	if err != nil {
		return nil, fmt.Errorf("failed to new s3 client: %v", err)
	}

	accessController := authority.NewAccessController(mgr.GetClient())

	return &Handler{
		Client:           mgr.GetClient(),
		dbClient:         dbClient,
		s3Client:         s3Client,
		accessController: accessController,
	}, nil
}

type handleFunc func(*gin.Context) (interface{}, error)

// handle is a middleware function that executes the provided handler function and processes its response.
// It handles errors by aborting the request with an API error, and formats successful responses.
func handle(c *gin.Context, fn handleFunc) {
	response, err := fn(c)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	code := http.StatusOK
	// If a status was previously set, use that status in the response.
	if c.Writer.Status() > 0 {
		code = c.Writer.Status()
	}
	switch responseType := response.(type) {
	case []byte:
		c.Data(code, common.JsonContentType, responseType)
	case string:
		c.Data(code, common.JsonContentType, []byte(responseType))
	default:
		c.JSON(code, responseType)
	}
}
