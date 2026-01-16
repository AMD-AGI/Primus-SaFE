/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commons3 "github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
)

// Handler handles HTTP requests for inference, playground, and dataset resources.
type Handler struct {
	k8sClient        client.Client // TODO Multi-cluster k8sclient
	dbClient         dbclient.Interface
	s3Client         commons3.Interface // S3 client for dataset storage (may be nil if S3 is disabled)
	accessController *authority.AccessController
}

// NewHandler creates a new inference handler.
func NewHandler(k8sClient client.Client, dbClient dbclient.Interface, accessController *authority.AccessController) *Handler {
	return &Handler{
		k8sClient:        k8sClient,
		dbClient:         dbClient,
		accessController: accessController,
	}
}

// NewHandlerWithS3 creates a new handler with S3 client for dataset operations.
func NewHandlerWithS3(k8sClient client.Client, dbClient dbclient.Interface, s3Client commons3.Interface, accessController *authority.AccessController) *Handler {
	return &Handler{
		k8sClient:        k8sClient,
		dbClient:         dbClient,
		s3Client:         s3Client,
		accessController: accessController,
	}
}

// IsDatasetEnabled returns true if dataset operations are enabled (S3 client is configured).
func (h *Handler) IsDatasetEnabled() bool {
	return h.s3Client != nil
}

// handle is a common handler wrapper for HTTP requests (for model/playground APIs).
func handle(c *gin.Context, fn func(c *gin.Context) (interface{}, error)) {
	result, err := fn(c)
	if err != nil {
		klog.ErrorS(err, "handler error", "path", c.Request.URL.Path)
		c.JSON(getHTTPStatusCode(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, result)
}

// handleDataset is a handler wrapper for dataset APIs with proper error handling.
func handleDataset(c *gin.Context, fn func(c *gin.Context) (interface{}, error)) {
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

// getHTTPStatusCode returns the appropriate HTTP status code for an error.
func getHTTPStatusCode(err error) int {
	// This is a simplified version. You may want to use a more sophisticated
	// error type checking mechanism based on your error types.
	switch {
	case err == nil:
		return 200
	default:
		return 500
	}
}
