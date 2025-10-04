/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"time"
)

const (
	DocId = "docId"
)

type ListLogRequest struct {
	// Start timestamp of the query. defaults to one day ago. e.g.: '2006-01-02T15:04:05.000Z'
	Since string `json:"since,omitempty"`
	// End timestamp of the query, defaults to current time.
	Until string `json:"until,omitempty"`
	// Starting offset for the results. default 0
	Offset int `json:"offset,omitempty"`
	// Limit the number of returned results. default 100
	Limit int `json:"limit,omitempty"`
	// asc/desc
	Order string `json:"order,omitempty"`
	// Search for the given keywords. Multiple entries are treated as AND conditions.
	// Example: ['key1', 'key2']
	Keywords []string `json:"keywords,omitempty"`
	// Use the specified pod names for filtering.
	// Multiple entries are allowed, separated by commas, and treated as OR conditions
	PodNames string `json:"podNames,omitempty"`
	// Run number to filter by. Default 0 means all;
	// a positive number indicates a specific number.
	DispatchCount int `json:"dispatchCount,omitempty"`
	// Kubernetes node name used for filtering. E.g., smc300x-ccs-aus-a16-10
	// Multiple entries are allowed, separated by commas, and treated as OR conditions
	// If both PodNames and NodeNames are set, only PodNames will take effect.
	NodeNames string `json:"nodeNames,omitempty"`

	// internal use
	SinceTime time.Time         `json:"-"`
	UntilTime time.Time         `json:"-"`
	Filters   map[string]string `json:"-"`
}

type ListContextLogRequest struct {
	Query *ListLogRequest
	Id    int
	Limit int
	DocId string
}
