/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"bufio"
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"k8s.io/klog/v2"

	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	// LogErrorMarker marks a log line of an OpsJob workload that reports an error.
	LogErrorMarker = "[ERROR]"
	// LogSuccessMarker marks a log line of an OpsJob workload that reports a result.
	LogSuccessMarker = "[SUCCESS]"

	// maxResultLineBytes is the longest line FilterRunResultLog parses.
	maxResultLineBytes = 1 << 20
)

var (
	ansiPattern      = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	rayPrefixPattern = regexp.MustCompile(`^\([^()]*pid=\d+[^()]*\)\s*`)
)

// StripMarkerPrefix removes color codes and a leading Ray actor prefix, such as "(TrainActor pid=692)", from a
// log line, so a marker printed by a Ray actor starts the line.
func StripMarkerPrefix(line string) string {
	return rayPrefixPattern.ReplaceAllString(strings.TrimRight(ansiPattern.ReplaceAllString(line, ""), "\r"), "")
}

// FilterRunResultLog keeps the [ERROR] and [SUCCESS] lines of a Post Training log that are addressed to run and
// returns them as a JSON array, or "" when there is none. After StripMarkerPrefix a line must start with the
// marker, followed by JSON whose run field is run; lines that merely contain a marker, and lines over 1 MB,
// are ignored.
func FilterRunResultLog(data []byte, run string) string {
	var lines []string
	for _, raw := range bytes.Split(data, []byte("\n")) {
		if len(raw) > maxResultLineBytes {
			continue
		}
		line := StripMarkerPrefix(string(raw))
		for _, marker := range []string{LogErrorMarker, LogSuccessMarker} {
			if !strings.HasPrefix(line, marker+" ") {
				continue
			}
			var body struct {
				Run string `json:"run"`
			}
			if payload := strings.TrimSpace(line[len(marker):]); json.Unmarshal([]byte(payload), &body) == nil &&
				body.Run == run {
				lines = append(lines, line)
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return string(jsonutils.MarshalSilently(lines))
}

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
