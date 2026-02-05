// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package sql

import (
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	dbKeyDefault = "default"
)

var (
	connPools    = map[string]*gorm.DB{}
	connPoolLock = &sync.RWMutex{}
)

var (
	errInvalidConfig = fmt.Errorf("config invalid")
)

type MultiDatabaseConfig map[string]DatabaseConfig

type DatabaseConfig struct {
	Host        string `json:"host" yaml:"host"`
	Port        int    `json:"port" yaml:"port"`
	UserName    string `json:"user_name" yaml:"user_name"`
	Password    string `json:"password" yaml:"password"`
	DBName      string `json:"db_name" yaml:"db_name"`
	LogMode     bool   `json:"log_mode" yaml:"log_mode"`
	MaxIdleConn int    `json:"max_idle_conn" yaml:"max_idle_conn"`
	MaxOpenConn int    `json:"max_open_conn" yaml:"max_open_conn"`
	SSLMode     string `json:"ssl_mode" yaml:"ssl_mode"`
	Driver      string `json:"driver" yaml:"driver"`
	TimeZone    string `json:"time_zone" yaml:"time_zone"`
}

func (d DatabaseConfig) Validate() error {
	if d.Host == "" || d.Port == 0 || d.DBName == "" {
		return errInvalidConfig
	}
	return nil
}

type opts func(db *gorm.DB)

func InitMulti(conf MultiDatabaseConfig, opts ...opts) error {
	for key, c := range conf {
		log.GlobalLogger().Debugf("Init database %s", key)
		if _, err := InitGormDB(key, c, opts...); err != nil {
			return err
		}
	}
	return nil
}

func InitDefault(conf DatabaseConfig, opts ...opts) (*gorm.DB, error) {
	return InitGormDB("default", conf, opts...)
}

func InitGormDB(key string, conf DatabaseConfig, opts ...opts) (*gorm.DB, error) {
	if gormDB := GetDB(key); gormDB != nil {
		return gormDB, nil
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	// First confirm default settings
	if conf.Driver == "" {
		conf.Driver = DriverNamePostgres
	}
	dialector := getDialector(conf)
	gormDB, err := gorm.Open(dialector, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		FullSaveAssociations:                     false,
		Logger:                                   NullLogger{},
		PrepareStmt:                              false,
		DisableAutomaticPing:                     false,
		DisableForeignKeyConstraintWhenMigrating: false,
		DisableNestedTransaction:                 false,
		AllowGlobalUpdate:                        false,
		QueryFields:                              false,
		Plugins:                                  nil,
	})
	if err != nil {
		return nil, err
	}

	// Configure connection pool parameters to ensure connections are periodically refreshed
	// This prevents connecting to old nodes after master-slave failover
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// Set connection pool parameters
	if conf.MaxIdleConn > 0 {
		sqlDB.SetMaxIdleConns(conf.MaxIdleConn)
	} else {
		sqlDB.SetMaxIdleConns(10) // Default max idle connections
	}

	if conf.MaxOpenConn > 0 {
		sqlDB.SetMaxOpenConns(conf.MaxOpenConn)
	} else {
		sqlDB.SetMaxOpenConns(40) // Default max open connections
	}

	// Set maximum connection lifetime: force close and re-establish connections after 5 minutes
	// This ensures old connections are replaced within 5 minutes after master-slave failover
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// Set maximum idle connection lifetime: close idle connections after 2 minutes
	// This helps to quickly clean up idle connections pointing to old nodes
	sqlDB.SetConnMaxIdleTime(2 * time.Minute)

	log.Infof("Configured connection pool for '%s': MaxIdleConn=%d, MaxOpenConn=%d, ConnMaxLifetime=5m, ConnMaxIdleTime=2m",
		key, conf.MaxIdleConn, conf.MaxOpenConn)

	for _, opt := range opts {
		opt(gormDB)
	}
	connPoolLock.Lock()
	defer connPoolLock.Unlock()
	connPools[key] = gormDB
	return gormDB, nil
}

func GetDB(key string) *gorm.DB {
	connPoolLock.RLock()
	defer connPoolLock.RUnlock()

	if db, ok := connPools[key]; ok {
		return db
	}
	return nil
}

func GetDefaultDB() *gorm.DB {
	return GetDB(dbKeyDefault)
}
