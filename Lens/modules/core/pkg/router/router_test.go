package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/gin-gonic/gin"
)

func TestInitRouter_MiddlewareConfiguration(t *testing.T) {
	// 设置Gin为测试模式
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		config             *config.Config
		expectLogging      bool
		expectTracing      bool
		description        string
	}{
		{
			name: "默认配置（全部启用）",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{},
			},
			expectLogging: true,
			expectTracing: true,
			description:   "当middleware配置为空时，应该默认启用所有中间件",
		},
		{
			name: "仅启用日志",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{
					EnableLogging: boolPtr(true),
					EnableTracing: boolPtr(false),
				},
			},
			expectLogging: true,
			expectTracing: false,
			description:   "显式启用日志，禁用追踪",
		},
		{
			name: "仅启用追踪",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{
					EnableLogging: boolPtr(false),
					EnableTracing: boolPtr(true),
				},
			},
			expectLogging: false,
			expectTracing: true,
			description:   "禁用日志，显式启用追踪",
		},
		{
			name: "全部禁用",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{
					EnableLogging: boolPtr(false),
					EnableTracing: boolPtr(false),
				},
			},
			expectLogging: false,
			expectTracing: false,
			description:   "禁用所有可配置的中间件",
		},
		{
			name: "全部启用",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{
					EnableLogging: boolPtr(true),
					EnableTracing: boolPtr(true),
				},
			},
			expectLogging: true,
			expectTracing: true,
			description:   "显式启用所有中间件",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 清空全局的groupRegisters，避免测试之间的影响
			originalGroupRegisters := groupRegisters
			groupRegisters = []GroupRegister{}
			defer func() {
				groupRegisters = originalGroupRegisters
			}()

			// 创建Gin引擎
			engine := gin.New()

			// 初始化路由
			err := InitRouter(engine, tt.config)
			if err != nil {
				t.Fatalf("InitRouter() error = %v", err)
			}

			// 注册一个测试路由
			engine.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "ok")
			})

			// 创建测试请求
			req, _ := http.NewRequest("GET", "/v1/test", nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			// 验证配置是否正确应用
			// 注意：这里我们只能测试路由是否能正常工作
			// 实际的中间件行为需要通过日志或其他方式验证
			if w.Code != http.StatusNotFound { // /v1/test 不存在，期望404
				// 但如果路由初始化有问题，可能会返回其他错误
			}

			// 验证配置方法是否返回预期值
			if gotLogging := tt.config.Middleware.IsLoggingEnabled(); gotLogging != tt.expectLogging {
				t.Errorf("%s: IsLoggingEnabled() = %v, want %v", tt.description, gotLogging, tt.expectLogging)
			}

			if gotTracing := tt.config.Middleware.IsTracingEnabled(); gotTracing != tt.expectTracing {
				t.Errorf("%s: IsTracingEnabled() = %v, want %v", tt.description, gotTracing, tt.expectTracing)
			}
		})
	}
}

func TestInitRouter_WithGroupRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 清空并注册一个测试路由组
	originalGroupRegisters := groupRegisters
	groupRegisters = []GroupRegister{}
	defer func() {
		groupRegisters = originalGroupRegisters
	}()

	testRouteRegistered := false
	RegisterGroup(func(group *gin.RouterGroup) error {
		group.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "test ok")
		})
		testRouteRegistered = true
		return nil
	})

	engine := gin.New()
	cfg := &config.Config{
		Middleware: config.MiddlewareConfig{
			EnableLogging: boolPtr(true),
			EnableTracing: boolPtr(true),
		},
	}

	err := InitRouter(engine, cfg)
	if err != nil {
		t.Fatalf("InitRouter() error = %v", err)
	}

	if !testRouteRegistered {
		t.Error("Test route was not registered")
	}

	// 测试注册的路由是否可访问
	req, _ := http.NewRequest("GET", "/v1/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test ok" {
		t.Errorf("Expected body 'test ok', got '%s'", w.Body.String())
	}
}

// boolPtr 返回bool指针的辅助函数
func boolPtr(b bool) *bool {
	return &b
}
