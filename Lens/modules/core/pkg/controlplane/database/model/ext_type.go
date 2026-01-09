package model

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
