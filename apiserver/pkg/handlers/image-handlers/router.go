package image_handlers

import "github.com/gin-gonic/gin"

func InitImageRouter(e *gin.Engine, h *ImageHandler) {
	group := e.Group("/api/v1/")
	{
		group.GET("harbor/stats", func(c *gin.Context) {
			handle(c, h.GetHarborStats)
		})
	}
}
