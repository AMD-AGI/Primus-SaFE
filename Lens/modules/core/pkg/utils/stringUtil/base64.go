// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stringUtil

import (
	"encoding/base64"
)

// EncodeBase64 encodes a string to base64 format
func EncodeBase64(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

// DecodeBase64 decodes a base64 string to original string
// Returns empty string and error if decoding fails
func DecodeBase64(str string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// EncodeBase64URL encodes a string to URL-safe base64 format
func EncodeBase64URL(str string) string {
	return base64.URLEncoding.EncodeToString([]byte(str))
}

// DecodeBase64URL decodes a URL-safe base64 string to original string
// Returns empty string and error if decoding fails
func DecodeBase64URL(str string) (string, error) {
	decoded, err := base64.URLEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
