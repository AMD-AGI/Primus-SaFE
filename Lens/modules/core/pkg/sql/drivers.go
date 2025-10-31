package sql

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	DriverNamePostgres = "postgres"
	DriverNameMysql    = "mysql"
)

type dialectorFactoryFunc func(cfg DatabaseConfig) gorm.Dialector

var dialectors = map[string]dialectorFactoryFunc{
	DriverNamePostgres: initPostgres,
	DriverNameMysql:    initMysql,
}

func getDialector(cfg DatabaseConfig) gorm.Dialector {
	factoryFunc, ok := dialectors[cfg.Driver]
	if !ok {
		panic(fmt.Sprintf("Unknown driver name %s", cfg.Driver))
	}
	return factoryFunc(cfg)
}

func initPostgres(cfg DatabaseConfig) gorm.Dialector {
	dsn := fmt.Sprintf("host=%s port=%v user=%s dbname=%s password=%s", cfg.Host, cfg.Port, cfg.UserName, cfg.DBName, cfg.Password)
	if cfg.SSLMode != "" {
		dsn = fmt.Sprintf("%s  sslmode=%s", dsn, cfg.SSLMode)
	} else {
		dsn = fmt.Sprintf("%s  sslmode=require", dsn)
	}
	if cfg.TimeZone != "" {
		dsn = fmt.Sprintf("%s timezone=%s", dsn, cfg.TimeZone)
	}
	return postgres.Dialector{
		Config: &postgres.Config{
			DSN: dsn,
		},
	}
}

func initMysql(cfg DatabaseConfig) gorm.Dialector {
	dsn := fmt.Sprintf("%s:%s@(%s:%d)/%s?parseTime=true", cfg.UserName, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
	return mysql.Dialector{
		Config: &mysql.Config{
			DSN: dsn,
		},
	}
}
