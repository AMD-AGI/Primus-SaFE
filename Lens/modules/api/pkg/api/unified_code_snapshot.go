// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Request / Response types =====

type CodeSnapshotGetRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID string `json:"workload_uid" param:"workload_uid" mcp:"workload_uid,description=Workload UID,required"`
}

type CodeSnapshotGetResponse struct {
	ID             int64       `json:"id"`
	WorkloadUID    string      `json:"workload_uid"`
	EntryScript    interface{} `json:"entry_script,omitempty"`
	ConfigFiles    interface{} `json:"config_files,omitempty"`
	LocalModules   interface{} `json:"local_modules,omitempty"`
	ImportGraph    interface{} `json:"import_graph,omitempty"`
	PipFreeze      string      `json:"pip_freeze,omitempty"`
	WorkingDirTree string      `json:"working_dir_tree,omitempty"`
	Fingerprint    string      `json:"fingerprint,omitempty"`
	TotalSize      int         `json:"total_size"`
	FileCount      int         `json:"file_count"`
	CapturedAt     string      `json:"captured_at,omitempty"`
	CreatedAt      string      `json:"created_at"`
}

type CodeSnapshotDiffRequest struct {
	Cluster      string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID1 string `json:"workload_uid_1" query:"workload_uid_1" mcp:"workload_uid_1,description=First workload UID,required"`
	WorkloadUID2 string `json:"workload_uid_2" query:"workload_uid_2" mcp:"workload_uid_2,description=Second workload UID,required"`
}

type CodeSnapshotDiffResponse struct {
	WorkloadUID1    string           `json:"workload_uid_1"`
	WorkloadUID2    string           `json:"workload_uid_2"`
	SameFingerprint bool             `json:"same_fingerprint"`
	Fingerprint1    string           `json:"fingerprint_1"`
	Fingerprint2    string           `json:"fingerprint_2"`
	EntryScriptDiff *FileDiff        `json:"entry_script_diff,omitempty"`
	ConfigFilesDiffs []ConfigFileDiff `json:"config_files_diffs,omitempty"`
	PipFreezeDiff   *StringDiff      `json:"pip_freeze_diff,omitempty"`
}

type FileDiff struct {
	Path1   string `json:"path_1,omitempty"`
	Path2   string `json:"path_2,omitempty"`
	Hash1   string `json:"hash_1,omitempty"`
	Hash2   string `json:"hash_2,omitempty"`
	Changed bool   `json:"changed"`
}

type ConfigFileDiff struct {
	Path    string `json:"path"`
	In1     bool   `json:"in_1"`
	In2     bool   `json:"in_2"`
	Changed bool   `json:"changed"`
}

type StringDiff struct {
	Length1 int  `json:"length_1"`
	Length2 int  `json:"length_2"`
	Changed bool `json:"changed"`
}

// ===== Endpoint registration =====

func init() {
	unified.Register(&unified.EndpointDef[CodeSnapshotGetRequest, CodeSnapshotGetResponse]{
		Name:        "code_snapshot_get",
		Description: "Get code snapshot for a workload (entry script, config files, pip freeze, fingerprint)",
		HTTPMethod:  "GET",
		HTTPPath:    "/code-snapshot/:workload_uid",
		MCPToolName: "lens_code_snapshot_get",
		Handler:     handleCodeSnapshotGet,
	})

	unified.Register(&unified.EndpointDef[CodeSnapshotDiffRequest, CodeSnapshotDiffResponse]{
		Name:        "code_snapshot_diff",
		Description: "Compare code snapshots between two workloads to identify configuration and code differences",
		HTTPMethod:  "GET",
		HTTPPath:    "/code-snapshot/diff",
		MCPToolName: "lens_code_snapshot_diff",
		Handler:     handleCodeSnapshotDiff,
	})
}

// ===== Handlers =====

func handleCodeSnapshotGet(ctx context.Context, req *CodeSnapshotGetRequest) (*CodeSnapshotGetResponse, error) {
	if req.WorkloadUID == "" {
		return nil, fmt.Errorf("workload_uid is required")
	}
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}
	raw, err := rc.GetRaw(ctx, "/intents/"+req.WorkloadUID+"/code-snapshot", nil)
	if err != nil {
		return nil, fmt.Errorf("robust code snapshot: %w", err)
	}
	var resp CodeSnapshotGetResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("robust code snapshot decode: %w", err)
	}
	return &resp, nil
}

func handleCodeSnapshotDiff(ctx context.Context, req *CodeSnapshotDiffRequest) (*CodeSnapshotDiffResponse, error) {
	if req.WorkloadUID1 == "" || req.WorkloadUID2 == "" {
		return nil, fmt.Errorf("both workload_uid_1 and workload_uid_2 are required")
	}
	rc, err := getRobustClient(req.Cluster)
	if err != nil {
		return nil, err
	}

	var s1, s2 CodeSnapshotGetResponse
	raw1, err1 := rc.GetRaw(ctx, "/intents/"+req.WorkloadUID1+"/code-snapshot", nil)
	if err1 == nil {
		_ = json.Unmarshal(raw1, &s1)
	}
	raw2, err2 := rc.GetRaw(ctx, "/intents/"+req.WorkloadUID2+"/code-snapshot", nil)
	if err2 == nil {
		_ = json.Unmarshal(raw2, &s2)
	}
	if err1 != nil && err2 != nil {
		return nil, fmt.Errorf("no code snapshots found for either workload")
	}

	resp := &CodeSnapshotDiffResponse{
		WorkloadUID1:    req.WorkloadUID1,
		WorkloadUID2:    req.WorkloadUID2,
		Fingerprint1:    s1.Fingerprint,
		Fingerprint2:    s2.Fingerprint,
		SameFingerprint: s1.Fingerprint != "" && s1.Fingerprint == s2.Fingerprint,
	}
	if s1.WorkloadUID != "" || s2.WorkloadUID != "" {
		resp.EntryScriptDiff = &FileDiff{Changed: !resp.SameFingerprint}
	}
	if s1.PipFreeze != "" || s2.PipFreeze != "" {
		resp.PipFreezeDiff = &StringDiff{
			Length1: len(s1.PipFreeze),
			Length2: len(s2.PipFreeze),
			Changed: s1.PipFreeze != s2.PipFreeze,
		}
	}
	return resp, nil
}
