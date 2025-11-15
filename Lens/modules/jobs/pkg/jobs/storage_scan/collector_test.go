package storage_scan

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockDriver is a mock implementation of Driver interface for testing
type mockDriver struct {
	name string
}

func (m *mockDriver) Name() string {
	return m.name
}

func (m *mockDriver) Detect(ctx context.Context, dctx DriverContext) (int, error) {
	return 0, nil
}

func (m *mockDriver) ListBackends(ctx context.Context, dctx DriverContext) ([]string, error) {
	return nil, nil
}

func (m *mockDriver) Collect(ctx context.Context, dctx DriverContext, backend string) (BackendReport, error) {
	return BackendReport{}, nil
}

func TestRegister(t *testing.T) {
	// Clear drivers map before test
	regMu.Lock()
	originalDrivers := drivers
	drivers = make(map[string]Driver)
	regMu.Unlock()

	// Restore original drivers after test
	defer func() {
		regMu.Lock()
		drivers = originalDrivers
		regMu.Unlock()
	}()

	driver := &mockDriver{name: "test-driver"}

	// Test successful registration
	Register(driver)

	regMu.RLock()
	registered, exists := drivers[driver.Name()]
	regMu.RUnlock()

	assert.True(t, exists, "Driver should be registered")
	assert.Equal(t, driver, registered, "Registered driver should match")
}

func TestRegisterDuplicate(t *testing.T) {
	// Clear drivers map before test
	regMu.Lock()
	originalDrivers := drivers
	drivers = make(map[string]Driver)
	regMu.Unlock()

	// Restore original drivers after test
	defer func() {
		regMu.Lock()
		drivers = originalDrivers
		regMu.Unlock()
	}()

	driver1 := &mockDriver{name: "duplicate-driver"}
	driver2 := &mockDriver{name: "duplicate-driver"}

	// Register first driver
	Register(driver1)

	// Attempt to register duplicate should panic
	assert.Panics(t, func() {
		Register(driver2)
	}, "Registering duplicate driver should panic")
}

func TestRegisterMultipleDrivers(t *testing.T) {
	// Clear drivers map before test
	regMu.Lock()
	originalDrivers := drivers
	drivers = make(map[string]Driver)
	regMu.Unlock()

	// Restore original drivers after test
	defer func() {
		regMu.Lock()
		drivers = originalDrivers
		regMu.Unlock()
	}()

	driver1 := &mockDriver{name: "driver1"}
	driver2 := &mockDriver{name: "driver2"}
	driver3 := &mockDriver{name: "driver3"}

	Register(driver1)
	Register(driver2)
	Register(driver3)

	regMu.RLock()
	defer regMu.RUnlock()

	assert.Len(t, drivers, 3, "Should have 3 registered drivers")
	assert.Contains(t, drivers, "driver1", "Should contain driver1")
	assert.Contains(t, drivers, "driver2", "Should contain driver2")
	assert.Contains(t, drivers, "driver3", "Should contain driver3")
}

func TestRegisterPanicMessage(t *testing.T) {
	// Clear drivers map before test
	regMu.Lock()
	originalDrivers := drivers
	drivers = make(map[string]Driver)
	regMu.Unlock()

	// Restore original drivers after test
	defer func() {
		regMu.Lock()
		drivers = originalDrivers
		regMu.Unlock()
	}()

	driverName := "test-duplicate"
	driver1 := &mockDriver{name: driverName}
	driver2 := &mockDriver{name: driverName}

	Register(driver1)

	// Check panic message contains driver name
	defer func() {
		if r := recover(); r != nil {
			panicMsg, ok := r.(string)
			assert.True(t, ok, "Panic value should be a string")
			assert.Contains(t, panicMsg, "duplicate driver", "Panic message should mention duplicate driver")
			assert.Contains(t, panicMsg, driverName, "Panic message should contain driver name")
		}
	}()

	Register(driver2)
}

func TestRegisterThreadSafety(t *testing.T) {
	// Clear drivers map before test
	regMu.Lock()
	originalDrivers := drivers
	drivers = make(map[string]Driver)
	regMu.Unlock()

	// Restore original drivers after test
	defer func() {
		regMu.Lock()
		drivers = originalDrivers
		regMu.Unlock()
	}()

	// Test concurrent registrations (different drivers)
	done := make(chan bool)
	
	for i := 0; i < 10; i++ {
		go func(idx int) {
			driver := &mockDriver{name: string(rune('a' + idx))}
			Register(driver)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	regMu.RLock()
	count := len(drivers)
	regMu.RUnlock()

	assert.Equal(t, 10, count, "All drivers should be registered")
}

func TestDriverName(t *testing.T) {
	tests := []struct {
		name         string
		driver       *mockDriver
		expectedName string
	}{
		{
			name:         "simple name",
			driver:       &mockDriver{name: "simple"},
			expectedName: "simple",
		},
		{
			name:         "name with hyphen",
			driver:       &mockDriver{name: "my-driver"},
			expectedName: "my-driver",
		},
		{
			name:         "name with underscore",
			driver:       &mockDriver{name: "my_driver"},
			expectedName: "my_driver",
		},
		{
			name:         "empty name",
			driver:       &mockDriver{name: ""},
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.driver.Name()
			assert.Equal(t, tt.expectedName, result, "Driver name should match expected")
		})
	}
}

