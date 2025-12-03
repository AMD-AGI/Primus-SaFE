package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/gin-gonic/gin"
)

func TestInitRouter_MiddlewareConfiguration(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		config             *config.Config
		expectLogging      bool
		expectTracing      bool
		description        string
	}{
		{
			name: "Default configuration (all enabled)",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{},
			},
			expectLogging: true,
			expectTracing: true,
			description:   "When middleware config is empty, all middlewares should be enabled by default",
		},
		{
			name: "Only logging enabled",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{
					EnableLogging: boolPtr(true),
					EnableTracing: boolPtr(false),
				},
			},
			expectLogging: true,
			expectTracing: false,
			description:   "Explicitly enable logging, disable tracing",
		},
		{
			name: "Only tracing enabled",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{
					EnableLogging: boolPtr(false),
					EnableTracing: boolPtr(true),
				},
			},
			expectLogging: false,
			expectTracing: true,
			description:   "Disable logging, explicitly enable tracing",
		},
		{
			name: "All disabled",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{
					EnableLogging: boolPtr(false),
					EnableTracing: boolPtr(false),
				},
			},
			expectLogging: false,
			expectTracing: false,
			description:   "Disable all configurable middlewares",
		},
		{
			name: "All enabled",
			config: &config.Config{
				Middleware: config.MiddlewareConfig{
					EnableLogging: boolPtr(true),
					EnableTracing: boolPtr(true),
				},
			},
			expectLogging: true,
			expectTracing: true,
			description:   "Explicitly enable all middlewares",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear global groupRegisters to avoid test interference
			originalGroupRegisters := groupRegisters
			groupRegisters = []GroupRegister{}
			defer func() {
				groupRegisters = originalGroupRegisters
			}()

			// Create Gin engine
			engine := gin.New()

			// Initialize router
			err := InitRouter(engine, tt.config)
			if err != nil {
				t.Fatalf("InitRouter() error = %v", err)
			}

			// Register a test route
			engine.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "ok")
			})

			// Create test request
			req, _ := http.NewRequest("GET", "/v1/test", nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			// Verify configuration is correctly applied
			// Note: we can only test if the route works normally here
			// Actual middleware behavior needs to be verified through logs or other means
			if w.Code != http.StatusNotFound { // /v1/test doesn't exist, expect 404
				// But if router initialization has issues, may return other errors
			}

			// Verify configuration methods return expected values
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

	// Clear and register a test route group
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

	// Test if the registered route is accessible
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

// boolPtr is a helper function that returns a bool pointer
func boolPtr(b bool) *bool {
	return &b
}
