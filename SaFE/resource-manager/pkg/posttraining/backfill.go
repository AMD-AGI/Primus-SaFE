/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"encoding/json"
	"time"

	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/cicdlog"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
)

const (
	// backfillInterval spaces the log searches for a gap; logs reach the search with a delay, so a gap that
	// is not found yet is searched again.
	backfillInterval = 5 * time.Minute
	backfillMargin   = time.Minute
	backfillLimit    = 10000
)

// backfillFromLogSearch reads the [METRIC] lines of the submitter around the run's gaps from the cluster's log
// search, which keeps the whole log however often the kubelet rotates it. It does nothing without a log
// search, without gaps, or within backfillInterval of the previous search.
func (r *RunReconciler) backfillFromLogSearch(ctx context.Context, w *v1.Workload, cur *logCursor, in *ingestion) {
	now := in.u.now
	since, until, ok := cur.gapWindow(now)
	if !ok || cur.Pod == "" || now.Sub(time.Unix(cur.Backfilled, 0)) < backfillInterval || !commonconfig.IsOpenSearchEnable() {
		return
	}
	search := commonsearch.GetOpensearchClient(v1.GetClusterId(w))
	if search == nil {
		return
	}
	cur.Backfilled = now.Unix()
	query := cicdlog.Query{
		ListLogInput: cicdlog.ListLogInput{Keywords: []string{markerMetric}, PodNames: cur.Pod, Limit: backfillLimit, Order: "asc"},
		SinceTime:    since.Add(-backfillMargin),
		UntilTime:    until.Add(backfillMargin),
	}
	raw, err := search.SearchByTimeRangeContext(ctx, query.SinceTime, query.UntilTime, "", "/_search",
		cicdlog.BuildSearchBody(&query, w.Name))
	if err != nil {
		klog.V(2).InfoS("the log search for a metric gap failed", "run", in.u.run.RunID, "err", err)
		return
	}
	var resp commonsearch.OpenSearchLogResponse
	if err = json.Unmarshal(raw, &resp); err != nil {
		klog.V(2).InfoS("the log search for a metric gap returned an unreadable answer", "run", in.u.run.RunID)
		return
	}
	ingestSearchHits(&resp, in)
}

// ingestSearchHits applies the [METRIC] markers found by a log search.
func ingestSearchHits(resp *commonsearch.OpenSearchLogResponse, in *ingestion) {
	for i := range resp.Hits.Hits {
		if m, ok := parseMarker(resp.Hits.Hits[i].EffectiveMessage(), in.u.run.RunID); ok && m.Kind == markerMetric {
			in.add(m)
		}
	}
}
