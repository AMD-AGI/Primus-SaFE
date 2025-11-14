package regexUtil

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegexToStruct(t *testing.T) {
	t.Run("basic string field matching", func(t *testing.T) {
		type Person struct {
			Name string
			Age  string
		}

		re := regexp.MustCompile(`(?P<Name>\w+) is (?P<Age>\d+) years old`)
		input := "John is 30 years old"
		
		var result Person
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, "John", result.Name)
		assert.Equal(t, "30", result.Age)
	})

	t.Run("integer field matching", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		re := regexp.MustCompile(`(?P<Name>\w+) is (?P<Age>\d+) years old`)
		input := "John is 30 years old"
		
		var result Person
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, "John", result.Name)
		assert.Equal(t, 30, result.Age)
	})

	t.Run("float field matching", func(t *testing.T) {
		type Product struct {
			Name  string
			Price float64
		}

		re := regexp.MustCompile(`(?P<Name>\w+) costs (?P<Price>\d+\.\d+)`)
		input := "Apple costs 12.50"
		
		var result Product
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, "Apple", result.Name)
		assert.Equal(t, 12.50, result.Price)
	})

	t.Run("mixed type fields", func(t *testing.T) {
		type Record struct {
			ID     int
			Name   string
			Score  float64
			Status string
		}

		re := regexp.MustCompile(`ID:(?P<ID>\d+),Name:(?P<Name>\w+),Score:(?P<Score>\d+\.\d+),Status:(?P<Status>\w+)`)
		input := "ID:123,Name:John,Score:95.5,Status:active"
		
		var result Record
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, 123, result.ID)
		assert.Equal(t, "John", result.Name)
		assert.Equal(t, 95.5, result.Score)
		assert.Equal(t, "active", result.Status)
	})

	t.Run("partial field matching", func(t *testing.T) {
		type Person struct {
			Name    string
			Age     int
			Ignored string // not in regex
		}

		re := regexp.MustCompile(`(?P<Name>\w+) is (?P<Age>\d+) years old`)
		input := "John is 30 years old"
		
		var result Person
		result.Ignored = "original value"
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, "John", result.Name)
		assert.Equal(t, 30, result.Age)
		assert.Equal(t, "original value", result.Ignored) // should remain unchanged
	})

	t.Run("unnamed capture groups ignored", func(t *testing.T) {
		type Person struct {
			Name string
		}

		re := regexp.MustCompile(`(\w+) is (?P<Name>\w+)`)
		input := "Hello is John"
		
		var result Person
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, "John", result.Name)
	})

	t.Run("input does not match regex", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		re := regexp.MustCompile(`(?P<Name>\w+) is (?P<Age>\d+) years old`)
		input := "This does not match"
		
		var result Person
		err := RegexToStruct(re, input, &result)
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no match found")
	})

	t.Run("output parameter is not pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}

		re := regexp.MustCompile(`(?P<Name>\w+)`)
		input := "John"
		
		var result Person
		err := RegexToStruct(re, input, result) // passing value instead of pointer
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be a pointer to a struct")
	})

	t.Run("output parameter is not struct", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<Value>\w+)`)
		input := "test"
		
		var result string
		err := RegexToStruct(re, input, &result)
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be a pointer to a struct")
	})

	t.Run("invalid integer conversion", func(t *testing.T) {
		type Data struct {
			Value int
		}

		re := regexp.MustCompile(`(?P<Value>\w+)`)
		input := "notanumber"
		
		var result Data
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, 0, result.Value) // conversion fails, keeps default value
	})

	t.Run("invalid float conversion", func(t *testing.T) {
		type Data struct {
			Value float64
		}

		re := regexp.MustCompile(`(?P<Value>\w+)`)
		input := "notanumber"
		
		var result Data
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, 0.0, result.Value) // conversion fails, keeps default value
	})

	t.Run("empty string matching", func(t *testing.T) {
		type Data struct {
			Value string
		}

		re := regexp.MustCompile(`(?P<Value>.*)`)
		input := ""
		
		var result Data
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, "", result.Value)
	})

	t.Run("complex URL parsing", func(t *testing.T) {
		type URL struct {
			Protocol string
			Host     string
			Port     int
			Path     string
		}

		re := regexp.MustCompile(`(?P<Protocol>\w+)://(?P<Host>[^:]+):(?P<Port>\d+)(?P<Path>/.*)?`)
		input := "https://example.com:8080/path/to/resource"
		
		var result URL
		err := RegexToStruct(re, input, &result)
		
		assert.NoError(t, err)
		assert.Equal(t, "https", result.Protocol)
		assert.Equal(t, "example.com", result.Host)
		assert.Equal(t, 8080, result.Port)
		assert.Equal(t, "/path/to/resource", result.Path)
	})
}

