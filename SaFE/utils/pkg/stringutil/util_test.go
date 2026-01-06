/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
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
	pwd := "root"
	encode := MD5(pwd)
	fmt.Println(encode)
	assert.Equal(t, encode, "63a9f0ea7bb98050796b649e85481845")
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
