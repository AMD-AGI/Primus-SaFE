package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"unsafe"
)

// ExtType is for JSONB fields that store JSON objects (map[string]interface{})
// Used when database column default is '{}' or similar object structure
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

// ExtJSON is for JSONB fields that can store any JSON value (object, array, string, number, etc.)
// Used when database column default is '[]' indicating a JSON array
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

// MarshalJSON returns the raw JSON bytes (prevents Base64 encoding)
func (e ExtJSON) MarshalJSON() ([]byte, error) {
	if len(e) == 0 {
		return []byte("null"), nil
	}
	return []byte(e), nil
}

// UnmarshalJSON sets the raw JSON bytes
func (e *ExtJSON) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("ExtJSON: UnmarshalJSON on nil pointer")
	}
	*e = append((*e)[0:0], data...)
	return nil
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
