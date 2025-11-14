package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"sync"
)

var once sync.Once

var engine *gin.Engine

var defaultGather prometheus.Gatherer

func init() {
	defaultGather = prometheus.DefaultGatherer
	AddRegister(addMetrics)
}

var registers []func(g *gin.RouterGroup)
var registersMu sync.Mutex

func SetDefaultGather(g prometheus.Gatherer) {
	defaultGather = g
}

func AddRegister(register func(g *gin.RouterGroup)) {
	registersMu.Lock()
	defer registersMu.Unlock()
	registers = append(registers, register)
}

func AddDefaultRegister(path string, method func() (interface{}, error)) {
	AddRegister(func(g *gin.RouterGroup) {
		g.GET(path, func(c *gin.Context) {
			data, err := method()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, data)
		})
	})
}

func InitHealthServer(port int) {
	once.Do(func() {
		engine = gin.New()
		g := engine.Group("")
		g.Use(gin.Recovery())
		g.Use(gin.Logger())
		
		// Apply all registered routes
		registersMu.Lock()
		for _, reg := range registers {
			reg(g)
		}
		registersMu.Unlock()
		
		go func() {
			engine.Run(fmt.Sprintf(":%d", port))
		}()
	})
}

func addMetrics(g *gin.RouterGroup) {
	g.GET("/metrics", func(c *gin.Context) {
		h := promhttp.HandlerFor(
			defaultGather,
			promhttp.HandlerOpts{
				EnableOpenMetrics: true,
			},
		)
		h.ServeHTTP(c.Writer, c.Request)
	})
}
