// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package mean_time_between_fail

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
)

type MtbfJob struct {
}

func (m *MtbfJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet) error {

	return nil
}

func (m *MtbfJob) Schedule() string {
	return "@every 30s"
}
