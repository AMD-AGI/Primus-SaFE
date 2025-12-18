package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/conf"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/sql"

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
	logConf := conf.DefaultConfig()
	logConf.Level = conf.TraceLevel
	log.InitGlobalLogger(logConf)
	// Use PostgreSQL URI format to properly handle special characters in password
	encodedPass := url.QueryEscape(*dbPass)
	encodedUser := url.QueryEscape(*dbUser)
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", encodedUser, encodedPass, *dbHost, *dbPort, *dbName, *sslMode)
	db, err := gorm.Open(postgres.Dialector{
		Config: &postgres.Config{
			DSN: dsn,
		},
	}, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		FullSaveAssociations:                     false,
		Logger:                                   sql.NullLogger{},
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
	// Write custom type file
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
`
)
