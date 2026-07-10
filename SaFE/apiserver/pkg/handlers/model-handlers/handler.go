/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	commons3 "github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
)

// globalRobustClient is the process-wide robust-analyzer client used by
// posttrain / Lens-compat handlers to talk to per-cluster robust-api.
// Set once at startup via SetRobustClient; nil means posttrain Lens-style
// features are disabled (they'll return an explicit error).
var globalRobustClient *robustclient.Client

// SetRobustClient wires the process-wide robust-analyzer client. Called from
// apiserver.Init after the robustclient discovery is started. Safe to call
// once, before any handler runs.
func SetRobustClient(rc *robustclient.Client) {
	globalRobustClient = rc
}

// GetRobustClient returns the process-wide robust-analyzer client. Handlers
// should check for nil and return an error if Robust integration is not
// configured.
func GetRobustClient() *robustclient.Client {
	return globalRobustClient
}

// Handler handles HTTP requests for inference, playground, and dataset resources.
type Handler struct {
	k8sClient        client.Client
	dbClient         dbclient.Interface
	s3Client         commons3.Interface
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

// authorizeModel enforces resource-level RBAC for model write operations.
// It fails closed: a nil access controller denies the request rather than
// silently allowing it. For create, owner is empty; for update/delete, pass the
// model's owner (v1.GetUserId) and workspace so owner/workspace rules apply.
func (h *Handler) authorizeModel(c *gin.Context, verb v1.RoleVerb, owner, workspace string) error {
	if h.accessController == nil {
		return commonerrors.NewInternalError("model access controller is not initialized")
	}
	in := authority.AccessInput{
		Context:       c.Request.Context(),
		ResourceKind:  v1.ModelKind,
		Verb:          verb,
		UserId:        c.GetString(common.UserId),
		ResourceOwner: owner,
	}
	if workspace != "" {
		in.Workspaces = []string{workspace}
	}
	return h.accessController.Authorize(in)
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
// Errors that carry a Kubernetes-style status (including the commonerrors
// helpers such as NewForbidden/NewNotFound) map to their real HTTP code so
// authorization denials return 403 instead of a generic 500. Plain errors
// still fall back to 500.
func getHTTPStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		if code := int(status.Status().Code); code != 0 {
			return code
		}
	}
	return http.StatusInternalServerError
}
