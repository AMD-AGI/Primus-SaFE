/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cicdlog

import "time"

type ListLogInput struct {
	Since         string   `json:"since,omitempty"`
	Until         string   `json:"until,omitempty"`
	Offset        int      `json:"offset,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	Order         string   `json:"order,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	PodNames      string   `json:"podNames,omitempty"`
	DispatchCount int      `json:"dispatchCount,omitempty"`
	NodeNames     string   `json:"nodeNames,omitempty"`
}

type Query struct {
	ListLogInput
	SinceTime     time.Time
	UntilTime     time.Time
	TermFilters   map[string]string
	PrefixFilters map[string]string
	UseK8sLabel   bool
	DisableOutput bool
}
