/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package stringutil

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	Base64 string = "^(?:[A-Za-z0-9+\\/]{4})*(?:[A-Za-z0-9+\\/]{2}==|[A-Za-z0-9+\\/]{3}=|[A-Za-z0-9+\\/]{4})$"
)

var (
	rxBase64 = regexp.MustCompile(Base64)
)

// Base64Encode encodes a string to base64 format
func Base64Encode(inputString string) string {
	if inputString == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(inputString))
}

// Base64Decode decodes a base64 encoded string, returns empty string if decode fails
func Base64Decode(inputString string) string {
	if inputString == "" {
		return ""
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(inputString)
	if err != nil {
		return ""
	}
	return string(decodedBytes)
}

// IsBase64 check if a string is base64 encoded.
func IsBase64(str string) bool {
	return rxBase64.MatchString(str)
}

// MD5 generates MD5 hash of the input string and returns it as hex string
func MD5(input string) string {
	data := []byte(input)

	hash := md5.New()
	hash.Write(data)
	hashInBytes := hash.Sum(nil)

	md5String := hex.EncodeToString(hashInBytes)
	return md5String
}

// NormalizeName converts string to lowercase, trims whitespace, and replaces underscores with hyphens
func NormalizeName(str string) string {
	if str == "" {
		return ""
	}
	str = strings.ToLower(str)
	str = strings.TrimSpace(str)
	str = strings.ReplaceAll(str, "_", "-")
	return str
}

// StrCaseEqual compares two strings case-insensitively
func StrCaseEqual(str1, str2 string) bool {
	if strings.ToLower(str1) == strings.ToLower(str2) {
		return true
	}
	return false
}

// ExtractNumber extracts numeric characters from a string and converts to int64
func ExtractNumber(s string) int64 {
	var str string
	for _, c := range s {
		if c >= '0' && c <= '9' {
			str += string(c)
		}
	}
	num, err := strconv.ParseInt(str, 10, 0)
	if err != nil {
		return 0
	}
	return num
}

// IsNumber checks if a string can be converted to an integer
func IsNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// ConvertToString converts various types to string representation
func ConvertToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%f", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		// Return empty string if type is not any of the above
		return ""
	}
}

// Split splits a string by the given separator and trims whitespace from each part.
// Empty strings after trimming are filtered out from the result.
// Returns a slice of non-empty, whitespace-trimmed strings or nil if the input string is empty.
func Split(str, sep string) []string {
	if len(str) == 0 {
		return nil
	}
	strList := strings.Split(str, sep)
	var result []string
	for _, s := range strList {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		result = append(result, s)
	}
	return result
}
