/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
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
)

var (
	once     sync.Once
	instance *Client
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
// Returns:
//   - *Client: Singleton database client instance
func NewClient() *Client {
	once.Do(func() {
		cfg := &utils.DBConfig{
			DBName:         commonconfig.GetDBName(),
			Username:       commonconfig.GetDBUser(),
			Password:       commonconfig.GetDBPassword(),
			Host:           commonconfig.GetDBHost(),
			Port:           commonconfig.GetDBPort(),
			SSLMode:        commonconfig.GetDBSslMode(),
			MaxOpenConns:   commonconfig.GetDBMaxOpenConns(),
			MaxIdleConns:   commonconfig.GetDBMaxIdleConns(),
			MaxLifetime:    time.Duration(commonconfig.GetDBMaxLifetimeSecond()) * time.Second,
			MaxIdleTime:    time.Duration(commonconfig.GetDBMaxIdleTimeSecond()) * time.Second,
			ConnectTimeout: commonconfig.GetDBConnectTimeoutSecond(),
			RequestTimeout: time.Duration(commonconfig.GetDBRequestTimeoutSecond()) * time.Second,
		}
		if err := checkParams(cfg); err != nil {
			klog.ErrorS(err, "failed to check db params")
			return
		}
		db, err := utils.Connect(cfg, utils.PgDriver)
		if err != nil {
			klog.Errorf("%s", err.Error())
			return
		}
		err = db.Ping()
		if err != nil {
			klog.ErrorS(err, "failed to ping db")
			return
		}
		gormDb, err := utils.ConnectGorm(cfg)
		instance = &Client{db: db, DBConfig: cfg, gorm: gormDb}
		klog.Infof("init db-client successfully! conn-timeout: %d(s), request-timeout: %d(s)",
			cfg.ConnectTimeout, commonconfig.GetDBRequestTimeoutSecond())
	})
	return instance
}

// Close performs the Close operation.
func (c *Client) Close() {
	err := c.db.Close()
	if err != nil {
		klog.ErrorS(err, "failed to close db connection")
	}
}

// getDB retrieves DB for internal use.
func (c *Client) getDB() (*sqlx.DB, error) {
	if c.db == nil {
		return nil, commonerrors.NewInternalError("The client of db has not been initialized")
	}
	return c.db.Unsafe(), nil
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
