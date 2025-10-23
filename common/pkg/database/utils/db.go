/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

type DBDriver string

const (
	PgDriver DBDriver = "postgres"
)

func Connect(cfg *DBConfig, driverName DBDriver) (*sqlx.DB, error) {
	dataSource := cfg.SourceName()
	db, err := sqlx.Connect(string(driverName), dataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to connect db %s, err: %v", cfg.DBName, err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	db.SetConnMaxIdleTime(cfg.MaxIdleTime)
	db.SetConnMaxLifetime(cfg.MaxLifetime)
	return db, nil
}

func ConnectGorm(cfg *DBConfig) (*gorm.DB, error) {
	// init gorm
	dsn := fmt.Sprintf("host=%s port=%v user=%s dbname=%s password=%s sslmode=%s", cfg.Host, cfg.Port, cfg.Username, cfg.DBName, cfg.Password, cfg.SSLMode)
	dialector := postgres.Dialector{
		Config: &postgres.Config{
			DSN: dsn,
		},
	}
	gormDB, err := gorm.Open(dialector, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		FullSaveAssociations:                     false,
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
	return gormDB, nil
}

func ParseNullString(str sql.NullString) string {
	if str.Valid {
		return str.String
	}
	return ""
}

func ParseNullTimeToString(t pq.NullTime) string {
	if t.Valid && !t.Time.IsZero() {
		return timeutil.FormatRFC3339(&t.Time)
	}
	return ""
}

func ParseNullTime(t pq.NullTime) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

func NullString(str string) sql.NullString {
	if str == "" {
		return sql.NullString{
			Valid: false,
		}
	}
	return sql.NullString{
		String: str,
		Valid:  true,
	}
}

func NullTime(t time.Time) pq.NullTime {
	if t.IsZero() {
		return pq.NullTime{
			Valid: false,
		}
	}
	return pq.NullTime{
		Time:  t,
		Valid: true,
	}
}

func NullMetaV1Time(t *metav1.Time) pq.NullTime {
	if t.IsZero() {
		return pq.NullTime{
			Valid: false,
		}
	}
	return pq.NullTime{
		Time:  t.Time,
		Valid: true,
	}
}

func CvtToSqlStr(sql sqrl.Sqlizer) string {
	sqlStr, args, err := sql.ToSql()
	if err != nil {
		klog.Errorf("failed to convert sql, err: %v", err)
		return ""
	}
	return sqlStr + " " + string(jsonutils.MarshalSilently(args))
}
