/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
)

func TestSearchHitsFillAGap(t *testing.T) {
	in := newTestIngestion(100)
	feed(in, stageLine("a1", "train", "running", ""), metricLine("a1", 1, 1, `{"train/loss":1}`),
		metricLine("a1", 3, 3, `{"train/loss":0.8}`))
	require.Len(t, in.cur.Attempts["a1"].Gaps, 1)

	resp := &commonsearch.OpenSearchLogResponse{}
	resp.Hits.Hits = make([]commonsearch.OpenSearchLogDoc, 2)
	resp.Hits.Hits[0].Source.Log = metricLine("a1", 2, 2, `{"train/loss":0.9}`)
	resp.Hits.Hits[1].Source.Message = strings.Replace(metricLine("a1", 2, 2, `{}`), testRun, "pt-other", 1)
	ingestSearchHits(resp, in)
	require.Empty(t, in.cur.Attempts["a1"].Gaps, "the searched line fills the gap")
	require.Equal(t, []string{"train/loss@1=1", "train/loss@2=0.9", "train/loss@3=0.8"}, pointKeys(in.finish()))
}
