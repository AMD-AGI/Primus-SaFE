package trace

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

const (
	callerKeyword = "primus-lens"
)

var (
	callerIgnoresRegex = []*regexp.Regexp{
		regexp.MustCompile(`^primus-lens/[^*]+/database/[^*]+/dal[^*]+$`),
	}
	packagePrefixList = []string{
		"github.com/AMD-AGI/primus-lens",
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
