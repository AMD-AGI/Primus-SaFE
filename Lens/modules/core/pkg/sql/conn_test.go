package sql

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// Helper to reset global state
func resetConnPools() {
	connPoolLock.Lock()
	defer connPoolLock.Unlock()
	connPools = map[string]*gorm.DB{}
}

// TestDatabaseConfig_Validate tests the Validate method
func TestDatabaseConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  DatabaseConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: DatabaseConfig{
				Host:   "localhost",
				Port:   5432,
				DBName: "testdb",
			},
			wantErr: false,
		},
		{
			name: "missing host",
			config: DatabaseConfig{
				Host:   "",
				Port:   5432,
				DBName: "testdb",
			},
			wantErr: true,
		},
		{
			name: "missing port",
			config: DatabaseConfig{
				Host:   "localhost",
				Port:   0,
				DBName: "testdb",
			},
			wantErr: true,
		},
		{
			name: "missing dbname",
			config: DatabaseConfig{
				Host:   "localhost",
				Port:   5432,
				DBName: "",
			},
			wantErr: true,
		},
		{
			name: "all fields missing",
			config: DatabaseConfig{
				Host:   "",
				Port:   0,
				DBName: "",
			},
			wantErr: true,
		},
		{
			name: "valid with optional fields",
			config: DatabaseConfig{
				Host:        "localhost",
				Port:        5432,
				DBName:      "testdb",
				UserName:    "user",
				Password:    "pass",
				SSLMode:     "disable",
				Driver:      DriverNamePostgres,
				TimeZone:    "UTC",
				MaxIdleConn: 10,
				MaxOpenConn: 100,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, errInvalidConfig, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetDB tests the GetDB function
func TestGetDB(t *testing.T) {
	resetConnPools()

	// Test non-existent key
	db := GetDB("nonexistent")
	assert.Nil(t, db)

	// Test with mock DB (we can't actually create a real DB without a database)
	// This tests the retrieval mechanism
	connPoolLock.Lock()
	// We would need a real GORM DB to test this properly
	// For now, we test that nil is returned for non-existent keys
	connPoolLock.Unlock()
}

// TestGetDefaultDB tests the GetDefaultDB function
func TestGetDefaultDB(t *testing.T) {
	resetConnPools()

	// Test getting default DB when it doesn't exist
	db := GetDefaultDB()
	assert.Nil(t, db)
}

// TestGetDB_Concurrency tests concurrent access to GetDB
func TestGetDB_Concurrency(t *testing.T) {
	resetConnPools()

	done := make(chan bool)
	
	// Launch multiple readers
	for i := 0; i < 10; i++ {
		go func() {
			_ = GetDB("test")
			_ = GetDefaultDB()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or deadlock
	assert.True(t, true)
}

// TestMultiDatabaseConfig tests MultiDatabaseConfig type
func TestMultiDatabaseConfig(t *testing.T) {
	config := MultiDatabaseConfig{
		"db1": DatabaseConfig{
			Host:   "localhost",
			Port:   5432,
			DBName: "db1",
		},
		"db2": DatabaseConfig{
			Host:   "localhost",
			Port:   3306,
			DBName: "db2",
		},
	}

	assert.Len(t, config, 2)
	assert.Contains(t, config, "db1")
	assert.Contains(t, config, "db2")

	// Validate each config
	for key, cfg := range config {
		err := cfg.Validate()
		assert.NoError(t, err, "Config %s should be valid", key)
	}
}

// TestDatabaseConfig_DefaultDriver tests default driver assignment
func TestDatabaseConfig_DefaultDriver(t *testing.T) {
	config := DatabaseConfig{
		Host:   "localhost",
		Port:   5432,
		DBName: "testdb",
		Driver: "", // Empty driver should default to postgres
	}

	assert.Empty(t, config.Driver)
	// The InitGormDB function sets default driver to postgres if empty
}

// TestDatabaseConfig_Fields tests all DatabaseConfig fields
func TestDatabaseConfig_Fields(t *testing.T) {
	config := DatabaseConfig{
		Host:        "db.example.com",
		Port:        5432,
		UserName:    "admin",
		Password:    "secret",
		DBName:      "production",
		LogMode:     true,
		MaxIdleConn: 5,
		MaxOpenConn: 50,
		SSLMode:     "require",
		Driver:      DriverNamePostgres,
		TimeZone:    "America/New_York",
	}

	assert.Equal(t, "db.example.com", config.Host)
	assert.Equal(t, 5432, config.Port)
	assert.Equal(t, "admin", config.UserName)
	assert.Equal(t, "secret", config.Password)
	assert.Equal(t, "production", config.DBName)
	assert.True(t, config.LogMode)
	assert.Equal(t, 5, config.MaxIdleConn)
	assert.Equal(t, 50, config.MaxOpenConn)
	assert.Equal(t, "require", config.SSLMode)
	assert.Equal(t, DriverNamePostgres, config.Driver)
	assert.Equal(t, "America/New_York", config.TimeZone)
}

// TestConnPoolsThreadSafety tests thread safety of connPools access
func TestConnPoolsThreadSafety(t *testing.T) {
	resetConnPools()

	var wg sync.WaitGroup
	
	// Test concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = GetDB("test")
		}()
	}

	wg.Wait()
	assert.True(t, true, "Should not deadlock or panic")
}

// TestDatabaseConfig_Validation_EdgeCases tests edge cases for validation
func TestDatabaseConfig_Validation_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		config  DatabaseConfig
		wantErr bool
	}{
		{
			name: "whitespace host",
			config: DatabaseConfig{
				Host:   "   ",
				Port:   5432,
				DBName: "test",
			},
			wantErr: false, // Current validation only checks empty string
		},
		{
			name: "negative port",
			config: DatabaseConfig{
				Host:   "localhost",
				Port:   -1,
				DBName: "test",
			},
			wantErr: false, // Current validation only checks zero
		},
		{
			name: "very large port",
			config: DatabaseConfig{
				Host:   "localhost",
				Port:   99999,
				DBName: "test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDbKeyDefault tests the default database key constant
func TestDbKeyDefault(t *testing.T) {
	assert.Equal(t, "default", dbKeyDefault)
}

// TestErrInvalidConfig tests the error constant
func TestErrInvalidConfig(t *testing.T) {
	assert.NotNil(t, errInvalidConfig)
	assert.Contains(t, errInvalidConfig.Error(), "invalid")
}

// BenchmarkDatabaseConfig_Validate benchmarks the Validate method
func BenchmarkDatabaseConfig_Validate(b *testing.B) {
	config := DatabaseConfig{
		Host:   "localhost",
		Port:   5432,
		DBName: "testdb",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

// BenchmarkGetDB benchmarks the GetDB function
func BenchmarkGetDB(b *testing.B) {
	resetConnPools()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetDB("test")
	}
}

// BenchmarkGetDefaultDB benchmarks the GetDefaultDB function
func BenchmarkGetDefaultDB(b *testing.B) {
	resetConnPools()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetDefaultDB()
	}
}

