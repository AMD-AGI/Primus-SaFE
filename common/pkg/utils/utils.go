/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"

	utilrand "k8s.io/apimachinery/pkg/util/rand"
)

const (
	Safe                   = "safe-"
	MaxNameLength          = 63
	randomLength           = 5
	MaxGeneratedNameLength = MaxNameLength - randomLength - 1
	// 12 is the fixed suffix length of pytorchjob.
	MaxDisplayNameLen = MaxGeneratedNameLength - len(Safe) - 12
)

func GenerateName(base string) string {
	if base == "" {
		return ""
	}
	if len(base) > MaxGeneratedNameLength {
		return ""
	}
	return fmt.Sprintf("%s-%s", base, utilrand.String(randomLength))
}

func GenerateTruncationName(base string) string {
	if base == "" {
		return ""
	}
	if len(base) > MaxGeneratedNameLength {
		base = base[0:MaxGeneratedNameLength]
	}
	return fmt.Sprintf("%s-%s", base, utilrand.String(randomLength))
}

func GenerateNameWithPrefix(base string) string {
	name := Safe + base
	return GenerateName(name)
}

func GetBaseByGenerateName(name string) string {
	if len(name) <= randomLength+1 {
		return name
	}
	return name[0 : len(name)-randomLength-1]
}
