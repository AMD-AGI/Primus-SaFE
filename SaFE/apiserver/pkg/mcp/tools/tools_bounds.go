// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"encoding/json"
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// An MCP client is usually a language model, and whatever a tool returns lands in its
// context. The tools that read logs and events therefore bound what they ask for and what
// they hand back; the numbers below are the whole policy.
const (
	// maxLogLimit caps how many log lines or events one call may ask for.
	maxLogLimit = 200
	// defaultLogLimit and defaultEventLimit apply when the caller gives no limit.
	defaultLogLimit   = 100
	defaultEventLimit = 50
	// maxTailLines caps tail_lines of node_pod_logs; defaultTailLines applies without one.
	maxTailLines     = 1000
	defaultTailLines = 200
	// maxFieldChars caps one log message or one text field of an event.
	maxFieldChars = 1000
	// maxResultBytes caps what a bounded tool returns, measured as the server encodes it into
	// the text a client receives.
	maxResultBytes = 64 * 1024

	truncatedMark = "...[truncated]"
)

// clampedInt reads p[key] as a count: def when it is absent, not a number or not positive,
// and at most max.
func clampedInt(p map[string]any, key string, def, max int) int {
	n, ok := getInt(p, key)
	if !ok || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// cutText shortens s to max characters and says whether it did.
func cutText(s string, max int) (string, bool) {
	if len(s) <= max { // bytes >= characters, so this is a cheap exit
		return s, false
	}
	r := []rune(s)
	if len(r) <= max {
		return s, false
	}
	return string(r[:max]) + truncatedMark, true
}

// cutStrings returns v with every string in it shortened to max characters.
func cutStrings(v any, max int) (any, bool) {
	switch t := v.(type) {
	case string:
		return cutText(t, max)
	case map[string]any:
		cut := false
		out := make(map[string]any, len(t))
		for k, e := range t {
			ne, c := cutStrings(e, max)
			out[k] = ne
			cut = cut || c
		}
		return out, cut
	case []any:
		cut := false
		out := make([]any, len(t))
		for i, e := range t {
			ne, c := cutStrings(e, max)
			out[i] = ne
			cut = cut || c
		}
		return out, cut
	default:
		return v, false
	}
}

// encodedSize is the length of v as the MCP server turns a tool result into text
// (NewJSONContent: indented JSON, with the HTML-safe escaping of encoding/json).
func encodedSize(v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return maxResultBytes + 1 // unmeasurable counts as too big
	}
	return len(b)
}

// fitCount returns the largest k in [0, n] for which build(k) stays within maxResultBytes.
// build(k) must grow with k.
func fitCount(n int, build func(k int) any) int {
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if encodedSize(build(mid)) <= maxResultBytes {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// searchHits digs hits.hits out of an OpenSearch reply.
func searchHits(out any) (reply, hits map[string]any, list []any, ok bool) {
	reply, ok = out.(map[string]any)
	if !ok {
		return nil, nil, nil, false
	}
	hits, ok = reply["hits"].(map[string]any)
	if !ok {
		return nil, nil, nil, false
	}
	list, ok = hits["hits"].([]any)
	return reply, hits, list, ok
}

// hitsTotal returns hits.total.value and whether it is only a lower bound: a search that
// matches more than OpenSearch is willing to count reports relation "gte".
func hitsTotal(hits map[string]any) (value any, lowerBound bool) {
	total, ok := hits["total"].(map[string]any)
	if !ok {
		return nil, false
	}
	return total["value"], total["relation"] == "gte"
}

// moreMatches says whether the search matched entries beyond the ones this reply holds,
// counting from offset. A reply without a usable total says no. A lower-bound total says
// yes: the real count is at least that, and nothing says it is what came back.
func moreMatches(total any, lowerBound bool, offset, held int) bool {
	var n float64
	switch t := total.(type) {
	case float64:
		n = t
	case int:
		n = float64(t)
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return false
		}
		n = f
	default:
		return false
	}
	return lowerBound || float64(offset+held) < n
}

func textField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// dispatchCountKey is the dispatch-count label as the log search stores it under
// kubernetes.labels: with its dots written as underscores.
var dispatchCountKey = strings.ReplaceAll(v1.WorkloadDispatchCntLabel, ".", "_")

// compactLogReply turns the search reply for workload logs into one small record per line:
// time, message, and, when the search returned them, pod, node, stream and dispatch_count
// (the run of a retried workload that printed it). The reply repeats a block of Kubernetes metadata on
// every hit, which is most of its size and of no use to a reader. Lines come out in the
// order the search returned them, and as many of them as fit the size budget. truncated is
// true when the reply does not hold every matching line from offset on: lines were left
// out, a message was cut, or the search matched more than it returned. Any other reply is
// returned as it is.
func compactLogReply(out any, offset int) any {
	_, hits, list, ok := searchHits(out)
	if !ok {
		return out
	}
	total, lowerBound := hitsTotal(hits)
	more := moreMatches(total, lowerBound, offset, len(list))
	lines := make([]any, 0, len(list))
	cut := false
	for _, h := range list {
		hit, _ := h.(map[string]any)
		src, _ := hit["_source"].(map[string]any)
		msg := textField(src, "message")
		if msg == "" {
			msg = textField(src, "log")
		}
		msg, c := cutText(msg, maxFieldChars)
		cut = cut || c
		line := map[string]any{"time": textField(src, "@timestamp"), "message": msg}
		if v := textField(src, "stream"); v != "" {
			line["stream"] = v
		}
		if k8s, ok := src["kubernetes"].(map[string]any); ok {
			if v := textField(k8s, "pod_name"); v != "" {
				line["pod"] = v
			}
			if v := textField(k8s, "host"); v != "" {
				line["node"] = v
			}
			if labels, ok := k8s["labels"].(map[string]any); ok {
				if v := textField(labels, dispatchCountKey); v != "" {
					line["dispatch_count"] = v
				}
			}
		}
		lines = append(lines, line)
	}
	build := func(k int) any {
		r := map[string]any{"total": total, "returned": k, "truncated": cut || more || k < len(lines), "lines": lines[:k]}
		if lowerBound {
			r["totalIsLowerBound"] = true
		}
		return r
	}
	return build(fitCount(len(lines), build))
}

// boundEventReply keeps the shape of the search reply for workload events (the fields of an
// event differ from cluster to cluster, so they are not picked apart) but shortens long
// text and drops the hits that do not fit the size budget. It adds returned and truncated
// to the reply, with the meaning they have for compactLogReply.
func boundEventReply(out any, offset int) any {
	reply, hits, list, ok := searchHits(out)
	if !ok {
		return out
	}
	total, lowerBound := hitsTotal(hits)
	more := moreMatches(total, lowerBound, offset, len(list))
	kept := make([]any, 0, len(list))
	cut := false
	for _, h := range list {
		hit, _ := h.(map[string]any)
		src, c := cutStrings(hit["_source"], maxFieldChars)
		cut = cut || c
		kept = append(kept, map[string]any{"_id": hit["_id"], "_source": src})
	}
	build := func(k int) any {
		r := make(map[string]any, len(reply)+2)
		for key, v := range reply {
			r[key] = v
		}
		h := make(map[string]any, len(hits))
		for key, v := range hits {
			h[key] = v
		}
		h["hits"] = kept[:k]
		r["hits"] = h
		r["returned"] = k
		r["truncated"] = cut || more || k < len(kept)
		return r
	}
	return build(fitCount(len(kept), build))
}

// boundNodeLogs keeps the newest lines of a node pod log reply, in log order: at most tail
// of them, each cut to maxFieldChars, as many as fit the size budget. truncated is true
// when lines were left out or one was cut. Any other reply is returned as it is.
func boundNodeLogs(out any, tail int) any {
	reply, ok := out.(map[string]any)
	if !ok {
		return out
	}
	logs, ok := reply["logs"].([]any)
	if !ok {
		return out
	}
	// The backend splits the log text on newlines, so a log that ends with one, as
	// Kubernetes logs do, yields an empty last element that is not a line.
	if n := len(logs); n > 0 && logs[n-1] == "" {
		logs = logs[:n-1]
	}
	start := 0
	if len(logs) > tail {
		start = len(logs) - tail
	}
	window := make([]any, 0, len(logs)-start)
	cut := false
	for _, l := range logs[start:] {
		s, _ := l.(string)
		s, c := cutText(s, maxFieldChars)
		cut = cut || c
		window = append(window, s)
	}
	build := func(k int) any {
		r := make(map[string]any, len(reply)+1)
		for key, v := range reply {
			r[key] = v
		}
		r["logs"] = window[len(window)-k:]
		r["truncated"] = cut || start > 0 || k < len(window)
		return r
	}
	return build(fitCount(len(window), build))
}
