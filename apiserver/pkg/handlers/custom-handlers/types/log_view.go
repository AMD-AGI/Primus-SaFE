/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"time"
)

const (
	LogId = "logId"
)

type GetLogRequest struct {
	// Start timestamp of the query. defaults to one day ago. e.g.: '2006-01-02T15:04:05.000Z'
	Since string `json:"since,omitempty"`
	// End timestamp of the query, defaults to current time.
	Until string `json:"until,omitempty"`
	// Starting offset for the results
	Offset int `json:"offset,omitempty"`
	// Limit the number of returned results
	Limit int `json:"limit,omitempty"`
	// asc/desc
	Order string `json:"order,omitempty"`
	// Search for the given keywords. Multiple entries are treated as AND conditions.
	// Example: ['key1', 'key2']
	Keywords []string `json:"keywords,omitempty"`
	// Use the specified pod name for filtering.
	PodName string `json:"podName,omitempty"`
	// Run number to filter by. Default 0 means all;
	// a positive number indicates a specific number.
	DispatchCount int `json:"dispatchCount,omitempty"`
	// Kubernetes node name used for filtering. E.g., smc300x-ccs-aus-a16-10
	// Multiple entries are allowed, separated by commas, and treated as OR conditions
	NodeNames string `json:"nodeNames,omitempty"`

	// internal use
	SinceTime time.Time         `json:"-"`
	UntilTime time.Time         `json:"-"`
	Filters   map[string]string `json:"-"`
}

type GetLogRequestWrapper struct {
	Query *GetLogRequest
	Id    int
}
