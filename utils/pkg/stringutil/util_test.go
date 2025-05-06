/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package stringutil

import (
	"fmt"
	"testing"

	"gotest.tools/assert"
)

func TestBase64Encode(t *testing.T) {
	pwd := "tT5+uQ0^qF4,fL6{"
	encode := Base64Encode(pwd)
	fmt.Println(encode)
	assert.Equal(t, encode, "dFQ1K3VRMF5xRjQsZkw2ew==")
}

func TestMD5(t *testing.T) {
	pwd := "tT5+uQ0^qF4,fL6{"
	encode := MD5(pwd)
	fmt.Println(encode)
	assert.Equal(t, encode, "091611f47f7f5c5ac81c004c0169f831")
}

func TestConvertToString(t *testing.T) {
	var value interface{} = true
	assert.Equal(t, ConvertToString(value), "true")
	value = false
	assert.Equal(t, ConvertToString(value), "false")

	value = 123
	assert.Equal(t, ConvertToString(value), "123")

	value = 3.14
	assert.Equal(t, ConvertToString(value), "3.140000")

	value = "hello"
	assert.Equal(t, ConvertToString(value), "hello")

	value = struct{ name string }{}
	assert.Equal(t, ConvertToString(value), "")
}
