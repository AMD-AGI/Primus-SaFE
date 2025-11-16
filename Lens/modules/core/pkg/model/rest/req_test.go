package rest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPage tests the Page struct
func TestPage(t *testing.T) {
	page := Page{
		PageNum:  1,
		PageSize: 10,
	}

	assert.Equal(t, 1, page.PageNum)
	assert.Equal(t, 10, page.PageSize)
}

// TestPage_DefaultValues tests Page with default values
func TestPage_DefaultValues(t *testing.T) {
	page := Page{}

	assert.Equal(t, 0, page.PageNum)
	assert.Equal(t, 0, page.PageSize)
}

// TestPage_CustomValues tests Page with custom values
func TestPage_CustomValues(t *testing.T) {
	tests := []struct {
		name     string
		pageNum  int
		pageSize int
	}{
		{"first page", 1, 10},
		{"second page", 2, 20},
		{"large page", 100, 100},
		{"single item", 1, 1},
		{"zero page", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := Page{
				PageNum:  tt.pageNum,
				PageSize: tt.pageSize,
			}

			assert.Equal(t, tt.pageNum, page.PageNum)
			assert.Equal(t, tt.pageSize, page.PageSize)
		})
	}
}

// TestPage_JSONMarshal tests JSON marshaling of Page
func TestPage_JSONMarshal(t *testing.T) {
	page := Page{
		PageNum:  5,
		PageSize: 25,
	}

	data, err := json.Marshal(page)
	require.NoError(t, err)

	var decoded Page
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, page.PageNum, decoded.PageNum)
	assert.Equal(t, page.PageSize, decoded.PageSize)
}

// TestPage_JSONUnmarshal tests JSON unmarshaling into Page
func TestPage_JSONUnmarshal(t *testing.T) {
	jsonStr := `{"page_num":3,"page_size":15}`

	var page Page
	err := json.Unmarshal([]byte(jsonStr), &page)
	require.NoError(t, err)

	assert.Equal(t, 3, page.PageNum)
	assert.Equal(t, 15, page.PageSize)
}

// TestPage_JSONUnmarshal_PartialData tests unmarshaling with partial data
func TestPage_JSONUnmarshal_PartialData(t *testing.T) {
	jsonStr := `{"page_num":2}`

	var page Page
	err := json.Unmarshal([]byte(jsonStr), &page)
	require.NoError(t, err)

	assert.Equal(t, 2, page.PageNum)
	assert.Equal(t, 0, page.PageSize) // Default value
}

// TestPage_JSONUnmarshal_EmptyObject tests unmarshaling empty JSON object
func TestPage_JSONUnmarshal_EmptyObject(t *testing.T) {
	jsonStr := `{}`

	var page Page
	err := json.Unmarshal([]byte(jsonStr), &page)
	require.NoError(t, err)

	assert.Equal(t, 0, page.PageNum)
	assert.Equal(t, 0, page.PageSize)
}

// TestPage_CalculateOffset tests offset calculation
func TestPage_CalculateOffset(t *testing.T) {
	tests := []struct {
		name           string
		pageNum        int
		pageSize       int
		expectedOffset int
	}{
		{"first page", 1, 10, 0},
		{"second page", 2, 10, 10},
		{"third page", 3, 10, 20},
		{"large page size", 5, 100, 400},
		{"zero page", 0, 10, -10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := Page{
				PageNum:  tt.pageNum,
				PageSize: tt.pageSize,
			}

			offset := (page.PageNum - 1) * page.PageSize
			assert.Equal(t, tt.expectedOffset, offset)
		})
	}
}

// TestPage_NegativeValues tests Page with negative values
func TestPage_NegativeValues(t *testing.T) {
	page := Page{
		PageNum:  -1,
		PageSize: -10,
	}

	assert.Equal(t, -1, page.PageNum)
	assert.Equal(t, -10, page.PageSize)
}

// TestPage_LargeValues tests Page with very large values
func TestPage_LargeValues(t *testing.T) {
	page := Page{
		PageNum:  1000000,
		PageSize: 10000,
	}

	assert.Equal(t, 1000000, page.PageNum)
	assert.Equal(t, 10000, page.PageSize)
}

// BenchmarkPage_Creation benchmarks Page creation
func BenchmarkPage_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Page{
			PageNum:  1,
			PageSize: 10,
		}
	}
}

// BenchmarkPage_JSONMarshal benchmarks JSON marshaling
func BenchmarkPage_JSONMarshal(b *testing.B) {
	page := Page{
		PageNum:  1,
		PageSize: 10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(page)
	}
}

// BenchmarkPage_JSONUnmarshal benchmarks JSON unmarshaling
func BenchmarkPage_JSONUnmarshal(b *testing.B) {
	jsonStr := []byte(`{"page_num":1,"page_size":10}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var page Page
		_ = json.Unmarshal(jsonStr, &page)
	}
}

