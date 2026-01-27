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

// Tag names for different binding sources
const (
	tagQuery  = "query"  // URL query parameter, e.g., ?cluster=prod
	tagParam  = "param"  // URL path parameter, e.g., /nodes/:name
	tagJSON   = "json"   // JSON field in request body
	tagHeader = "header" // HTTP header value
	tagMCP    = "mcp"    // MCP tool parameter definition
)

// BindGinRequest binds request parameters from gin.Context to a struct based on struct tags.
// It supports the following tags:
//   - query:"name"  -> c.Query("name")
//   - param:"name"  -> c.Param("name")
//   - json:"name"   -> from JSON body (for POST/PUT)
//   - header:"name" -> c.GetHeader("name")
func BindGinRequest[Req any](c *gin.Context, req *Req) error {
	v := reflect.ValueOf(req)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("req must be a non-nil pointer")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("req must be a pointer to struct")
	}

	// For POST/PUT/PATCH, try to bind JSON body first
	method := c.Request.Method
	if method == "POST" || method == "PUT" || method == "PATCH" {
		if c.ContentType() == "application/json" {
			// Try to bind JSON body, but don't fail if empty
			if err := c.ShouldBindJSON(req); err != nil {
				// Only return error if there was actual content that failed to parse
				if c.Request.ContentLength > 0 {
					return fmt.Errorf("failed to parse JSON body: %w", err)
				}
			}
		}
	}

	// Bind query, param, and header tags
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if !fieldValue.CanSet() {
			continue
		}

		// Handle embedded structs
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			embeddedReq := fieldValue.Addr().Interface()
			if err := bindStructFields(c, embeddedReq); err != nil {
				return err
			}
			continue
		}

		// Bind query parameter
		if queryTag := field.Tag.Get(tagQuery); queryTag != "" {
			queryTag = parseTagName(queryTag)
			if value := c.Query(queryTag); value != "" {
				if err := setFieldValue(fieldValue, value); err != nil {
					return fmt.Errorf("failed to set query param %s: %w", queryTag, err)
				}
			}
		}

		// Bind path parameter
		if paramTag := field.Tag.Get(tagParam); paramTag != "" {
			paramTag = parseTagName(paramTag)
			if value := c.Param(paramTag); value != "" {
				if err := setFieldValue(fieldValue, value); err != nil {
					return fmt.Errorf("failed to set path param %s: %w", paramTag, err)
				}
			}
		}

		// Bind header
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

// bindStructFields is a helper to bind fields for embedded structs
func bindStructFields(c *gin.Context, req any) error {
	v := reflect.ValueOf(req)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

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

// parseTagName extracts the field name from a struct tag, handling options like "name,omitempty".
func parseTagName(tag string) string {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx]
	}
	return tag
}

// setFieldValue sets the value of a reflect.Value from a string.
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
		// Handle comma-separated values for slices
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
		// Handle pointer types
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

// ParseMCPTag parses the mcp struct tag and returns options.
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
