// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"reflect"
	"strings"
)

// JSONSchema represents a JSON Schema definition.
type JSONSchema struct {
	Type        string                `json:"type"`
	Description string                `json:"description,omitempty"`
	Properties  map[string]JSONSchema `json:"properties,omitempty"`
	Required    []string              `json:"required,omitempty"`
	Items       *JSONSchema           `json:"items,omitempty"`
	Enum        []any                 `json:"enum,omitempty"`
	Default     any                   `json:"default,omitempty"`
}

// GenerateJSONSchema generates a JSON Schema from a struct type.
// It uses struct tags to determine field names and descriptions:
//   - json:"name" for the property name
//   - mcp:"name,description=xxx,required" for MCP-specific metadata
func GenerateJSONSchema[Req any]() map[string]any {
	var req Req
	t := reflect.TypeOf(req)

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return map[string]any{
			"type": "object",
		}
	}

	schema := generateSchemaFromType(t)
	return schemaToMap(schema)
}

// generateSchemaFromType generates a JSONSchema from a reflect.Type.
func generateSchemaFromType(t reflect.Type) JSONSchema {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := JSONSchema{
		Type:       "object",
		Properties: make(map[string]JSONSchema),
		Required:   []string{},
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Handle embedded structs
		if field.Anonymous {
			embeddedSchema := generateSchemaFromType(field.Type)
			for name, prop := range embeddedSchema.Properties {
				schema.Properties[name] = prop
			}
			schema.Required = append(schema.Required, embeddedSchema.Required...)
			continue
		}

		// Get field name from json tag or use field name
		fieldName := getFieldName(field)
		if fieldName == "-" {
			continue
		}

		// Generate schema for field
		fieldSchema := generateFieldSchema(field)

		// Parse mcp tag for description and required
		if mcpTag := field.Tag.Get(tagMCP); mcpTag != "" {
			mcpOpts := ParseMCPTag(mcpTag)
			if mcpOpts.Description != "" {
				fieldSchema.Description = mcpOpts.Description
			}
			if mcpOpts.Required {
				schema.Required = append(schema.Required, fieldName)
			}
			// Use mcp name if specified
			if mcpOpts.Name != "" {
				fieldName = mcpOpts.Name
			}
		}

		schema.Properties[fieldName] = fieldSchema
	}

	return schema
}

// generateFieldSchema generates a JSONSchema for a struct field.
func generateFieldSchema(field reflect.StructField) JSONSchema {
	return typeToSchema(field.Type)
}

// typeToSchema converts a reflect.Type to a JSONSchema.
func typeToSchema(t reflect.Type) JSONSchema {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		return typeToSchema(t.Elem())
	}

	switch t.Kind() {
	case reflect.String:
		return JSONSchema{Type: "string"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return JSONSchema{Type: "integer"}

	case reflect.Float32, reflect.Float64:
		return JSONSchema{Type: "number"}

	case reflect.Bool:
		return JSONSchema{Type: "boolean"}

	case reflect.Slice, reflect.Array:
		itemSchema := typeToSchema(t.Elem())
		return JSONSchema{
			Type:  "array",
			Items: &itemSchema,
		}

	case reflect.Map:
		// Maps are represented as objects with additionalProperties
		return JSONSchema{Type: "object"}

	case reflect.Struct:
		// Handle time.Time specially
		if t.String() == "time.Time" {
			return JSONSchema{Type: "string", Description: "ISO 8601 datetime"}
		}
		return generateSchemaFromType(t)

	case reflect.Interface:
		return JSONSchema{Type: "object"}

	default:
		return JSONSchema{Type: "string"}
	}
}

// getFieldName returns the JSON field name from struct tags.
func getFieldName(field reflect.StructField) string {
	// Try json tag first
	if jsonTag := field.Tag.Get("json"); jsonTag != "" {
		name := parseTagName(jsonTag)
		if name != "" {
			return name
		}
	}

	// Try query tag
	if queryTag := field.Tag.Get("query"); queryTag != "" {
		name := parseTagName(queryTag)
		if name != "" {
			return name
		}
	}

	// Try param tag
	if paramTag := field.Tag.Get("param"); paramTag != "" {
		name := parseTagName(paramTag)
		if name != "" {
			return name
		}
	}

	// Fall back to field name in lowercase
	return strings.ToLower(field.Name)
}

// schemaToMap converts a JSONSchema to a map[string]any for JSON serialization.
func schemaToMap(schema JSONSchema) map[string]any {
	result := map[string]any{
		"type": schema.Type,
	}

	if schema.Description != "" {
		result["description"] = schema.Description
	}

	if len(schema.Properties) > 0 {
		props := make(map[string]any)
		for name, prop := range schema.Properties {
			props[name] = schemaToMap(prop)
		}
		result["properties"] = props
	}

	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}

	if schema.Items != nil {
		result["items"] = schemaToMap(*schema.Items)
	}

	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}

	if schema.Default != nil {
		result["default"] = schema.Default
	}

	return result
}
