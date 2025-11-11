package exporter

import (
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
)

func (h *Handler) register() {
	router.RegisterGroup(func(group *gin.RouterGroup) error {
		g := group.Group("/net-flow")
		{
			g.GET("/tcp-listen", func(c *gin.Context) {
				ports, err := h.debugGetTcpListen(c)
				if err != nil {
					_ = c.Error(err)
					return
				}
				c.JSON(http.StatusOK, rest.SuccessResp(c, ports))
			})
			g.GET("/tcp-file", func(c *gin.Context) {
				file, err := h.debugGetTcpFile(c)
				if err != nil {
					_ = c.Error(err)
					return
				}
				c.String(http.StatusOK, file)
			})
		}
		return nil
	})
}

func (h *Handler) debugGetTcpListen(ctx context.Context) ([]int, error) {
	return h.getAllListingPort()
}

func (h *Handler) debugGetTcpFile(ctx context.Context) (string, error) {
	result, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		return "", err
	}
	return string(result), nil
}
