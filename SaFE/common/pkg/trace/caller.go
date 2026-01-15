// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package trace

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

const (
	callerKeyword = "primus-safe"
)

var (
	callerIgnoresRegex = []*regexp.Regexp{
		// Ignore DAL layer calls in primus-safe
		regexp.MustCompile(`^primus-safe/[^*]+/database/[^*]+/dal[^*]+$`),
	}
	packagePrefixList = []string{
		"github.com/AMD-AGI/Primus-SaFE/SaFE",
	}
)

func GetNearestCaller(callerSkip int) string {
	callers := make([]uintptr, 64)
	i := runtime.Callers(1+callerSkip, callers)
	frames := runtime.CallersFrames(callers[:i])
	for {
		frame, more := frames.Next()
		if !more {
			break
		}
		if !strings.Contains(frame.File, callerKeyword) {
			continue
		}
		if !isCallerIgnored(frame.Function) {
			return fmt.Sprintf("%s:%d", getPackageName(frame.Function), frame.Line)
		}
	}
	return ""
}

func isCallerIgnored(caller string) bool {
	for _, reg := range callerIgnoresRegex {
		if reg.MatchString(caller) {
			return true
		}
	}
	return false
}

func getPackageName(caller string) string {
	datas := strings.Split(caller, "github.com/")
	if len(datas) < 2 {
		return caller
	}
	return datas[1]
}

func TrimPackagePrefixes(caller string) string {
	result := caller
	for _, prefix := range packagePrefixList {
		result = strings.TrimPrefix(result, prefix)
	}
	return result
}
