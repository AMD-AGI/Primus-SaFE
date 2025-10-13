package image_handlers

import (
	"context"
	"fmt"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"net/http"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	jsonContentType = "application/json; charset=utf-8"
)

func NewImageHandler(mgr ctrlruntime.Manager) (*ImageHandler, error) {
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

	h := &ImageHandler{
		Client:    mgr.GetClient(),
		clientSet: clientSet,
		dbClient:  dbClient,
	}
	err = h.initHarbor(context.Background())
	if err != nil {
		klog.Warningf("failed to init harbor: %v", err)
	}
	return h, nil
}

type ImageHandler struct {
	client.Client
	clientSet  *kubernetes.Clientset
	dbClient   dbclient.Interface
	httpClient httpclient.Interface
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
