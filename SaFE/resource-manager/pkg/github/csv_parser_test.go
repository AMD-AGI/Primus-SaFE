/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCSVToRowsRegular(t *testing.T) {
	csv := "name,value\nfoo,1.5\nbar,2\n"
	rows, err := ParseCSVToRows([]byte(csv))
	assert.NoError(t, err)
	assert.Len(t, rows, 2)
	assert.Equal(t, "foo", rows[0]["name"])
	assert.Equal(t, 1.5, rows[0]["value"])
}

func TestParseCSVToRowsWide(t *testing.T) {
	csv := "#,Op,2026-02-05,2026-02-06\n1,Attention,255.36,260.12\n"
	rows, err := ParseCSVToRows([]byte(csv))
	assert.NoError(t, err)
	assert.Len(t, rows, 2)
	assert.Equal(t, "Attention", rows[0]["Op"])
	assert.Equal(t, "2026-02-05", rows[0]["date"])
	assert.Equal(t, 255.36, rows[0]["value"])
}

func TestParseCSVToRowsTooFewRows(t *testing.T) {
	rows, err := ParseCSVToRows([]byte("only-header\n"))
	assert.NoError(t, err)
	assert.Nil(t, rows)
}

func TestIsWideTable(t *testing.T) {
	assert.True(t, isWideTable([]string{"a", "2026-02-05"}))
	assert.False(t, isWideTable([]string{"a", "b"}))
}

func TestParseValueAndFloat(t *testing.T) {
	assert.Nil(t, parseValue(""))
	assert.Equal(t, 3.14, parseValue("3.14"))
	assert.Equal(t, "abc", parseValue("abc"))
	assert.Equal(t, 5.0, parseFloat("5"))
	assert.Equal(t, "x", parseFloat("x"))
}

func TestParseJSONToRows(t *testing.T) {
	arr, err := ParseJSONToRows([]byte(`[{"a":1},{"b":2}]`))
	assert.NoError(t, err)
	assert.Len(t, arr, 2)

	obj, err := ParseJSONToRows([]byte(`{"a":1}`))
	assert.NoError(t, err)
	assert.Len(t, obj, 1)

	none, err := ParseJSONToRows([]byte(`"scalar"`))
	assert.NoError(t, err)
	assert.Nil(t, none)

	_, err = ParseJSONToRows([]byte(`not-json`))
	assert.Error(t, err)
}

func TestParseFileToRows(t *testing.T) {
	csvRows, err := ParseFileToRows([]byte("a,b\n1,2\n"), "data.csv")
	assert.NoError(t, err)
	assert.Len(t, csvRows, 1)

	jsonRows, err := ParseFileToRows([]byte(`[{"a":1}]`), "data.json")
	assert.NoError(t, err)
	assert.Len(t, jsonRows, 1)

	// Unknown extension that parses as CSV.
	autoRows, err := ParseFileToRows([]byte("a,b\n1,2\n"), "data.txt")
	assert.NoError(t, err)
	assert.Len(t, autoRows, 1)
}
