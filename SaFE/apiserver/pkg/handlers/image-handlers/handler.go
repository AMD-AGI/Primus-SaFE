/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

// NewImageHandler creates a new ImageHandler instance.
func NewImageHandler(mgr ctrlruntime.Manager) (*ImageHandler, error) {
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

	h := &ImageHandler{
		Client:           mgr.GetClient(),
		clientSet:        clientSet,
		dbClient:         dbClient,
		httpClient:       httpclient.NewClient(),
		accessController: authority.NewAccessController(mgr.GetClient()),
	}
	err = h.initHarbor(context.Background())
	if err != nil {
		klog.Warningf("failed to init harbor: %v", err)
	}
	return h, nil
}

type ImageHandler struct {
	client.Client
	clientSet        kubernetes.Interface
	dbClient         dbclient.Interface
	httpClient       httpclient.Interface
	accessController *authority.AccessController
}

type handleFunc[T any] func(*gin.Context) (T, error)

func handle[T any](c *gin.Context, fn handleFunc[T]) {
	rsp, err := fn(c)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	code := http.StatusOK
	// If a status was previously set, use that status in the response.
	if c.Writer.Status() > 0 {
		code = c.Writer.Status()
	}
	switch rspType := any(rsp).(type) {
	case []byte:
		c.Data(code, common.JsonContentType, rspType)
	case string:
		c.Data(code, common.JsonContentType, []byte(rspType))
	default:
		c.JSON(code, rspType)
	}
}

func (h *ImageHandler) listImagePullSecretsName(ctx context.Context, clusterClient client.Client, namespace string) ([]string, error) {
	secrets := &corev1.SecretList{}
	err := clusterClient.List(ctx, secrets, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}
	var secretNames []string
	for _, secret := range secrets.Items {
		if secret.Type == corev1.SecretTypeDockerConfigJson {
			secretNames = append(secretNames, secret.Name)
		}
	}
	return secretNames, nil
}
