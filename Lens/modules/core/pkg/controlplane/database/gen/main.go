// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var (
	targetDir = flag.String("targetDir", "../dal", "Target directory for generated files")
	dbHost    = flag.String("dbHost", "localhost", "Database host")
	dbPort    = flag.String("dbPort", "5432", "Database port")
	dbName    = flag.String("dbName", "primus-lens-control-plane", "Database name")
	dbUser    = flag.String("dbUser", "primus-lens-control-plane", "Database user")
	dbPass    = flag.String("dbPass", "", "Database password")
	sslMode   = flag.String("sslMode", "require", "SSL mode")
)

func main() {
	flag.Parse()

	if *dbPass == "" {
		fmt.Println("Error: -dbPass is required")
		os.Exit(1)
	}

	g := gen.NewGenerator(gen.Config{
		OutPath:      *targetDir,
		ModelPkgPath: "../model",
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
	})

	// Use PostgreSQL URI format to properly handle special characters in password
	encodedPass := url.QueryEscape(*dbPass)
	encodedUser := url.QueryEscape(*dbUser)
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		encodedUser, encodedPass, *dbHost, *dbPort, *dbName, *sslMode)

	db, err := gorm.Open(postgres.Dialector{
		Config: &postgres.Config{
			DSN: dsn,
		},
	}, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		Logger: logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	g.UseDB(db)

	// Map JSONB to appropriate types
	g.WithDataTypeMap(map[string]func(columnType gorm.ColumnType) (dataType string){
		"jsonb": func(columnType gorm.ColumnType) (dataType string) {
			// Check default value to determine if it's an array or object
			if def, ok := columnType.DefaultValue(); ok {
				if strings.Contains(def, "[]") || strings.Contains(def, "'[]'") {
					return "ExtJSON"
				}
			}
			return "ExtType"
		},
	})

	// Generate for Control Plane tables
	tables := g.GenerateAllTable()
	g.ApplyBasic(tables...)
	g.Execute()

	// Determine output path
	var outPath string
	if strings.Contains(g.ModelPkgPath, string(os.PathSeparator)) {
		outPath, err = filepath.Abs(g.ModelPkgPath)
		if err != nil {
			panic(err)
		}
	} else {
		outPath = filepath.Join(filepath.Dir(g.OutPath), g.ModelPkgPath)
	}

	// Write custom type file
	customFilePath := fmt.Sprintf("%s/ext_type.go", outPath)
	err = os.WriteFile(customFilePath, []byte(customTypeFileContent), 0644)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Code generation completed. Output: %s\n", *targetDir)
}

const customTypeFileContent = `package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"unsafe"
)

// ExtType is for JSONB fields that store JSON objects (map[string]interface{})
type ExtType map[string]interface{}

func (e ExtType) Value() (driver.Value, error) {
	b, err := json.Marshal(e)
	return *(*string)(unsafe.Pointer(&b)), err
}

func (e *ExtType) Scan(value interface{}) error {
	if value == nil {
		*e = make(ExtType)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, &e)
	case string:
		return json.Unmarshal([]byte(v), &e)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
}

func (e *ExtType) GetStringValue(key string) string {
	if val, ok := (*e)[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// ExtJSON is for JSONB fields that can store any JSON value
type ExtJSON json.RawMessage

func (e ExtJSON) Value() (driver.Value, error) {
	if len(e) == 0 {
		return "null", nil
	}
	return string(e), nil
}

func (e *ExtJSON) Scan(value interface{}) error {
	if value == nil {
		*e = ExtJSON("null")
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*e = ExtJSON(v)
		return nil
	case string:
		*e = ExtJSON(v)
		return nil
	default:
		return errors.New("type assertion to []byte or string failed for ExtJSON")
	}
}

// UnmarshalTo unmarshals the JSON into the provided destination
func (e ExtJSON) UnmarshalTo(dest interface{}) error {
	if len(e) == 0 {
		return nil
	}
	return json.Unmarshal(e, dest)
}

// MarshalFrom marshals the provided value into ExtJSON
func (e *ExtJSON) MarshalFrom(src interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	*e = ExtJSON(b)
	return nil
}

// IsArray returns true if the JSON value is an array
func (e ExtJSON) IsArray() bool {
	for _, c := range e {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c == '['
	}
	return false
}

// IsObject returns true if the JSON value is an object
func (e ExtJSON) IsObject() bool {
	for _, c := range e {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c == '{'
	}
	return false
}
`
