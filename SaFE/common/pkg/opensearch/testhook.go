/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import "time"

// SearchFunc is the signature of SearchClient.SearchByTimeRange. It is used by
// NewTestSearchClient to build a SearchClient whose data-plane requests are
// replaced by an in-process stub.
type SearchFunc func(sinceTime, untilTime time.Time, index, uri string, body []byte) ([]byte, error)

// NewTestSearchClient returns a SearchClient whose SearchByTimeRange delegates
// to fn instead of contacting a real robust-analyzer / OpenSearch backend.
//
// It exists so that handlers depending on the package-level GetOpensearchClient
// singleton can be unit-tested without standing up a live data plane. It must
// only be used from tests.
func NewTestSearchClient(fn SearchFunc) *SearchClient {
	return &SearchClient{searchFunc: fn}
}

// RegisterClientForTest installs sc as the cached OpenSearch client for the
// given cluster name and returns a cleanup function that removes it again.
// Intended for tests only.
func RegisterClientForTest(clusterName string, sc *SearchClient) func() {
	mu.Lock()
	defer mu.Unlock()
	multiClusterClients[clusterName] = sc
	return func() {
		mu.Lock()
		defer mu.Unlock()
		delete(multiClusterClients, clusterName)
	}
}
