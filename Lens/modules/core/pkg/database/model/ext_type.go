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
