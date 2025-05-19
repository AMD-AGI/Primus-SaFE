/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

func Base64Encode(inputString string) string {
	if inputString == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(inputString))
}

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

func MD5(input string) string {
	data := []byte(input)

	hash := md5.New()
	hash.Write(data)
	hashInBytes := hash.Sum(nil)

	md5String := hex.EncodeToString(hashInBytes)
	return md5String
}

func NormalizeName(str string) string {
	if str == "" {
		return ""
	}
	str = strings.ToLower(str)
	str = strings.TrimSpace(str)
	str = strings.ReplaceAll(str, "_", "-")
	return str
}

func StrCaseEqual(str1, str2 string) bool {
	if strings.ToLower(str1) == strings.ToLower(str2) {
		return true
	}
	return false
}

// Extract the numeric part from the string
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

func IsNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

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
		// 如果类型不是上述任何一种，返回其类型名称
		return ""
	}
}
