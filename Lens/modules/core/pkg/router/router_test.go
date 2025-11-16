package router

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Setup helper to reset global state before each test
func resetGroupRegisters() {
	groupRegisters = []GroupRegister{}
}

// TestRegisterGroup tests the RegisterGroup function
func TestRegisterGroup(t *testing.T) {
	resetGroupRegisters()

	// Create a test group register
	testRegister := func(group *gin.RouterGroup) error {
		group.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "test")
		})
		return nil
	}

	// Register the group
	RegisterGroup(testRegister)

	// Verify it was added
	assert.Len(t, groupRegisters, 1)
}

// TestRegisterGroup_Multiple tests registering multiple groups
func TestRegisterGroup_Multiple(t *testing.T) {
	resetGroupRegisters()

	// Create multiple test registers
	register1 := func(group *gin.RouterGroup) error {
		group.GET("/route1", func(c *gin.Context) {})
		return nil
	}
	register2 := func(group *gin.RouterGroup) error {
		group.GET("/route2", func(c *gin.Context) {})
		return nil
	}
	register3 := func(group *gin.RouterGroup) error {
		group.GET("/route3", func(c *gin.Context) {})
		return nil
	}

	// Register multiple groups
	RegisterGroup(register1)
	RegisterGroup(register2)
	RegisterGroup(register3)

	// Verify all were added
	assert.Len(t, groupRegisters, 3)
}

// TestRegisterGroup_Order tests that groups are registered in order
func TestRegisterGroup_Order(t *testing.T) {
	resetGroupRegisters()

	var executionOrder []int

	// Create registers that track execution order
	register1 := func(group *gin.RouterGroup) error {
		executionOrder = append(executionOrder, 1)
		return nil
	}
	register2 := func(group *gin.RouterGroup) error {
		executionOrder = append(executionOrder, 2)
		return nil
	}
	register3 := func(group *gin.RouterGroup) error {
		executionOrder = append(executionOrder, 3)
		return nil
	}

	// Register in specific order
	RegisterGroup(register1)
	RegisterGroup(register2)
	RegisterGroup(register3)

	// Initialize router to trigger execution
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	err := InitRouter(engine)
	require.NoError(t, err)

	// Verify execution order
	assert.Equal(t, []int{1, 2, 3}, executionOrder)
}

// TestInitRouter tests the InitRouter function with successful registration
func TestInitRouter(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	// Create a test group register
	testRegister := func(group *gin.RouterGroup) error {
		group.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		return nil
	}

	RegisterGroup(testRegister)

	// Initialize router
	engine := gin.New()
	err := InitRouter(engine)

	// Verify no error
	require.NoError(t, err)

	// Test the registered route
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/health", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

// TestInitRouter_WithError tests InitRouter when a group register returns an error
func TestInitRouter_WithError(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	expectedErr := errors.New("registration failed")

	// Create a failing group register
	failingRegister := func(group *gin.RouterGroup) error {
		return expectedErr
	}

	RegisterGroup(failingRegister)

	// Initialize router
	engine := gin.New()
	err := InitRouter(engine)

	// Verify error is returned
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// TestInitRouter_PartialFailure tests that InitRouter stops on first error
func TestInitRouter_PartialFailure(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	var executionCount int
	expectedErr := errors.New("second register failed")

	// First register succeeds
	register1 := func(group *gin.RouterGroup) error {
		executionCount++
		return nil
	}

	// Second register fails
	register2 := func(group *gin.RouterGroup) error {
		executionCount++
		return expectedErr
	}

	// Third register should not execute
	register3 := func(group *gin.RouterGroup) error {
		executionCount++
		return nil
	}

	RegisterGroup(register1)
	RegisterGroup(register2)
	RegisterGroup(register3)

	// Initialize router
	engine := gin.New()
	err := InitRouter(engine)

	// Verify error is returned and only first two registers executed
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 2, executionCount)
}

// TestInitRouter_GroupPath tests that routes are registered under /v1
func TestInitRouter_GroupPath(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	// Create a test register
	testRegister := func(group *gin.RouterGroup) error {
		group.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "success")
		})
		return nil
	}

	RegisterGroup(testRegister)

	// Initialize router
	engine := gin.New()
	err := InitRouter(engine)
	require.NoError(t, err)

	// Test that route is accessible under /v1
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/test", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test that route is NOT accessible without /v1 prefix
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	engine.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code)
}

// TestInitRouter_EmptyRegisters tests InitRouter with no registered groups
func TestInitRouter_EmptyRegisters(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	err := InitRouter(engine)

	// Should succeed even with no groups
	require.NoError(t, err)
}

// TestInitRouter_MultipleRoutes tests multiple routes in different groups
func TestInitRouter_MultipleRoutes(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	// Register first group
	register1 := func(group *gin.RouterGroup) error {
		group.GET("/users", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"resource": "users"})
		})
		return nil
	}

	// Register second group
	register2 := func(group *gin.RouterGroup) error {
		group.GET("/posts", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"resource": "posts"})
		})
		return nil
	}

	RegisterGroup(register1)
	RegisterGroup(register2)

	// Initialize router
	engine := gin.New()
	err := InitRouter(engine)
	require.NoError(t, err)

	// Test first route
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/v1/users", nil)
	engine.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Contains(t, w1.Body.String(), "users")

	// Test second route
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/v1/posts", nil)
	engine.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "posts")
}

// TestInitRouter_NestedGroups tests nested route groups
func TestInitRouter_NestedGroups(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	// Create a register with nested groups
	nestedRegister := func(group *gin.RouterGroup) error {
		apiGroup := group.Group("/api")
		{
			apiGroup.GET("/version", func(c *gin.Context) {
				c.String(http.StatusOK, "v1.0")
			})
		}
		return nil
	}

	RegisterGroup(nestedRegister)

	// Initialize router
	engine := gin.New()
	err := InitRouter(engine)
	require.NoError(t, err)

	// Test nested route
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/api/version", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "v1.0", w.Body.String())
}

// TestInitRouter_HTTPMethods tests different HTTP methods
func TestInitRouter_HTTPMethods(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	// Register routes with different HTTP methods
	methodsRegister := func(group *gin.RouterGroup) error {
		group.GET("/resource", func(c *gin.Context) {
			c.String(http.StatusOK, "GET")
		})
		group.POST("/resource", func(c *gin.Context) {
			c.String(http.StatusOK, "POST")
		})
		group.PUT("/resource", func(c *gin.Context) {
			c.String(http.StatusOK, "PUT")
		})
		group.DELETE("/resource", func(c *gin.Context) {
			c.String(http.StatusOK, "DELETE")
		})
		return nil
	}

	RegisterGroup(methodsRegister)

	// Initialize router
	engine := gin.New()
	err := InitRouter(engine)
	require.NoError(t, err)

	tests := []struct {
		method   string
		expected string
	}{
		{"GET", "GET"},
		{"POST", "POST"},
		{"PUT", "PUT"},
		{"DELETE", "DELETE"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, "/v1/resource", nil)
			engine.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tt.expected, w.Body.String())
		})
	}
}

// TestInitRouter_Middleware tests that middlewares are applied
func TestInitRouter_Middleware(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	middlewareCalled := false

	// Create a custom test middleware to verify middleware chain
	testMiddleware := func(c *gin.Context) {
		middlewareCalled = true
		c.Next()
	}

	// Register a simple route
	testRegister := func(group *gin.RouterGroup) error {
		group.Use(testMiddleware)
		group.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})
		return nil
	}

	RegisterGroup(testRegister)

	// Initialize router
	engine := gin.New()
	err := InitRouter(engine)
	require.NoError(t, err)

	// Make a request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/test", nil)
	engine.ServeHTTP(w, req)

	// Verify middleware was called
	assert.True(t, middlewareCalled)
}

// TestGroupRegister_FunctionType tests that GroupRegister can be used as expected
func TestGroupRegister_FunctionType(t *testing.T) {
	// Test that we can create and use GroupRegister functions
	var register GroupRegister = func(group *gin.RouterGroup) error {
		group.GET("/test", func(c *gin.Context) {})
		return nil
	}

	// Verify it's callable
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	routerGroup := engine.Group("/v1")
	err := register(routerGroup)
	assert.NoError(t, err)
}

// TestRouterRegister_FunctionType tests that RouterRegister can be used as expected
func TestRouterRegister_FunctionType(t *testing.T) {
	// Test that we can create and use RouterRegister functions
	var register RouterRegister = func(engine *gin.Engine) error {
		engine.GET("/test", func(c *gin.Context) {})
		return nil
	}

	// Verify it's callable
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	err := register(engine)
	assert.NoError(t, err)
}

// TestInitRouter_SeparateEngines tests that multiple engines can be initialized independently
func TestInitRouter_SeparateEngines(t *testing.T) {
	resetGroupRegisters()
	gin.SetMode(gin.TestMode)

	// Register a simple group
	testRegister := func(group *gin.RouterGroup) error {
		group.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})
		return nil
	}

	RegisterGroup(testRegister)

	// Initialize first engine
	engine1 := gin.New()
	err1 := InitRouter(engine1)
	require.NoError(t, err1)

	// Initialize second engine (separate instance)
	engine2 := gin.New()
	err2 := InitRouter(engine2)
	require.NoError(t, err2)

	// Both engines should work independently
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/v1/test", nil)
	engine1.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/v1/test", nil)
	engine2.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// BenchmarkRegisterGroup benchmarks the RegisterGroup operation
func BenchmarkRegisterGroup(b *testing.B) {
	testRegister := func(group *gin.RouterGroup) error {
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetGroupRegisters()
		RegisterGroup(testRegister)
	}
}

// BenchmarkInitRouter benchmarks the InitRouter operation
func BenchmarkInitRouter(b *testing.B) {
	gin.SetMode(gin.TestMode)
	
	testRegister := func(group *gin.RouterGroup) error {
		group.GET("/test", func(c *gin.Context) {})
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetGroupRegisters()
		RegisterGroup(testRegister)
		engine := gin.New()
		_ = InitRouter(engine)
	}
}

// BenchmarkInitRouter_MultipleGroups benchmarks InitRouter with multiple groups
func BenchmarkInitRouter_MultipleGroups(b *testing.B) {
	gin.SetMode(gin.TestMode)

	registers := make([]GroupRegister, 10)
	for i := range registers {
		index := i // Capture loop variable
		registers[i] = func(group *gin.RouterGroup) error {
			// Use different paths to avoid duplicate registration
			group.GET("/test"+string(rune('0'+index)), func(c *gin.Context) {})
			return nil
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetGroupRegisters()
		for _, reg := range registers {
			RegisterGroup(reg)
		}
		engine := gin.New()
		_ = InitRouter(engine)
	}
}

