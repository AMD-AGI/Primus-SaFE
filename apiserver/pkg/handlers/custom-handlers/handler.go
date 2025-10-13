/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

var (
	jsonContentType = "application/json; charset=utf-8"
)

type Handler struct {
	client.Client
	clientSet     *kubernetes.Clientset
	dbClient      dbclient.Interface
	searchClient  *commonsearch.SearchClient
	httpClient    httpclient.Interface
	clientManager *commonutils.ObjectManager
	auth          *authority.Authorizer
}

// NewHandler: creates a new Handler instance with the provided controller manager.
// It initializes all required clients and components including:
// - clientSet: Kubernetes clientset for direct API access
// - dbClient: Database client (if database is enabled)
// - searchClient: OpenSearch client used for log search
// - httpClient: HTTP client for external requests
// - clientManager: Object manager for dataplane client caching
// - auth: Authorizer for access control
// Returns the initialized Handler or an error if initialization fails.
func NewHandler(mgr ctrlruntime.Manager) (*Handler, error) {
	clientSet, err := kubernetes.NewForConfig(mgr.GetConfig())
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
		Client:        mgr.GetClient(),
		clientSet:     clientSet,
		searchClient:  commonsearch.NewClient(),
		dbClient:      dbClient,
		httpClient:    httpclient.NewHttpClient(),
		clientManager: commonutils.NewObjectManagerSingleton(),
		auth:          authority.NewAuthorizer(mgr.GetClient()),
	}
	return h, nil
}

type handleFunc func(*gin.Context) (interface{}, error)

// handle: is a middleware function that executes the provided handler function and processes its response.
// It handles errors by aborting the request with an API error, and formats successful responses
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
		c.Data(code, jsonContentType, responseType)
	case string:
		c.Data(code, jsonContentType, []byte(responseType))
	default:
		c.JSON(code, responseType)
	}
}

// parseRequestBody: reads the request body and unmarshals it into the provided struct.
// It returns the raw body bytes and any error encountered during the process.
// If the body is empty, it returns nil for both body and error.
// If JSON unmarshaling fails, it returns a BadRequest error with the unmarshaling error details.
func parseRequestBody(req *http.Request, bodyStruct interface{}) ([]byte, error) {
	body, err := apiutils.ReadBody(req)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	if err = jsonutils.Unmarshal(body, bodyStruct); err != nil {
		return body, commonerrors.NewBadRequest(err.Error())
	}
	return body, nil
}

// cvtToResourceList: converts a Kubernetes ResourceList to a custom ResourceList type.
// It iterates through the resource list and converts each resource quantity to a numeric value.
// Negative resource values are converted to 0 to ensure valid resource representations.
// Returns the converted resource list or nil if the input list is empty.
func cvtToResourceList(resourceList corev1.ResourceList) types.ResourceList {
	if len(resourceList) == 0 {
		return nil
	}
	result := make(types.ResourceList)
	for key, val := range resourceList {
		if val.Value() < 0 {
			result[string(key)] = 0
		} else {
			result[string(key)] = val.Value()
		}
	}
	return result
}

// getAndSetUsername: retrieves the user information based on the user ID stored in the context
// and sets the username in the context for further use.
// Returns the user object and any error encountered during the process.
// If no user ID is found in the context, it returns nil
func (h *Handler) getAndSetUsername(c *gin.Context) (*v1.User, error) {
	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, nil
	}
	user, err := h.auth.GetRequestUser(c.Request.Context(), userId)
	if err != nil {
		return nil, err
	}

	c.Set(common.UserName, v1.GetUserName(user))
	return user, nil
}
