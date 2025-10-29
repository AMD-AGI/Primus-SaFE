/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var (
	targetDir = flag.String("targetDir", "", "The directory to store the generated files")
	dbName    = flag.String("dbName", "", "The name of the database")
	dbUser    = flag.String("dbUser", "", "The user of the database")
	dbPass    = flag.String("dbPass", "", "The password of the database")
	dbHost    = flag.String("dbHost", "", "The host of the database")
	dbPort    = flag.String("dbPort", "5432", "The port of the database")
	sslMode   = flag.String("sslMode", "disable", "The ssl mode of the database")
)

func main() {
	flag.Parse()
	g := gen.NewGenerator(gen.Config{
		OutPath: *targetDir,
		//Mode:    gen.WithoutContext | gen.WithDefaultQuery | gen.WithQueryInterface, // generate mode
	})
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=%s", *dbHost, *dbPort, *dbUser, *dbName, *dbPass, *sslMode)
	db, err := gorm.Open(postgres.Dialector{
		Config: &postgres.Config{
			DSN: dsn,
		},
	}, &gorm.Config{
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
		panic(err)
	}
	g.UseDB(db)
	g.WithDataTypeMap(map[string]func(columnType gorm.ColumnType) (dataType string){
		"jsonb": func(columnType gorm.ColumnType) (dataType string) {
			return "ExtType"
		},
	})
	tables := g.GenerateAllTable()
	g.ApplyBasic(tables...)
	g.Execute()
	var outPath string
	if strings.Contains(g.ModelPkgPath, string(os.PathSeparator)) {
		outPath, err = filepath.Abs(g.ModelPkgPath)
		if err != nil {
			panic(err)
		}
	} else {
		outPath = filepath.Join(filepath.Dir(g.OutPath), g.ModelPkgPath)
	}
	// 写入custom type file
	customFilePath := fmt.Sprintf("%s/ext_type.go", outPath)
	err = os.WriteFile(customFilePath, []byte(customTypeFileContent), 0644)
	if err != nil {
		panic(err)
	}
}

const (
	customTypeFileContent = `package model

import (
    "database/sql/driver"
    "encoding/json"
    "errors"
    "unsafe"
)

type ExtType map[string]interface{}

func (e ExtType) Value() (driver.Value, error) {
    b, err := json.Marshal(e)
    return *(*string)(unsafe.Pointer(&b)), err
}

func (e *ExtType) Scan(value interface{}) error {
    if b, ok := value.([]byte); ok {
       return json.Unmarshal(b, &e)
    }
    return errors.New("type assertion to []byte failed")
}

func (e *ExtType) GetStringValue(key string) string {
    if val, ok := (*e)[key]; ok {
       if str, ok := val.(string); ok {
          return str
       }
    }
    return ""
}
`
)
