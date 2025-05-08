/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"

	utilrand "k8s.io/apimachinery/pkg/util/rand"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
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

func GenerateTaintKey(code string) string {
	key := v1.PrimusTaintPrefix + code
	return stringutil.NormalizeName(key)
}

func GetCodeByTaintKey(taintKey string) string {
	if len(taintKey) <= len(v1.PrimusTaintPrefix) {
		return ""
	}
	return taintKey[len(v1.PrimusTaintPrefix):]
}
