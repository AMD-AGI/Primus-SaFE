/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import "strings"

// isSharedNfsPath identifies OCI-style shared storage paths used by AINIC jobs.
func isSharedNfsPath(path string) bool {
	return strings.HasPrefix(path, "/shared_nfs")
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
