/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"gorm.io/gorm"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/metrics"
)

var (
	// instanceMu guards instance. A mutex rather than sync.Once, because once is
	// spent whether the body succeeded or not: a boot that could not reach the
	// database left instance nil for the life of the process, with no second
	// attempt and no way for a caller to tell "not ready" from "never will be".
	instanceMu sync.Mutex
	instance   *Client
)

// Client represents a database client that manages both sqlx and gorm database connections.
// It encapsulates the database configuration and provides methods to interact with the database.
type Client struct {
	db              *sqlx.DB // sqlx database instance
	gorm            *gorm.DB // gorm ORM database instance
	*utils.DBConfig          // Embedded database configuration
}

// NewClient creates a singleton instance of the database Client.
// It initializes the database configuration from common configuration,
// validates the parameters, establishes connections using both sqlx and gorm
// The initialization happens only once even if called multiple times.
//
// A nil result means "no client right now", and callers decide what that costs
// them -- the audit middleware degrades to a passthrough, the email relay
// reports it. That contract is kept deliberately.
//
// What changed is that a failure no longer settles the question. Built under a
// mutex rather than sync.Once, because once is spent whether the body succeeded
// or not: a boot that could not reach the database used to leave the singleton
// nil for the life of the process, so a blip while the cluster was electing a
// primary cost a deployment its database client until someone restarted the pod.
// Now the next caller tries again, and a database that has finished failing over
// is picked up without one.
//
// Returns:
//   - *Client: Singleton database client instance, or nil when one cannot be
//     built right now.
func NewClient() *Client {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	if instance != nil {
		return instance
	}
	client, err := buildClient(configuredDB())
	if err != nil {
		klog.ErrorS(err, "failed to init db-client; a later call will try again")
		return nil
	}
	instance = client
	klog.Infof("init db-client successfully! conn-timeout: %d(s), request-timeout: %d(s)",
		client.ConnectTimeout, commonconfig.GetDBRequestTimeoutSecond())
	return instance
}

// configuredDB reads the database settings this process was started with.
//
// A variable so a test can count how often an init is attempted without standing
// up viper state for settings it is not testing.
var configuredDB = func() *utils.DBConfig {
	return &utils.DBConfig{
		DBName:             commonconfig.GetDBName(),
		Username:           commonconfig.GetDBUser(),
		Password:           commonconfig.GetDBPassword(),
		Host:               commonconfig.GetDBHost(),
		Port:               commonconfig.GetDBPort(),
		SSLMode:            commonconfig.GetDBSslMode(),
		TargetSessionAttrs: commonconfig.GetDBTargetSessionAttrs(),
		MaxOpenConns:       commonconfig.GetDBMaxOpenConns(),
		MaxIdleConns:       commonconfig.GetDBMaxIdleConns(),
		MaxLifetime:        time.Duration(commonconfig.GetDBMaxLifetimeSecond()) * time.Second,
		MaxIdleTime:        time.Duration(commonconfig.GetDBMaxIdleTimeSecond()) * time.Second,
		ConnectTimeout:     commonconfig.GetDBConnectTimeoutSecond(),
		RequestTimeout:     time.Duration(commonconfig.GetDBRequestTimeoutSecond()) * time.Second,
	}
}

// buildClient opens both pools for cfg, or returns why it could not.
//
// Separate from NewClient so the failure paths can be exercised without a
// database and without a singleton to reset between tests. Anything opened
// before the failure is closed and stops reporting, so a retry does not leave a
// pool behind each time it is attempted.
func buildClient(cfg *utils.DBConfig) (*Client, error) {
	if err := checkParams(cfg); err != nil {
		return nil, fmt.Errorf("check db params: %w", err)
	}
	db, err := utils.Connect(cfg, utils.PgDriver)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	if err = db.Ping(); err != nil {
		discardPool(db, cfg)
		return nil, fmt.Errorf("ping db: %w", err)
	}
	gormDb, err := utils.ConnectGorm(cfg)
	if err != nil {
		discardPool(db, cfg)
		return nil, fmt.Errorf("connect gorm db: %w", err)
	}
	return &Client{db: db, DBConfig: cfg, gorm: gormDb}, nil
}

// discardPool closes a pool that will not be used and stops it reporting.
//
// Connect registers a metrics collector, and a closed pool that is still
// registered publishes a frozen set of zeroes for the life of the process --
// indistinguishable on a dashboard from a pool that is simply idle. Client.Close
// pairs these for the same reason.
func discardPool(db *sqlx.DB, cfg *utils.DBConfig) {
	if err := db.Close(); err != nil {
		klog.ErrorS(err, "failed to close db connection")
	}
	metrics.UnregisterDBPool(utils.SqlxPoolKey(cfg))
}

// NewClientWithConfig creates a new database Client instance with the provided configuration.
// Unlike NewClient, this method creates a new instance each time it's called (non-singleton).
// It validates the parameters, establishes connections using both sqlx and gorm.
//
// Parameters:
//   - cfg: Database configuration
//
// Returns:
//   - *Client: New database client instance
//   - error: Error if initialization fails
func NewClientWithConfig(cfg *utils.DBConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("database config cannot be nil")
	}

	// Same build as the singleton takes, so the two cannot come to differ in
	// which pools they open or what they clean up when one of them fails.
	client, err := buildClient(cfg)
	if err != nil {
		klog.ErrorS(err, "failed to create db-client")
		return nil, err
	}
	klog.Infof("created new db-client successfully! host: %s, dbname: %s, conn-timeout: %d(s), request-timeout: %d(s)",
		cfg.Host, cfg.DBName, cfg.ConnectTimeout, int(cfg.RequestTimeout.Seconds()))

	return client, nil
}

// Ping verifies the database connection is alive. It is used by the self-health
// reporter to publish the "database" subsystem status. Returns an error if the
// client (or its underlying connection) is not initialized or unreachable.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.db == nil {
		return commonerrors.NewInternalError("The client of db has not been initialized")
	}
	return c.db.PingContext(ctx)
}

// Close performs the Close operation.
func (c *Client) Close() {
	err := c.db.Close()
	if err != nil {
		klog.ErrorS(err, "failed to close db connection")
	}
	// Both pools belong to this client, so neither should keep reporting once
	// the client is discarded.
	metrics.UnregisterDBPool(utils.SqlxPoolKey(c.DBConfig))
	metrics.UnregisterDBPool(utils.GormPoolKey(c.DBConfig))
}

// getDB retrieves DB for internal use. The handle is wrapped so that every
// query issued through it is reported to Prometheus.
func (c *Client) getDB() (*instrumentedDB, error) {
	if c.db == nil {
		return nil, commonerrors.NewInternalError("The client of db has not been initialized")
	}
	return &instrumentedDB{DB: c.db.Unsafe()}, nil
}

// GetGormDB retrieves the GORM DB instance for external use.
// Returns:
//   - *gorm.DB: GORM database instance
//   - error: Error if the client has not been initialized
func (c *Client) GetGormDB() (*gorm.DB, error) {
	if c.gorm == nil {
		return nil, commonerrors.NewInternalError("The GORM client has not been initialized")
	}
	return c.gorm, nil
}

// checkParams checks Params and returns the result.
func checkParams(cfg *utils.DBConfig) error {
	var errs []error
	if cfg.DBName == "" {
		errs = append(errs, fmt.Errorf("dbname not found"))
	}
	if cfg.Username == "" {
		errs = append(errs, fmt.Errorf("username not found"))
	}
	if cfg.Password == "" {
		errs = append(errs, fmt.Errorf("password not found"))
	}
	if cfg.Host == "" {
		errs = append(errs, fmt.Errorf("host not found"))
	}
	if cfg.SSLMode == "" {
		errs = append(errs, fmt.Errorf("ssl_mode not found"))
	}
	if cfg.Port == 0 {
		errs = append(errs, fmt.Errorf("port not found"))
	}
	return utilerrors.NewAggregate(errs)
}
