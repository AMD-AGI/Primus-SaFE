// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package installer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	_ "github.com/lib/pq"
)

// DefaultMigrationsPath is the default path where migrations are stored in the container
const DefaultMigrationsPath = "/app/migrations"

// Migrator handles database migrations
type Migrator struct {
	db             *sql.DB
	migrationsPath string
}

// NewMigrator creates a new Migrator instance
func NewMigrator(db *sql.DB, migrationsPath string) *Migrator {
	if migrationsPath == "" {
		migrationsPath = DefaultMigrationsPath
	}
	return &Migrator{
		db:             db,
		migrationsPath: migrationsPath,
	}
}

// ConnectAndMigrate connects to database and runs migrations
func ConnectAndMigrate(ctx context.Context, host string, port int, user, password, dbName, sslMode, migrationsPath string) error {
	if sslMode == "" {
		sslMode = "require"
	}

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbName, sslMode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	migrator := NewMigrator(db, migrationsPath)
	return migrator.Run(ctx)
}

// Run executes all pending migrations
func (m *Migrator) Run(ctx context.Context) error {
	// Check if migrations directory exists
	if _, err := os.Stat(m.migrationsPath); os.IsNotExist(err) {
		log.Warnf("Migrations directory '%s' does not exist, skipping migrations", m.migrationsPath)
		return nil
	}

	// Ensure schema_migrations table exists
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	// Get list of applied migrations
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Get list of available migrations
	available, err := m.getAvailableMigrations()
	if err != nil {
		return fmt.Errorf("failed to get available migrations: %w", err)
	}

	// Find pending migrations
	pending := m.findPendingMigrations(available, applied)
	if len(pending) == 0 {
		log.Info("No pending migrations")
		return nil
	}

	log.Infof("Found %d pending migrations", len(pending))

	// Execute pending migrations
	for _, migration := range pending {
		if err := m.executeMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to execute migration '%s': %w", migration, err)
		}
	}

	log.Infof("Successfully applied %d migrations", len(pending))
	return nil
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist
func (m *Migrator) ensureMigrationsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`
	_, err := m.db.ExecContext(ctx, query)
	return err
}

// getAppliedMigrations returns a map of applied migration versions
func (m *Migrator) getAppliedMigrations(ctx context.Context) (map[string]bool, error) {
	query := `SELECT version FROM schema_migrations`
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

// getAvailableMigrations returns a sorted list of available migration files
func (m *Migrator) getAvailableMigrations() ([]string, error) {
	var migrations []string

	entries, err := os.ReadDir(m.migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, entry.Name())
		}
	}

	// Sort migrations by name (they should be named with patch001, patch002, etc.)
	sort.Strings(migrations)
	return migrations, nil
}

// findPendingMigrations returns migrations that haven't been applied yet
func (m *Migrator) findPendingMigrations(available []string, applied map[string]bool) []string {
	var pending []string
	for _, migration := range available {
		// Use the filename without .sql extension as the version
		version := strings.TrimSuffix(migration, ".sql")
		if !applied[version] {
			pending = append(pending, migration)
		}
	}
	return pending
}

// executeMigration runs a single migration within a transaction
func (m *Migrator) executeMigration(ctx context.Context, filename string) error {
	log.Infof("Applying migration: %s", filename)

	// Read migration file
	content, err := os.ReadFile(filepath.Join(m.migrationsPath, filename))
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	// Start transaction
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration
	version := strings.TrimSuffix(filename, ".sql")
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	log.Infof("Successfully applied migration: %s", filename)
	return nil
}
