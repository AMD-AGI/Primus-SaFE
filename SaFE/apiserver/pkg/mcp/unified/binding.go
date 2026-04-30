// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	tagQuery  = "query"
	tagParam  = "param"
	tagJSON   = "json"
	tagHeader = "header"
	tagMCP    = "mcp"
)

// BindGinRequest binds request parameters from gin.Context to a struct.
// Supports tags: query, param, json (body), header.
func BindGinRequest[Req any](c *gin.Context, req *Req) error {
	v := reflect.ValueOf(req)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("req must be a non-nil pointer")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("req must be a pointer to struct")
	}

	method := c.Request.Method
	if method == "POST" || method == "PUT" || method == "PATCH" {
		if c.ContentType() == "application/json" {
			if err := c.ShouldBindJSON(req); err != nil {
				if c.Request.ContentLength > 0 {
					return fmt.Errorf("failed to parse JSON body: %w", err)
				}
			}
		}
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			if err := bindStructFields(c, v, i); err != nil {
				return err
			}
			continue
		}

		if !fieldValue.CanSet() {
			continue
		}

		if queryTag := field.Tag.Get(tagQuery); queryTag != "" {
			queryTag = parseTagName(queryTag)
			if value := c.Query(queryTag); value != "" {
				if err := setFieldValue(fieldValue, value); err != nil {
					return fmt.Errorf("failed to set query param %s: %w", queryTag, err)
				}
			}
		}

		if paramTag := field.Tag.Get(tagParam); paramTag != "" {
			paramTag = parseTagName(paramTag)
			if value := c.Param(paramTag); value != "" {
				if err := setFieldValue(fieldValue, value); err != nil {
					return fmt.Errorf("failed to set path param %s: %w", paramTag, err)
				}
			}
		}

		if headerTag := field.Tag.Get(tagHeader); headerTag != "" {
			headerTag = parseTagName(headerTag)
			if value := c.GetHeader(headerTag); value != "" {
				if err := setFieldValue(fieldValue, value); err != nil {
					return fmt.Errorf("failed to set header %s: %w", headerTag, err)
				}
			}
		}
	}
	return nil
}

func bindStructFields(c *gin.Context, parent reflect.Value, embeddedIdx int) error {
	embedded := parent.Field(embeddedIdx)
	if embedded.Kind() != reflect.Struct {
		return nil
	}

	t := embedded.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := embedded.Field(i)
		if !fieldValue.CanSet() {
			continue
		}

		if queryTag := field.Tag.Get(tagQuery); queryTag != "" {
			queryTag = parseTagName(queryTag)
			if value := c.Query(queryTag); value != "" {
				if err := setFieldValue(fieldValue, value); err != nil {
					return err
				}
			}
		}

		if paramTag := field.Tag.Get(tagParam); paramTag != "" {
			paramTag = parseTagName(paramTag)
			if value := c.Param(paramTag); value != "" {
				if err := setFieldValue(fieldValue, value); err != nil {
					return err
				}
			}
		}

		if headerTag := field.Tag.Get(tagHeader); headerTag != "" {
			headerTag = parseTagName(headerTag)
			if value := c.GetHeader(headerTag); value != "" {
				if err := setFieldValue(fieldValue, value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// BindMCPRequest binds MCP request parameters (JSON) to a struct.
func BindMCPRequest[Req any](params json.RawMessage, req *Req) error {
	if len(params) == 0 {
		return nil
	}
	return json.Unmarshal(params, req)
}

func parseTagName(tag string) string {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx]
	}
	return tag
}

func setFieldValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(uintVal)
	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(floatVal)
	case reflect.Bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(boolVal)
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(value, ",")
			slice := reflect.MakeSlice(field.Type(), len(parts), len(parts))
			for i, part := range parts {
				slice.Index(i).SetString(strings.TrimSpace(part))
			}
			field.Set(slice)
		} else {
			return fmt.Errorf("unsupported slice element type: %s", field.Type().Elem().Kind())
		}
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setFieldValue(field.Elem(), value)
	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}
	return nil
}

// MCPTagOptions holds parsed mcp tag options.
type MCPTagOptions struct {
	Name        string
	Description string
	Required    bool
}

// ParseMCPTag parses the mcp struct tag.
// Format: mcp:"name,description=xxx,required"
func ParseMCPTag(tag string) MCPTagOptions {
	opts := MCPTagOptions{}
	parts := strings.Split(tag, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if i == 0 {
			opts.Name = part
			continue
		}
		if part == "required" {
			opts.Required = true
			continue
		}
		if strings.HasPrefix(part, "description=") {
			opts.Description = strings.TrimPrefix(part, "description=")
		}
	}
	return opts
}
