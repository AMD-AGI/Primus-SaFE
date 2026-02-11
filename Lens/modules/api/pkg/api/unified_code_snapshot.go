// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Request / Response types =====

// CodeSnapshotGetRequest retrieves code snapshot for a workload
type CodeSnapshotGetRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID string `json:"workload_uid" param:"workload_uid" mcp:"workload_uid,description=Workload UID,required"`
}

// CodeSnapshotGetResponse returns the code snapshot
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

// CodeSnapshotDiffRequest compares code snapshots of two workloads
type CodeSnapshotDiffRequest struct {
	Cluster      string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID1 string `json:"workload_uid_1" query:"workload_uid_1" mcp:"workload_uid_1,description=First workload UID,required"`
	WorkloadUID2 string `json:"workload_uid_2" query:"workload_uid_2" mcp:"workload_uid_2,description=Second workload UID,required"`
}

// CodeSnapshotDiffResponse returns the diff between two snapshots
type CodeSnapshotDiffResponse struct {
	WorkloadUID1     string             `json:"workload_uid_1"`
	WorkloadUID2     string             `json:"workload_uid_2"`
	SameFingerprint  bool               `json:"same_fingerprint"`
	Fingerprint1     string             `json:"fingerprint_1"`
	Fingerprint2     string             `json:"fingerprint_2"`
	EntryScriptDiff  *FileDiff          `json:"entry_script_diff,omitempty"`
	ConfigFilesDiffs []ConfigFileDiff   `json:"config_files_diffs,omitempty"`
	PipFreezeDiff    *StringDiff        `json:"pip_freeze_diff,omitempty"`
}

// FileDiff represents a diff between two file snapshots
type FileDiff struct {
	Path1   string `json:"path_1,omitempty"`
	Path2   string `json:"path_2,omitempty"`
	Hash1   string `json:"hash_1,omitempty"`
	Hash2   string `json:"hash_2,omitempty"`
	Changed bool   `json:"changed"`
}

// ConfigFileDiff represents a diff between config files
type ConfigFileDiff struct {
	Path    string `json:"path"`
	In1     bool   `json:"in_1"` // Present in snapshot 1
	In2     bool   `json:"in_2"` // Present in snapshot 2
	Changed bool   `json:"changed"`
}

// StringDiff represents a simple diff between two string values
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
		return nil, errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("workload_uid is required")
	}

	cm := clientsets.GetClusterManager()
	clusterName, err := resolveWorkloadCluster(cm, req.WorkloadUID, req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName)
	snapshot, err := facade.GetWorkloadCodeSnapshot().GetByWorkloadUID(ctx, req.WorkloadUID)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to get code snapshot: " + err.Error())
	}
	if snapshot == nil {
		return nil, errors.NewError().
			WithCode(errors.RequestDataNotExisted).
			WithMessage("no code snapshot found for workload: " + req.WorkloadUID)
	}

	return convertSnapshotToResponse(snapshot), nil
}

func handleCodeSnapshotDiff(ctx context.Context, req *CodeSnapshotDiffRequest) (*CodeSnapshotDiffResponse, error) {
	if req.WorkloadUID1 == "" || req.WorkloadUID2 == "" {
		return nil, errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("both workload_uid_1 and workload_uid_2 are required")
	}

	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)

	snapshot1, err := facade.GetWorkloadCodeSnapshot().GetByWorkloadUID(ctx, req.WorkloadUID1)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to get snapshot for workload 1: " + err.Error())
	}

	snapshot2, err := facade.GetWorkloadCodeSnapshot().GetByWorkloadUID(ctx, req.WorkloadUID2)
	if err != nil {
		return nil, errors.NewError().
			WithCode(errors.InternalError).
			WithMessage("failed to get snapshot for workload 2: " + err.Error())
	}

	if snapshot1 == nil && snapshot2 == nil {
		return nil, errors.NewError().
			WithCode(errors.RequestDataNotExisted).
			WithMessage("no code snapshots found for either workload")
	}

	return buildDiffResponse(req.WorkloadUID1, req.WorkloadUID2, snapshot1, snapshot2), nil
}

// ===== Conversion helpers =====

func convertSnapshotToResponse(s *dbModel.WorkloadCodeSnapshot) *CodeSnapshotGetResponse {
	resp := &CodeSnapshotGetResponse{
		ID:             s.ID,
		WorkloadUID:    s.WorkloadUID,
		EntryScript:    s.EntryScript,
		ConfigFiles:    s.ConfigFiles,
		LocalModules:   s.LocalModules,
		ImportGraph:    s.ImportGraph,
		PipFreeze:      s.PipFreeze,
		WorkingDirTree: s.WorkingDirTree,
		Fingerprint:    s.Fingerprint,
		TotalSize:      int(s.TotalSize),
		FileCount:      int(s.FileCount),
		CreatedAt:      s.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if s.CapturedAt != nil {
		resp.CapturedAt = s.CapturedAt.Format("2006-01-02T15:04:05Z")
	}
	return resp
}

func buildDiffResponse(uid1, uid2 string, s1, s2 *dbModel.WorkloadCodeSnapshot) *CodeSnapshotDiffResponse {
	resp := &CodeSnapshotDiffResponse{
		WorkloadUID1: uid1,
		WorkloadUID2: uid2,
	}

	fp1 := ""
	fp2 := ""
	if s1 != nil {
		fp1 = s1.Fingerprint
	}
	if s2 != nil {
		fp2 = s2.Fingerprint
	}
	resp.Fingerprint1 = fp1
	resp.Fingerprint2 = fp2
	resp.SameFingerprint = fp1 != "" && fp1 == fp2

	// Entry script diff
	if s1 != nil || s2 != nil {
		resp.EntryScriptDiff = &FileDiff{
			Changed: !resp.SameFingerprint,
		}
	}

	// Pip freeze diff
	pipFreeze1 := ""
	pipFreeze2 := ""
	if s1 != nil {
		pipFreeze1 = s1.PipFreeze
	}
	if s2 != nil {
		pipFreeze2 = s2.PipFreeze
	}
	if pipFreeze1 != "" || pipFreeze2 != "" {
		resp.PipFreezeDiff = &StringDiff{
			Length1: len(pipFreeze1),
			Length2: len(pipFreeze2),
			Changed: pipFreeze1 != pipFreeze2,
		}
	}

	return resp
}
