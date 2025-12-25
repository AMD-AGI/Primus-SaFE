/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

type Handler struct {
	client.Client
	clientSet        kubernetes.Interface
	dbClient         dbclient.Interface
	httpClient       httpclient.Interface
	clientManager    *commonutils.ObjectManager
	accessController *authority.AccessController
}

// NewHandler creates a new Handler instance with the provided controller manager.
// It initializes all required clients and components including:
// - clientSet: Kubernetes clientSet for direct API access
// - dbClient: Database client (if database is enabled)
// - searchClient: OpenSearch client used for log search
// - httpClient: HTTP client for external requests
// - clientManager: Object manager for dataplane client caching
// - accessController: AccessController for access control
// Returns the initialized Handler or an error if initialization fails.
func NewHandler(mgr ctrlruntime.Manager) (*Handler, error) {
	clientSet, err := k8sclient.NewClientSetWithRestConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}
	var dbClient *dbclient.Client
	if commonconfig.IsDBEnable() {
		if dbClient = dbclient.NewClient(); dbClient == nil {
			return nil, fmt.Errorf("failed to new db client")
		}
	}
	h := &Handler{
		Client:           mgr.GetClient(),
		clientSet:        clientSet,
		dbClient:         dbClient,
		httpClient:       httpclient.NewClient(),
		clientManager:    commonutils.NewObjectManagerSingleton(),
		accessController: authority.NewAccessController(mgr.GetClient()),
	}
	return h, nil
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

// cvtToResourceList converts a Kubernetes ResourceList to a custom ResourceList type.
// It iterates through the resource list and converts each resource quantity to a numeric value.
// Negative resource values are converted to 0 to ensure valid resource representations.
// Returns the converted resource list or nil if the input list is empty.
func cvtToResourceList(resourceList corev1.ResourceList) view.ResourceList {
	if len(resourceList) == 0 {
		return nil
	}
	result := make(view.ResourceList)
	for key, val := range resourceList {
		if val.Value() < 0 {
			result[string(key)] = 0
		} else {
			result[string(key)] = val.Value()
		}
	}
	return result
}

// getAndSetUsername retrieves the user information based on the user ID stored in the context
// and sets the username in the context for further use.
// Returns the user object and any error encountered during the process.
// If no user ID is found in the context, it returns nil
func (h *Handler) getAndSetUsername(c *gin.Context) (*v1.User, error) {
	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, nil
	}
	user, err := h.accessController.GetRequestUser(c.Request.Context(), userId)
	if err != nil {
		return nil, err
	}

	c.Set(common.UserName, v1.GetUserName(user))
	return user, nil
}
