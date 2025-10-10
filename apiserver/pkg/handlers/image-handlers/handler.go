package image_handlers

import (
	"fmt"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
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
	return h, nil
}

type ImageHandler struct {
	client.Client
	clientSet *kubernetes.Clientset
	dbClient  dbclient.Interface
}

type handleFunc func(*gin.Context) (interface{}, error)

func handle(c *gin.Context, fn handleFunc) {
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
	switch rspType := rsp.(type) {
	case []byte:
		c.Data(code, jsonContentType, rspType)
	case string:
		c.Data(code, jsonContentType, []byte(rspType))
	default:
		c.JSON(code, rspType)
	}
}
