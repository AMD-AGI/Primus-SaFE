/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cicdlog

import (
	"fmt"
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func BuildSearchBody(query *Query, workloadId string) []byte {
	return jsonutils.MarshalSilently(buildSearchRequest(query, workloadId))
}

func buildSearchRequest(query *Query, workloadId string) *commonsearch.OpenSearchRequest {
	req := &commonsearch.OpenSearchRequest{
		From: query.Offset,
		Size: query.Limit,
	}
	req.Sort = []commonsearch.OpenSearchField{{
		commonsearch.TimeField: map[string]interface{}{
			"order": query.Order,
		}},
	}
	req.Query.Bool.Must = []commonsearch.OpenSearchField{{
		"range": map[string]interface{}{
			commonsearch.TimeField: map[string]string{
				"gte": query.SinceTime.Format(timeutil.TimeRFC3339Milli),
				"lte": query.UntilTime.Format(timeutil.TimeRFC3339Milli),
			},
		},
	}}
	BuildFilter(req, query)
	BuildKeywords(req, query)
	BuildOutput(req, query, workloadId)
	return req
}

func BuildFilter(req *commonsearch.OpenSearchRequest, query *Query) {
	BuildSingleTermFilter(req, query.TermFilters, query.UseK8sLabel, false)
	BuildSingleTermFilter(req, query.PrefixFilters, query.UseK8sLabel, true)
	if query.PodNames != "" {
		BuildMultiTermsFilter(req, "pod_name", query.PodNames)
	} else if query.NodeNames != "" {
		BuildMultiTermsFilter(req, "host", query.NodeNames)
	}
}

func BuildSingleTermFilter(req *commonsearch.OpenSearchRequest, filters map[string]string, isK8sLabel, isPrefixMatch bool) {
	for key, val := range filters {
		val = strings.TrimSpace(val)
		if key == "" || val == "" {
			continue
		}
		if isK8sLabel {
			key = strings.ReplaceAll(key, ".", "_")
			key = "kubernetes.labels." + key
		}
		filterType := ""
		if isPrefixMatch {
			filterType = "prefix"
		} else {
			filterType = "term"
		}
		req.Query.Bool.Filter = append(req.Query.Bool.Filter, commonsearch.OpenSearchField{
			filterType: map[string]interface{}{
				key + ".keyword": val,
			},
		})
	}
}

func BuildMultiTermsFilter(req *commonsearch.OpenSearchRequest, key, values string) {
	valueList := stringutil.Split(values, ",")
	if len(valueList) == 0 {
		return
	}
	var queries []map[string]interface{}
	termKey := fmt.Sprintf("kubernetes.%s.keyword", key)
	for _, val := range valueList {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		queries = append(queries, map[string]interface{}{
			"term": map[string]string{termKey: val},
		})
	}
	req.Query.Bool.Must = append(req.Query.Bool.Must, commonsearch.OpenSearchField{
		"bool": map[string]interface{}{
			"should": queries,
		},
	})
}

func BuildKeywords(req *commonsearch.OpenSearchRequest, query *Query) {
	for _, key := range query.Keywords {
		words := stringutil.Split(key, " ")
		if len(words) == 0 {
			continue
		}
		var phrase string
		var slop int
		if len(words) == 1 {
			phrase = Normalize(words[0])
			slop = 0
		} else {
			normalized := make([]string, len(words))
			for i, w := range words {
				normalized[i] = Normalize(w)
			}
			phrase = strings.Join(normalized, " ")
			slop = 100
		}
		req.Query.Bool.Must = append(req.Query.Bool.Must, KeywordMatchAnyField(phrase, slop))
	}
}

func KeywordMatchAnyField(phrase string, slop int) commonsearch.OpenSearchField {
	matchOn := func(field string) commonsearch.OpenSearchField {
		return commonsearch.OpenSearchField{
			"match_phrase": map[string]interface{}{
				field: map[string]interface{}{
					"query": phrase,
					"slop":  slop,
				},
			},
		}
	}
	return commonsearch.OpenSearchField{
		"bool": map[string]interface{}{
			"should": []commonsearch.OpenSearchField{
				matchOn(commonsearch.MessageField),
				matchOn(commonsearch.LogField),
			},
			"minimum_should_match": 1,
		},
	}
}

func Normalize(str string) string {
	return strings.ToLower(str)
}

func BuildOutput(req *commonsearch.OpenSearchRequest, query *Query, workloadId string) {
	if query.DisableOutput {
		return
	}
	req.Source = []string{
		commonsearch.TimeField, commonsearch.MessageField, commonsearch.LogField,
	}
	if !query.UseK8sLabel {
		return
	}
	req.Source = append(req.Source, "kubernetes.host")
	if workloadId != "" {
		req.Source = append(req.Source, commonsearch.StreamField)
		key := strings.ReplaceAll(v1.WorkloadDispatchCntLabel, ".", "_")
		req.Source = append(req.Source, fmt.Sprintf("kubernetes.labels.%s", key))
	}
	if query.PodNames == "" || strings.Contains(query.PodNames, ",") {
		req.Source = append(req.Source, "kubernetes.pod_name")
	}
}
