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
	str := "test"
	encode := Base64Encode(str)
	fmt.Println(encode)
	assert.Equal(t, encode, "dGVzdAo=")
}

func TestMD5(t *testing.T) {
	str := "root"
	encode := MD5(str)
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
