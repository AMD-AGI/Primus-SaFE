/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cicdlog

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
)

var (
	consoleError  = regexp.MustCompile(`^(?:[0-9][0-9T:Z.+/-]*\s+){0,2}ERROR(?:\s|$)`)
	consoleFields = regexp.MustCompile(`(?:^|[\s,{])([A-Za-z]+)[=:]\s*"?([^"\s,}]+)"?`)
)

func NewestARCControllerError(scope Scope, responses []commonsearch.OpenSearchLogResponse, since, until time.Time) string {
	var newest time.Time
	var result string
	for _, response := range responses {
		for _, record := range response.Hits.Hits {
			timestamp, err := time.Parse(time.RFC3339Nano, record.Source.Timestamp)
			if err != nil || timestamp.Before(since) || timestamp.After(until) || (!newest.IsZero() && timestamp.Before(newest)) {
				continue
			}
			if !isARCControllerRecord(record) {
				continue
			}
			message := strings.TrimSpace(record.EffectiveMessage())
			fields := arcMessageFields(message)
			for key, value := range record.SourceFields {
				fields[key] = value
			}
			if message == "" || !matchesARCObject(fields, scope) || !isARCError(fields, message) {
				continue
			}
			result, newest = message, timestamp
		}
	}
	return result
}

func isARCControllerRecord(record commonsearch.OpenSearchLogDoc) bool {
	kubernetes, ok := record.SourceFields["kubernetes"].(map[string]interface{})
	if !ok {
		return false
	}
	namespace, _ := kubernetes["namespace_name"].(string)
	pod, _ := kubernetes["pod_name"].(string)
	return namespace == common.CICDArcNamespace && strings.HasPrefix(pod, commonconfig.GetCICDControllerName())
}

func arcMessageFields(message string) map[string]interface{} {
	fields := make(map[string]interface{})
	for i, char := range message {
		if char == '{' && json.Unmarshal([]byte(message[i:]), &fields) == nil {
			return fields
		}
	}
	for _, match := range consoleFields.FindAllStringSubmatch(message, -1) {
		fields[match[1]] = match[2]
	}
	return fields
}

func matchesARCObject(fields map[string]interface{}, scope Scope) bool {
	if object, ok := fields[scope.Kind].(map[string]interface{}); ok {
		return object["name"] == scope.Name && object["namespace"] == scope.Workspace
	}
	kind, _ := fields["controllerKind"].(string)
	if kind == "" {
		kind, _ = fields["kind"].(string)
	}
	return kind == scope.Kind && fields["name"] == scope.Name && fields["namespace"] == scope.Workspace
}

func isARCError(fields map[string]interface{}, message string) bool {
	if level, _ := fields["level"].(string); level != "" {
		return strings.EqualFold(level, "error")
	}
	if err, _ := fields["error"].(string); strings.TrimSpace(err) != "" {
		return true
	}
	return consoleError.MatchString(message)
}
