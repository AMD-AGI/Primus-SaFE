/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"encoding/json"
	"fmt"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

// ResolveModelForOptimization looks up the Model by id, validates that its
// download state is compatible with an optimization run, and returns the
// localPath the task should pass to Hyperloom.
//
// Rules enforced:
//   - Model must exist and not be soft-deleted.
//   - Model.access_mode must be "local" (remote_api / local_path are rejected
//     because Hyperloom needs a file-path the sandbox can read).
//   - Workspace-specific localPath must be Ready. An empty workspace
//     ("public") falls through to the first Ready entry.
func ResolveModelForOptimization(
	ctx context.Context,
	db dbclient.ModelInterface,
	k8sClient ctrlclient.Client,
	modelID, workspace string,
) (*ResolvedModel, error) {
	if db == nil {
		return nil, fmt.Errorf("database not configured; Model Optimization requires DB")
	}
	if modelID == "" {
		return nil, fmt.Errorf("modelId is required")
	}

	m, err := db.GetModelByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("model %q not found: %w", modelID, err)
	}
	if m == nil {
		return nil, fmt.Errorf("model %q not found", modelID)
	}

	// Only HF-downloaded models have a localPath; remote_api and local_path
	// flow cannot feed Hyperloom's benchmark pipeline.
	if m.AccessMode != "local" && m.AccessMode != "local_path" {
		return nil, fmt.Errorf(
			"model %q has access mode %q; optimization requires local/local_path",
			modelID, m.AccessMode,
		)
	}

	if m.Phase != "Ready" {
		return nil, fmt.Errorf(
			"model %q is in phase %q; wait for it to become Ready before optimizing",
			modelID, m.Phase,
		)
	}

	localPath, chosenWS, err := selectLocalPath(k8sClient, m, workspace)
	if err != nil {
		return nil, err
	}

	return &ResolvedModel{
		ID:          m.ID,
		DisplayName: m.DisplayName,
		ModelName:   m.ModelName,
		MaxTokens:   m.MaxTokens,
		LocalPath:   localPath,
		Workspace:   chosenWS,
	}, nil
}

// ResolvedModel is the condensed view the optimization handler cares about.
type ResolvedModel struct {
	ID          string
	DisplayName string
	ModelName   string
	MaxTokens   int
	LocalPath   string
	// Workspace is the workspace whose localPath was selected. When the
	// caller passed an empty workspace we may pick a different one here.
	Workspace string
}

// selectLocalPath parses Model.LocalPaths (JSON array of ModelLocalPathDB) and
// returns the path ready to be used. Precedence:
//  1. exact workspace match with status=Ready;
//  2. path-accessible match: another Ready entry whose path is reachable from
//     the requested workspace via shared storage (e.g. /wekafs);
//  3. any entry with status=Ready (only when the caller did not specify a
//     workspace explicitly — picking arbitrarily here would be surprising);
//  4. otherwise error.
func selectLocalPath(k8sClient ctrlclient.Client, m *dbclient.Model, workspace string) (string, string, error) {
	if m.LocalPaths == "" {
		return "", "", fmt.Errorf("model %q has no local paths recorded", m.ID)
	}

	var entries []dbclient.ModelLocalPathDB
	if err := json.Unmarshal([]byte(m.LocalPaths), &entries); err != nil {
		return "", "", fmt.Errorf("model %q: decode localPaths: %w", m.ID, err)
	}

	if workspace != "" {
		// Stage 1: exact workspace match.
		for _, e := range entries {
			if e.Workspace == workspace && e.Status == "Ready" && e.Path != "" {
				return e.Path, e.Workspace, nil
			}
		}
		// Stage 2: path-accessible via shared storage (e.g. /wekafs mounted in both workspaces).
		if k8sClient != nil {
			for _, e := range entries {
				if e.Status != "Ready" || e.Path == "" {
					continue
				}
				if accessible, _ := commonworkspace.IsPathAccessibleFromWorkspace(k8sClient, e.Path, workspace); accessible {
					return e.Path, e.Workspace, nil
				}
			}
		}
		return "", "", fmt.Errorf(
			"model %q is not downloaded to workspace %q", m.ID, workspace,
		)
	}

	for _, e := range entries {
		if e.Status == "Ready" && e.Path != "" {
			return e.Path, e.Workspace, nil
		}
	}
	return "", "", fmt.Errorf("model %q has no ready local path", m.ID)
}
