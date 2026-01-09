// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stringUtil

import (
	"strings"
	"unicode"
)

func SnakeCaseToCamelCase(input string) string {
	titleCased := strings.Title(strings.ReplaceAll(input, "_", " "))
	camelCased := strings.ReplaceAll(titleCased, " ", "")
	return string(unicode.ToLower(rune(camelCased[0]))) + camelCased[1:]
}

func SnakeCaseToUpperCamelCase(input string) string {
	titleCased := strings.Title(strings.ReplaceAll(input, "_", " "))
	return strings.ReplaceAll(titleCased, " ", "")
}

func CamelCaseToSnakeCase(input string) string {
	var result []rune
	for i, r := range input {
		if unicode.IsUpper(r) {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}
