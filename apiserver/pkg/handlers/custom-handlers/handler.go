/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/controllers"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

var (
	jsonContentType = "application/json; charset=utf-8"
	passKey         = "pass"
)

type Handler struct {
	client.Client
	clientSet      *kubernetes.Clientset
	httpClient     httpclient.Interface
	clusterManager *controllers.ClusterManager
}

func NewHandler(mgr ctrlruntime.Manager) (*Handler, error) {
	clientSet, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	h := &Handler{
		Client:         mgr.GetClient(),
		clientSet:      clientSet,
		httpClient:     httpclient.Instance(),
		clusterManager: controllers.NewClusterManager(),
	}
	return h, nil
}

type handleFunc func(*gin.Context) (interface{}, error)

func handle(c *gin.Context, fn handleFunc) {
	rsp, err := fn(c)
	_, ok := c.Get(passKey)
	if ok {
		return
	}
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	code := http.StatusOK
	if c.Writer.Status() > 0 {
		code = c.Writer.Status()
	}
	switch rspType := rsp.(type) {
	case []byte:
		c.Data(code, jsonContentType, rspType)
	case string:
		c.Data(code, jsonContentType, []byte(rspType))
	default:
		c.JSON(code, rspType)
	}
}

func getBodyFromRequest(req *http.Request, bodyStruct interface{}) ([]byte, error) {
	body, err := apiutils.ReadBody(req)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	if err = jsonutils.UnmarshalWithCheck(body, bodyStruct); err != nil {
		return body, commonerrors.NewBadRequest(err.Error())
	}
	return body, nil
}

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
