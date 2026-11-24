/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// FormatRuntime renders a timeout in seconds as the workload timeline shows it, such as 3h or 1h30m; zero or less
// is unlimited.
func FormatRuntime(seconds int) string {
	if seconds <= 0 {
		return "unlimited"
	}
	text := (time.Duration(seconds) * time.Second).String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}
	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}
	return text
}

// NewRuntimeExtendedCondition returns the RuntimeExtended timeline entry of a workload whose runtime was set above
// its qos limit, or extended, with message saying by whom and to what.
func NewRuntimeExtendedCondition(message string) *metav1.Condition {
	return &metav1.Condition{
		Type:               string(v1.RuntimeExtended),
		Status:             metav1.ConditionTrue,
		Reason:             "Extended",
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now().UTC()),
	}
}
