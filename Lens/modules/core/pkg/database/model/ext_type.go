package model

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
       *e = make(map[string]interface{})
       return nil
    }
    
    var b []byte
    switch v := value.(type) {
    case []byte:
       b = v
    case string:
       b = []byte(v)
    default:
       return errors.New("type assertion to []byte failed")
    }
    
    if len(b) == 0 {
       *e = make(map[string]interface{})
       return nil
    }
    
    return json.Unmarshal(b, &e)
}

func (e *ExtType) GetStringValue(key string) string {
    if val, ok := (*e)[key]; ok {
       if str, ok := val.(string); ok {
          return str
       }
    }
    return ""
}
