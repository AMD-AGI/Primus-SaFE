/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package inference_handlers

import (
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// Handler handles HTTP requests for inference and playground resources.
type Handler struct {
	k8sClient        client.Client
	dbClient         dbclient.Interface
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

// handle is a common handler wrapper for HTTP requests.
func handle(c *gin.Context, fn func(c *gin.Context) (interface{}, error)) {
	result, err := fn(c)
	if err != nil {
		klog.ErrorS(err, "handler error", "path", c.Request.URL.Path)
		c.JSON(getHTTPStatusCode(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, result)
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
