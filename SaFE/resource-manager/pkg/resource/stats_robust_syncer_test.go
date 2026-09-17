/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

type fakeStatsDBWriter struct{}

func (fakeStatsDBWriter) UpsertWorkloadStatistic(_ context.Context, _ string, _ []WorkloadHourlyStats) error {
	return nil
}
func (fakeStatsDBWriter) UpsertNodeStatistic(_ context.Context, _ string, _ []NodeStats) error {
	return nil
}

func TestSetupStatsRobustSyncerNil(t *testing.T) {
	// Nil deps -> skip.
	assert.NoError(t, SetupStatsRobustSyncer(nil, nil, nil))
}

func TestStatsSyncNoClusters(t *testing.T) {
	s := &StatsRobustSyncer{
		robustClient: robustclient.NewClient(robustclient.ClientConfig{}),
		dbWriter:     fakeStatsDBWriter{},
	}
	// No clusters registered -> loops do nothing.
	s.syncWorkloadStats(context.Background())
	s.syncNodeStats(context.Background())
}
