/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workspace

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Failed team lookups are logged once per interval; the others are counted for the next log.
func TestLogTeamLookupErrorIsThrottled(t *testing.T) {
	teamLookupLog.Lock()
	teamLookupLog.last, teamLookupLog.suppressed = time.Time{}, 0
	teamLookupLog.Unlock()
	err := errors.New("index not ready")
	LogTeamLookupError(err, "t1")
	LogTeamLookupError(err, "t2")
	LogTeamLookupError(err, "t3")
	teamLookupLog.Lock()
	require.Equal(t, 2, teamLookupLog.suppressed)
	teamLookupLog.last = time.Now().Add(-teamLookupLogInterval)
	teamLookupLog.Unlock()
	LogTeamLookupError(err, "t4")
	teamLookupLog.Lock()
	defer teamLookupLog.Unlock()
	require.Zero(t, teamLookupLog.suppressed)
}
