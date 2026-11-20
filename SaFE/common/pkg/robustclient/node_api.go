/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"errors"
	"net/url"
)

const (
	nodeSyncPath       = "/api/v1/nodes/sync"
	nodeSyncBatchPath  = "/api/v1/nodes/sync/batch"
	nodeSyncDigestPath = "/api/v1/nodes/sync/digest"

	// NodeSyncBatchSize bounds the nodes sent in one batch request.
	NodeSyncBatchSize = 200
)

// SyncNode creates or updates one node in Robust.
func (cc *ClusterClient) SyncNode(ctx context.Context, node NodeSyncPayload) error {
	return asNotSupported(cc.Post(ctx, nodeSyncPath, node, nil))
}

// SyncNodes sends nodes in batches of NodeSyncBatchSize. It sends every batch
// even after a failure and returns how many nodes were in the batches Robust
// accepted, with the errors of the others.
func (cc *ClusterClient) SyncNodes(ctx context.Context, nodes []NodeSyncPayload) (int, error) {
	synced := 0
	var errs []error
	for start := 0; start < len(nodes); start += NodeSyncBatchSize {
		end := min(start+NodeSyncBatchSize, len(nodes))
		batch := NodeSyncBatchPayload{Nodes: nodes[start:end]}
		if err := cc.Post(ctx, nodeSyncBatchPath, batch, nil); err != nil {
			errs = append(errs, asNotSupported(err))
			continue
		}
		synced += end - start
	}
	return synced, errors.Join(errs...)
}

// DeleteNode removes a node from Robust.
func (cc *ClusterClient) DeleteNode(ctx context.Context, name string) error {
	return asNotSupported(cc.Delete(ctx, nodeSyncPath+"/"+url.PathEscape(name), nil))
}

// GetNodeSyncDigest returns what Robust holds for every node synced from SaFE,
// soft-deleted nodes included.
func (cc *ClusterClient) GetNodeSyncDigest(ctx context.Context) (*NodeSyncDigest, error) {
	digest := &NodeSyncDigest{}
	if err := cc.Get(ctx, nodeSyncDigestPath, nil, digest); err != nil {
		return nil, asNotSupported(err)
	}
	return digest, nil
}
