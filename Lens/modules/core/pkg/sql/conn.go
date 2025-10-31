package sql

import (
	"fmt"
	"sync"

	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
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
