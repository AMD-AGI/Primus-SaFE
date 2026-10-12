/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"bufio"
	"bytes"
	"strings"

	"k8s.io/klog/v2"

	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	// LogErrorMarker marks a log line of an OpsJob workload that reports an error.
	LogErrorMarker = "[ERROR]"
	// LogSuccessMarker marks a log line of an OpsJob workload that reports a result.
	LogSuccessMarker = "[SUCCESS]"
)

// FilterResultLog keeps the lines of an OpsJob workload log that carry LogErrorMarker or
// LogSuccessMarker and returns them as a JSON array, or "" when there is none. Only these
// lines reach the job's completion message and outputs; a job that wants a value in its
// outputs prints it on a marked line.
func FilterResultLog(data []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, LogErrorMarker) || strings.Contains(line, LogSuccessMarker) {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		klog.ErrorS(err, "failed to read pod log lines")
	}
	if len(lines) == 0 {
		return ""
	}
	return string(jsonutils.MarshalSilently(lines))
}
