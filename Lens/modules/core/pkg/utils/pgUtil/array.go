package pgUtil

import (
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// StringArrayToPgArray converts a Go []string to PostgreSQL TEXT[] format string
// Example: []string{"tag1", "tag2"} -> "{tag1,tag2}"
func StringArrayToPgArray(tags []string) string {
	if len(tags) == 0 {
		return "{}"
	}

	// Use pq.Array to properly format the array for PostgreSQL
	array := pq.Array(tags)

	// Get the driver.Value which is the properly formatted string
	val, err := array.Value()
	if err != nil {
		// Fallback to manual formatting if error occurs
		escapedTags := make([]string, len(tags))
		for i, tag := range tags {
			// Escape special characters for PostgreSQL array
			escaped := strings.ReplaceAll(tag, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
			// Quote if contains special characters
			if strings.ContainsAny(tag, "{},\" ") {
				escapedTags[i] = fmt.Sprintf("\"%s\"", escaped)
			} else {
				escapedTags[i] = tag
			}
		}
		return fmt.Sprintf("{%s}", strings.Join(escapedTags, ","))
	}

	// Return the formatted string
	if str, ok := val.(string); ok {
		return str
	}

	// Fallback
	return "{}"
}

// PgArrayToStringArray converts PostgreSQL TEXT[] format string to Go []string
// Example: "{tag1,tag2}" -> []string{"tag1", "tag2"}
func PgArrayToStringArray(pgArray string) []string {
	if pgArray == "" || pgArray == "{}" {
		return []string{}
	}

	// Use pq.Array to parse PostgreSQL array format
	var result []string
	array := pq.Array(&result)

	if err := array.Scan(pgArray); err != nil {
		// Fallback: simple parsing for basic arrays
		// Remove surrounding braces
		trimmed := strings.Trim(pgArray, "{}")
		if trimmed == "" {
			return []string{}
		}
		// Split by comma
		return strings.Split(trimmed, ",")
	}

	return result
}
