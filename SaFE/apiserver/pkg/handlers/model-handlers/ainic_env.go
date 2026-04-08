/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// shouldApplyAinicEnv decides whether OCI/AINIC-specific RCCL env should be
// injected for SFT/RL workloads. The decision is derived from the workspace's
// mounted storage instead of a pre-resolved path string.
func shouldApplyAinicEnv(workspace *v1.Workspace) bool {
	if workspace == nil {
		return false
	}

	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.PFS {
			return strings.HasPrefix(vol.MountPath, "/shared_nfs")
		}
	}

	if len(workspace.Spec.Volumes) > 0 {
		return strings.HasPrefix(workspace.Spec.Volumes[0].MountPath, "/shared_nfs")
	}
	return false
}

// applyAinicWorkloadEnv injects the AINIC/RCCL environment values that were
// previously inferred in job-manager. Keeping them in SFT/RL handlers makes
// the OCI-specific behavior explicit to these training flows only.
func applyAinicWorkloadEnv(env map[string]string) {
	env["USING_AINIC"] = "1"
	env["NCCL_IB_GID_INDEX"] = "1"
	env["NCCL_DMABUF_ENABLE"] = "0"
	env["NCCL_MAX_P2P_CHANNELS"] = "56"
	env["NET_OPTIONAL_RECV_COMPLETION"] = "1"
	env["NCCL_IB_USE_INLINE"] = "1"
	env["RCCL_GDR_FLUSH_GPU_MEM_NO_RELAXED_ORDERING"] = "0"
	env["NCCL_GDR_FLUSH_DISABLE"] = "1"
	env["NCCL_IGNORE_CPU_AFFINITY"] = "1"
	env["LD_LIBRARY_PATH"] = "/opt/amd-anp/build:/opt/rccl/build/release:/opt/rocm/lib"
}
