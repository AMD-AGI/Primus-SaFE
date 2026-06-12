/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package stringutil

import (
	"fmt"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestBase64Encode(t *testing.T) {
	str := "test"
	encode := Base64Encode(str)
	fmt.Println(encode)
	assert.Equal(t, encode, "dGVzdA==")
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

func TestBase64Decode(t *testing.T) {
	assert.Equal(t, Base64Decode("dGVzdA=="), "test")
	assert.Equal(t, Base64Decode(""), "")
	assert.Equal(t, Base64Decode("not base64!!!"), "")
}

func TestIsBase64(t *testing.T) {
	assert.Equal(t, IsBase64("dGVzdA=="), true)
	assert.Equal(t, IsBase64("!!!"), false)
}

func TestNormalizeName(t *testing.T) {
	assert.Equal(t, NormalizeName(""), "")
	assert.Equal(t, NormalizeName("  Hello_World\n\r"), "hello-world")
}

func TestNormalizeForDNS(t *testing.T) {
	assert.Equal(t, NormalizeForDNS("My_Model.v1/test"), "my-model-v1-test")
	assert.Equal(t, NormalizeForDNS("123abc"), "n123abc")
	assert.Equal(t, NormalizeForDNS("!!!"), "model")
	assert.Equal(t, len(NormalizeForDNS(strings.Repeat("a", 60))) <= 45, true)
}

func TestStrCaseEqual(t *testing.T) {
	assert.Equal(t, StrCaseEqual("ABC", "abc"), true)
	assert.Equal(t, StrCaseEqual("a", "b"), false)
}

func TestExtractNumber(t *testing.T) {
	assert.Equal(t, ExtractNumber("node-12-3"), int64(123))
	assert.Equal(t, ExtractNumber("nonum"), int64(0))
}

func TestSplit(t *testing.T) {
	assert.Equal(t, len(Split("", ",")), 0)
	assert.DeepEqual(t, Split("a, b , ,c", ","), []string{"a", "b", "c"})
}
