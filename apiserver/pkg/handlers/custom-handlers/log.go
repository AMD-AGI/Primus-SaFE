/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func (h *Handler) GetWorkloadLog(c *gin.Context) {
	handle(c, h.getWorkloadLog)
}

func (h *Handler) GetServiceLog(c *gin.Context) {
	handle(c, h.getServiceLog)
}

func (h *Handler) GetWorkloadLogContext(c *gin.Context) {
	handle(c, h.getWorkloadLogContext)
}

func (h *Handler) getWorkloadLog(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}
	workload, err := h.getWorkloadForAuth(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:    c.Request.Context(),
		Resource:   workload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workload.Spec.Workspace},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	query, err := parseWorkloadLogQuery(c, workload)
	if err != nil {
		return nil, err
	}
	return h.searchClient.SearchByTimeRange(query.SinceTime, query.UntilTime,
		"/_search", buildSearchBody(query, name))
}

func (h *Handler) getServiceLog(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	if err := h.auth.AuthorizeSystemAdmin(authority.Input{
		Context: c.Request.Context(),
		UserId:  c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	query, err := parseServiceLogQuery(c)
	if err != nil {
		return nil, err
	}
	return h.searchClient.SearchByTimeRange(query.SinceTime, query.UntilTime,
		"/_search", buildSearchBody(query, ""))
}

func (h *Handler) getWorkloadLogContext(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}
	workload, err := h.getWorkloadForAuth(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:    c.Request.Context(),
		Resource:   workload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workload.Spec.Workspace},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	queries, err := parseContextQuery(c, workload)
	if err != nil {
		return nil, err
	}
	return h.searchContextLog(queries, name)
}

func (h *Handler) searchContextLog(queries []types.ListContextLogRequest, workloadId string) (*commonsearch.OpenSearchResponse, error) {
	startTime := time.Now().UTC()
	const count = 2
	ch := make(chan types.ListContextLogRequest, count)
	for i := range queries {
		ch <- queries[i]
	}

	var response [count]commonsearch.OpenSearchResponse
	_, err := concurrent.Exec(count, func() error {
		wrapper := <-ch
		query := wrapper.Query
		resp, err := h.searchClient.SearchByTimeRange(query.SinceTime, query.UntilTime,
			"/_search", buildSearchBody(query, workloadId))
		if err != nil {
			return err
		}
		if err = json.Unmarshal(resp, &response[wrapper.Id]); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := &commonsearch.OpenSearchResponse{}
	addContextDoc(result, queries[0], &response[0], true)
	addContextDoc(result, queries[1], &response[1], false)
	result.Took = time.Since(startTime).Milliseconds()
	return result, nil
}

func buildSearchBody(query *types.ListLogRequest, workloadId string) []byte {
	req := &commonsearch.OpenSearchRequest{
		From: query.Offset,
		Size: query.Limit,
		Sort: []commonsearch.OpenSearchField{{
			commonsearch.TimeField: map[string]interface{}{
				"order": query.Order,
			}},
		},
	}

	req.Query.Bool.Must = []commonsearch.OpenSearchField{{
		"range": map[string]interface{}{
			commonsearch.TimeField: map[string]string{
				"gte": query.SinceTime.Format(timeutil.TimeRFC3339Milli),
				"lte": query.UntilTime.Format(timeutil.TimeRFC3339Milli),
			},
		},
	}}

	buildFilter(req, query)
	buildKeywords(req, query)
	buildOutput(req, query, workloadId)
	return jsonutils.MarshalSilently(req)
}

func buildFilter(req *commonsearch.OpenSearchRequest, query *types.ListLogRequest) {
	buildLabelFilter(req, query.Filters)
	if query.PodNames != "" {
		buildMultiTermsFilter(req, "pod_name", query.PodNames)
	} else if query.NodeNames != "" {
		buildMultiTermsFilter(req, "host", query.NodeNames)
	}
}

func buildLabelFilter(req *commonsearch.OpenSearchRequest, labelFilters map[string]string) {
	// including workload id/service name/dispatch count
	for key, val := range labelFilters {
		if key == "" || val == "" {
			continue
		}
		// Use the same punctuation handling rules as OpenSearch.
		key = strings.ReplaceAll(key, ".", "_")
		req.Query.Bool.Filter = append(req.Query.Bool.Filter, commonsearch.OpenSearchField{
			"term": map[string]interface{}{
				"kubernetes.labels." + key + ".keyword": val,
			},
		})
	}
}

func buildMultiTermsFilter(req *commonsearch.OpenSearchRequest, key, values string) {
	valueList := split(values, ",")
	if len(valueList) == 0 {
		return
	}
	var queries []map[string]interface{}
	for _, val := range valueList {
		queries = append(queries, map[string]interface{}{
			"term": map[string]string{
				fmt.Sprintf("kubernetes.%s.keyword", key): val,
			},
		})
	}
	req.Query.Bool.Must = append(req.Query.Bool.Must, commonsearch.OpenSearchField{
		"bool": map[string]interface{}{
			"should": queries,
		},
	})
}

func buildKeywords(req *commonsearch.OpenSearchRequest, query *types.ListLogRequest) {
	// and search
	for _, key := range query.Keywords {
		words := split(key, " ")
		if len(words) == 0 {
			continue
		}
		if len(words) == 1 {
			req.Query.Bool.Must = append(req.Query.Bool.Must, commonsearch.OpenSearchField{
				"term": map[string]interface{}{
					commonsearch.MessageField: normalize(words[0]),
				},
			})
		} else {
			spanNearQuery := commonsearch.OpenSearchSpanNearQuery{
				Slop:    0,
				InOrder: true,
			}
			for _, word := range words {
				spanNearQuery.Clauses = append(spanNearQuery.Clauses, commonsearch.OpenSearchField{
					"span_term": map[string]interface{}{
						commonsearch.MessageField: normalize(word),
					},
				})
			}
			req.Query.Bool.Must = append(req.Query.Bool.Must, commonsearch.OpenSearchField{
				"span_near": spanNearQuery,
			})
		}
	}
}

func normalize(str string) string {
	str = strings.ToLower(str)
	punctuation := ",.!?;:/"
	str = strings.Trim(str, punctuation)
	return str
}

func buildOutput(req *commonsearch.OpenSearchRequest, query *types.ListLogRequest, workloadId string) {
	req.Source = []string{
		commonsearch.TimeField, commonsearch.MessageField, "kubernetes.host",
	}
	if workloadId != "" {
		req.Source = append(req.Source, commonsearch.StreamField)
		key := strings.ReplaceAll(v1.WorkloadDispatchCntLabel, ".", "_")
		req.Source = append(req.Source, fmt.Sprintf("kubernetes.labels.%s", key))
	}
	if query.PodNames == "" || strings.Contains(query.PodNames, ",") {
		req.Source = append(req.Source, "kubernetes.pod_name")
	}
}

func parseWorkloadLogQuery(c *gin.Context, workload *v1.Workload) (*types.ListLogRequest, error) {
	query, err := parseLogQuery(c.Request, workload.CreationTimestamp.Time, workload.EndTime())
	if err != nil {
		klog.ErrorS(err, "failed to parse log query")
		return nil, err
	}
	query.Filters = map[string]string{
		v1.WorkloadIdLabel: workload.Name,
	}
	if query.DispatchCount > 0 {
		query.Filters[v1.WorkloadDispatchCntLabel] = strconv.Itoa(query.DispatchCount)
	}
	return query, nil
}

func parseServiceLogQuery(c *gin.Context) (*types.ListLogRequest, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the service name is empty")
	}

	query, err := parseLogQuery(c.Request, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	query.DispatchCount = 0
	query.Filters = map[string]string{
		"app": name,
	}
	return query, nil
}

func parseContextQuery(c *gin.Context, workload *v1.Workload) ([]types.ListContextLogRequest, error) {
	docId := c.Param(types.DocId)
	if docId == "" {
		return nil, commonerrors.NewBadRequest("the docId parameter is empty")
	}
	query, err := parseWorkloadLogQuery(c, workload)
	if err != nil {
		klog.ErrorS(err, "failed to parse workload log query")
		return nil, err
	}
	// "since" is a required field for context queries.
	if query.Since == "" {
		return nil, commonerrors.NewBadRequest("the since parameter is empty")
	}

	limit := query.Limit
	result := make([]types.ListContextLogRequest, 0, 2)
	// Query with a higher limit to ensure the specified logId is among the results
	query.Limit += 100
	// context search should disable keywords search
	query.Offset = 0
	query.Keywords = nil

	query2 := new(types.ListLogRequest)
	*query2 = *query
	query.Order = dbclient.ASC
	result = append(result, types.ListContextLogRequest{
		Query: query,
		Id:    0,
		Limit: limit,
		DocId: docId,
	})

	query2.Order = dbclient.DESC
	query2.UntilTime = query.SinceTime
	query2.SinceTime = workload.CreationTimestamp.Time
	result = append(result, types.ListContextLogRequest{
		Query: query2,
		Id:    1,
		Limit: limit,
		DocId: docId,
	})
	return result, nil
}

func parseLogQuery(req *http.Request, beginTime, endTime time.Time) (*types.ListLogRequest, error) {
	query := &types.ListLogRequest{}
	_, err := getBodyFromRequest(req, query)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}

	if query.Offset < 0 || query.Limit < 0 {
		return nil, commonerrors.NewBadRequest("invalid query offset or limit")
	}
	if query.Offset >= commonsearch.MaxDocsPerQuery {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf(
			"the maximum offset of log requested cannot exceed %d", commonsearch.MaxDocsPerQuery))
	}
	if query.Limit == 0 {
		query.Limit = 100
	}
	if query.Limit+query.Offset > commonsearch.MaxDocsPerQuery {
		query.Limit = commonsearch.MaxDocsPerQuery - query.Offset
	}

	if query.Order == "" {
		query.Order = dbclient.ASC
	} else if query.Order != dbclient.ASC && query.Order != dbclient.DESC {
		return nil, commonerrors.NewBadRequest(
			fmt.Sprintf("the order parameter only supports %s and %s", dbclient.ASC, dbclient.DESC))
	}

	if query.SinceTime, err = timeutil.CvtStrToRFC3339Milli(query.Since); err != nil {
		return nil, err
	}
	if query.UntilTime, err = timeutil.CvtStrToRFC3339Milli(query.Until); err != nil {
		return nil, err
	}

	if endTime.IsZero() {
		endTime = time.Now().UTC()
	}
	if query.UntilTime.IsZero() || query.UntilTime.After(endTime) {
		query.UntilTime = endTime
	}
	if query.SinceTime.IsZero() {
		if beginTime.IsZero() {
			query.SinceTime = query.UntilTime.Add(-time.Hour * 168).UTC()
		} else {
			query.SinceTime = beginTime
		}
	} else if !beginTime.IsZero() && query.SinceTime.Before(beginTime) {
		query.SinceTime = beginTime
	}
	if query.SinceTime.After(query.UntilTime) {
		return nil, commonerrors.NewBadRequest("the since time is later than until time")
	}
	return query, nil
}

func addContextDoc(result *commonsearch.OpenSearchResponse,
	query types.ListContextLogRequest, response *commonsearch.OpenSearchResponse, isAsc bool) {
	id := 0
	for i := range response.Hits.Hits {
		if response.Hits.Hits[i].Id == query.DocId {
			id = i + 1
			break
		}
	}

	count := 0
	for ; id < len(response.Hits.Hits) && count < query.Limit; id++ {
		doc := &response.Hits.Hits[id]
		if doc.Source.Message == "" {
			continue
		}
		count++
		if isAsc {
			doc.Source.Line = count
		} else {
			doc.Source.Line = -count
		}
		result.Hits.Hits = append(result.Hits.Hits, *doc)
	}
	result.Hits.Total.Value += count
}

func split(str, sep string) []string {
	if len(str) == 0 {
		return nil
	}
	strList := strings.Split(str, sep)
	var result []string
	for _, s := range strList {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		result = append(result, s)
	}
	return result
}
