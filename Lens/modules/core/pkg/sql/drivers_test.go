package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
)

// TestDriverConstants tests driver name constants
func TestDriverConstants(t *testing.T) {
	assert.Equal(t, "postgres", DriverNamePostgres)
	assert.Equal(t, "mysql", DriverNameMysql)
}

// TestGetDialector_Postgres tests getDialector for Postgres
func TestGetDialector_Postgres(t *testing.T) {
	config := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		UserName: "user",
		Password: "pass",
		DBName:   "testdb",
		Driver:   DriverNamePostgres,
	}

	dialector := getDialector(config)
	require.NotNil(t, dialector)

	// Verify it's a Postgres dialector
	_, ok := dialector.(postgres.Dialector)
	assert.True(t, ok, "Should return a Postgres dialector")
}

// TestGetDialector_Mysql tests getDialector for MySQL
func TestGetDialector_Mysql(t *testing.T) {
	config := DatabaseConfig{
		Host:     "localhost",
		Port:     3306,
		UserName: "root",
		Password: "password",
		DBName:   "testdb",
		Driver:   DriverNameMysql,
	}

	dialector := getDialector(config)
	require.NotNil(t, dialector)

	// Verify it's a MySQL dialector
	_, ok := dialector.(mysql.Dialector)
	assert.True(t, ok, "Should return a MySQL dialector")
}

// TestGetDialector_UnknownDriver tests panic for unknown driver
func TestGetDialector_UnknownDriver(t *testing.T) {
	config := DatabaseConfig{
		Host:   "localhost",
		Port:   5432,
		DBName: "testdb",
		Driver: "unknown_driver",
	}

	assert.Panics(t, func() {
		getDialector(config)
	}, "Should panic for unknown driver")
}

// TestInitPostgres tests initPostgres function
func TestInitPostgres(t *testing.T) {
	tests := []struct {
		name         string
		config       DatabaseConfig
		expectedDSN  []string // Substrings that should be in DSN
	}{
		{
			name: "basic config",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				UserName: "user",
				Password: "pass",
				DBName:   "testdb",
			},
			expectedDSN: []string{
				"host=localhost",
				"port=5432",
				"user=user",
				"dbname=testdb",
				"password=pass",
				"sslmode=require", // Default
			},
		},
		{
			name: "with custom SSL mode",
			config: DatabaseConfig{
				Host:     "db.example.com",
				Port:     5432,
				UserName: "admin",
				Password: "secret",
				DBName:   "production",
				SSLMode:  "disable",
			},
			expectedDSN: []string{
				"host=db.example.com",
				"port=5432",
				"user=admin",
				"dbname=production",
				"password=secret",
				"sslmode=disable",
			},
		},
		{
			name: "with timezone",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				UserName: "user",
				Password: "pass",
				DBName:   "testdb",
				TimeZone: "UTC",
			},
			expectedDSN: []string{
				"host=localhost",
				"timezone=UTC",
			},
		},
		{
			name: "all options",
			config: DatabaseConfig{
				Host:     "db.example.com",
				Port:     5432,
				UserName: "admin",
				Password: "secret",
				DBName:   "production",
				SSLMode:  "verify-full",
				TimeZone: "America/New_York",
			},
			expectedDSN: []string{
				"host=db.example.com",
				"port=5432",
				"user=admin",
				"dbname=production",
				"password=secret",
				"sslmode=verify-full",
				"timezone=America/New_York",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialector := initPostgres(tt.config)
			require.NotNil(t, dialector)

			pgDialector, ok := dialector.(postgres.Dialector)
			require.True(t, ok)
			require.NotNil(t, pgDialector.Config)

			dsn := pgDialector.Config.DSN
			for _, expected := range tt.expectedDSN {
				assert.Contains(t, dsn, expected, "DSN should contain '%s'", expected)
			}
		})
	}
}

// TestInitMysql tests initMysql function
func TestInitMysql(t *testing.T) {
	tests := []struct {
		name        string
		config      DatabaseConfig
		expectedDSN []string // Substrings that should be in DSN
	}{
		{
			name: "basic config",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     3306,
				UserName: "root",
				Password: "password",
				DBName:   "testdb",
			},
			expectedDSN: []string{
				"root:password",
				"@(localhost:3306)",
				"/testdb",
				"parseTime=true",
			},
		},
		{
			name: "with different host and port",
			config: DatabaseConfig{
				Host:     "mysql.example.com",
				Port:     3307,
				UserName: "admin",
				Password: "secret",
				DBName:   "production",
			},
			expectedDSN: []string{
				"admin:secret",
				"@(mysql.example.com:3307)",
				"/production",
				"parseTime=true",
			},
		},
		{
			name: "empty password",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     3306,
				UserName: "user",
				Password: "",
				DBName:   "testdb",
			},
			expectedDSN: []string{
				"user:",
				"@(localhost:3306)",
				"/testdb",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialector := initMysql(tt.config)
			require.NotNil(t, dialector)

			mysqlDialector, ok := dialector.(mysql.Dialector)
			require.True(t, ok)
			require.NotNil(t, mysqlDialector.Config)

			dsn := mysqlDialector.Config.DSN
			for _, expected := range tt.expectedDSN {
				assert.Contains(t, dsn, expected, "DSN should contain '%s'", expected)
			}
		})
	}
}

// TestDialectorFactory tests the dialectorFactoryFunc type
func TestDialectorFactory(t *testing.T) {
	// Verify factory functions are registered
	assert.Contains(t, dialectors, DriverNamePostgres)
	assert.Contains(t, dialectors, DriverNameMysql)
	assert.Len(t, dialectors, 2)
}

// TestPostgres_SSLMode tests various SSL modes for Postgres
func TestPostgres_SSLMode(t *testing.T) {
	sslModes := []string{"disable", "require", "verify-ca", "verify-full"}

	for _, sslMode := range sslModes {
		t.Run("ssl_mode_"+sslMode, func(t *testing.T) {
			config := DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				UserName: "user",
				Password: "pass",
				DBName:   "testdb",
				SSLMode:  sslMode,
			}

			dialector := initPostgres(config)
			pgDialector := dialector.(postgres.Dialector)
			dsn := pgDialector.Config.DSN

			assert.Contains(t, dsn, "sslmode="+sslMode)
		})
	}
}

// TestMysql_SpecialCharacters tests MySQL DSN with special characters
func TestMysql_SpecialCharacters(t *testing.T) {
	config := DatabaseConfig{
		Host:     "localhost",
		Port:     3306,
		UserName: "user@host",
		Password: "p@ss:word",
		DBName:   "test-db",
	}

	dialector := initMysql(config)
	mysqlDialector := dialector.(mysql.Dialector)
	dsn := mysqlDialector.Config.DSN

	// DSN should contain the special characters (not URL encoded in this implementation)
	assert.Contains(t, dsn, "user@host")
	assert.Contains(t, dsn, "p@ss:word")
	assert.Contains(t, dsn, "test-db")
}

// BenchmarkGetDialector_Postgres benchmarks Postgres dialector creation
func BenchmarkGetDialector_Postgres(b *testing.B) {
	config := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		UserName: "user",
		Password: "pass",
		DBName:   "testdb",
		Driver:   DriverNamePostgres,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getDialector(config)
	}
}

// BenchmarkGetDialector_Mysql benchmarks MySQL dialector creation
func BenchmarkGetDialector_Mysql(b *testing.B) {
	config := DatabaseConfig{
		Host:     "localhost",
		Port:     3306,
		UserName: "root",
		Password: "password",
		DBName:   "testdb",
		Driver:   DriverNameMysql,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getDialector(config)
	}
}

// BenchmarkInitPostgres benchmarks initPostgres function
func BenchmarkInitPostgres(b *testing.B) {
	config := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		UserName: "user",
		Password: "pass",
		DBName:   "testdb",
		SSLMode:  "require",
		TimeZone: "UTC",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = initPostgres(config)
	}
}

// BenchmarkInitMysql benchmarks initMysql function
func BenchmarkInitMysql(b *testing.B) {
	config := DatabaseConfig{
		Host:     "localhost",
		Port:     3306,
		UserName: "root",
		Password: "password",
		DBName:   "testdb",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = initMysql(config)
	}
}

