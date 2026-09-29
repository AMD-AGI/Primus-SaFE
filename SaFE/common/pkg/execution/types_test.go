/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package execution

import (
	"encoding/json"
	"strings"
	"testing"
)

// The provider may add placement fields; DisallowUnknownFields rejects the whole reply
// unless the client struct names them. node_addresses is required for claim decode today.
func TestClaimPlacementDecodesNodeAddresses(t *testing.T) {
	const body = `{
		"api_version":"safe-exec/v1alpha1",
		"request_id":"11111111-1111-1111-1111-111111111111",
		"claim_id":"22222222-2222-2222-2222-222222222222",
		"revision":1,
		"phase":"Active",
		"workload_uid":"33333333-3333-3333-3333-333333333333",
		"dispatch_generation":1,
		"cluster_id":"crusoe",
		"workspace_id":"ws",
		"expires_at":"2026-09-29T12:00:00.000Z",
		"placements":[{
			"unit_key":"master/0",
			"allocation_id":"44444444-4444-4444-4444-444444444444",
			"allocation_generation":1,
			"expected_allocation_revision":1,
			"node_name":"vk-1",
			"image_ref":"registry.example/x@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"image_digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"resources":{"cpu_millis":1000,"memory_bytes":1,"scratch_bytes":0,"gpu_resource":"amd.com/gpu","gpu_model":"MI355X","gpu_count":1},
			"pid_limit":256,
			"device_ids":["0002:00:01.0"],
			"ports":[],
			"node_addresses":[{"type":"InternalIP","address":"10.245.155.27"}]
		}]
	}`
	var claim ClaimResponse
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&claim); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	if len(claim.Placements) != 1 || len(claim.Placements[0].NodeAddresses) != 1 {
		t.Fatalf("placements = %+v", claim.Placements)
	}
	addr := claim.Placements[0].NodeAddresses[0]
	if addr.Type != "InternalIP" || addr.Address != "10.245.155.27" {
		t.Fatalf("node_addresses = %+v", addr)
	}
}

// Placements may carry the per-container image approvals; a reply that names them must
// still decode.
func TestClaimPlacementDecodesImages(t *testing.T) {
	const body = `{
		"unit_key":"master/0",
		"allocation_id":"44444444-4444-4444-4444-444444444444",
		"allocation_generation":1,
		"expected_allocation_revision":1,
		"node_name":"vk-1",
		"image_ref":"docker.io/team/app:v1@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"image_digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"resources":{"cpu_millis":1000,"memory_bytes":1,"scratch_bytes":0,"gpu_resource":"amd.com/gpu","gpu_model":"MI355X","gpu_count":1},
		"pid_limit":256,
		"device_ids":[],
		"ports":[],
		"images":[{
			"name":"pytorch",
			"image_ref":"docker.io/team/app:v1@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"image_digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		}]
	}`
	var placement ClaimPlacement
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&placement); err != nil {
		t.Fatalf("decode placement: %v", err)
	}
	if len(placement.Images) != 1 || placement.Images[0].Name != "pytorch" {
		t.Fatalf("images = %+v", placement.Images)
	}
	// The field is replayed from the plan into the claim, so an absent one must stay absent.
	placement.Images = nil
	raw, err := json.Marshal(placement)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"images"`) {
		t.Fatalf("empty images must be omitted: %s", raw)
	}
}
