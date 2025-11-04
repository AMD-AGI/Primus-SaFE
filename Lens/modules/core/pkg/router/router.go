package router

import (
	"github.com/AMD-AGI/primus-lens/core/pkg/router/middleware"
	"github.com/gin-gonic/gin"
)

var (
	groupRegisters []GroupRegister
)

func RegisterGroup(group GroupRegister) {
	groupRegisters = append(groupRegisters, group)
}

func InitRouter(engine *gin.Engine) error {
	g := engine.Group("/v1")
	g.Use(middleware.HandleLogging())
	g.Use(middleware.HandleErrors())
	g.Use(middleware.HandleTracing())
	g.Use(middleware.CorsMiddleware())
	for _, group := range groupRegisters {
		err := group(g)
		if err != nil {
			return err
		}
	}
	return nil
}

type RouterRegister func(engine *gin.Engine) error

type GroupRegister func(group *gin.RouterGroup) error
