/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

const (
	syncDigestPath = "/api/v1/workloads/sync/digest"
	gpuWindowPath  = "/api/v1/workloads/gpu-utilization/window"

	// GPUWindowBatchSize bounds the workload uids sent in one window request.
	GPUWindowBatchSize = 200
)

// ErrNotSupported reports that the cluster's Robust API does not serve an
// endpoint yet (it answered 404 or 501).
var ErrNotSupported = errors.New("not supported by this Robust API")

// StatusError is a non-2xx answer of the Robust API.
type StatusError struct {
	StatusCode int
	msg        string
}

func (e *StatusError) Error() string { return e.msg }

// IsNotSupported tells whether err means the Robust API lacks the endpoint.
func IsNotSupported(err error) bool {
	return errors.Is(err, ErrNotSupported)
}

func asNotSupported(err error) error {
	var se *StatusError
	if errors.As(err, &se) && (se.StatusCode == http.StatusNotFound || se.StatusCode == http.StatusNotImplemented) {
		return fmt.Errorf("%w: %v", ErrNotSupported, err)
	}
	return err
}

// GetSyncDigest returns what Robust holds for every workload synced from SaFE.
func (cc *ClusterClient) GetSyncDigest(ctx context.Context) (*WorkloadSyncDigest, error) {
	digest := &WorkloadSyncDigest{}
	if err := cc.Get(ctx, syncDigestPath, nil, digest); err != nil {
		return nil, asNotSupported(err)
	}
	return digest, nil
}

// GetWorkloadGPUWindow returns the GPU utilization of the given workloads over
// window, sampled every step. Workloads without data are absent from the result.
func (cc *ClusterClient) GetWorkloadGPUWindow(ctx context.Context,
	uids []string, window, step string) ([]WorkloadGPUWindow, error) {
	var result []WorkloadGPUWindow
	for start := 0; start < len(uids); start += GPUWindowBatchSize {
		end := start + GPUWindowBatchSize
		if end > len(uids) {
			end = len(uids)
		}
		req := GPUWindowRequest{WorkloadUIDs: uids[start:end], Window: window, Step: step}
		resp := &GPUWindowResponse{}
		if err := cc.Post(ctx, gpuWindowPath, req, resp); err != nil {
			return nil, asNotSupported(err)
		}
		result = append(result, resp.Workloads...)
	}
	return result, nil
}
