/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func (h *Handler) ListWorkloadLog(c *gin.Context) {
	handle(c, h.listWorkloadLog)
}

func (h *Handler) ListServiceLog(c *gin.Context) {
	handle(c, h.listServiceLog)
}

func (h *Handler) ListWorkloadLogContext(c *gin.Context) {
	handle(c, h.listWorkloadLogContext)
}

func (h *Handler) listWorkloadLog(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsLogEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}

	query, _, err := h.parseWorkloadLogQuery(c, name)
	if err != nil {
		return nil, err
	}
	return h.searchLog(query, name)
}

func (h *Handler) listServiceLog(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsLogEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}

	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the service name is empty")
	}
	query, err := parseSearchLogQuery(c.Request, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	query.DispatchCount = 0
	query.Filters = map[string]string{
		"app": name,
	}
	return h.searchLog(query, "")
}

func (h *Handler) listWorkloadLogContext(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsLogEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	startTime := time.Now().UTC()
	logId := c.Param(types.LogId)
	if logId == "" {
		return nil, commonerrors.NewBadRequest("the logId parameter is empty")
	}
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}

	query, startTime, err := h.parseWorkloadLogQuery(c, name)
	if err != nil {
		klog.ErrorS(err, "failed to parse workload log query")
		return nil, err
	}
	limit := query.Limit
	queryWrappers, err := genListContextQuery(query, startTime)
	if err != nil {
		klog.ErrorS(err, "failed to build query for context search")
		return nil, err
	}

	const count = 2
	ch := make(chan types.GetLogRequestWrapper, count)
	for i := range queryWrappers {
		ch <- queryWrappers[i]
	}
	var response [count]commonsearch.OpenSearchResponse
	_, err = concurrent.Exec(count, func() error {
		wrapper := <-ch
		resp, err := h.searchLog(wrapper.Query, name)
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

	result := &commonsearch.OpenSearchResponse{
		Took: time.Since(startTime).Milliseconds(),
	}
	addContextDoc(result, &response[0], logId, true, limit)
	addContextDoc(result, &response[1], logId, false, limit)
	return result, nil
}

func (h *Handler) parseWorkloadLogQuery(c *gin.Context, name string) (*types.GetLogRequest, time.Time, error) {
	startTime, endTime, err := h.getWorkloadStartEndTime(c.Request.Context(), name)
	if err != nil {
		return nil, time.Time{}, err
	}

	query, err := parseSearchLogQuery(c.Request, startTime, endTime)
	if err != nil {
		klog.ErrorS(err, "failed to parse log query")
		return nil, startTime, err
	}
	query.Filters = map[string]string{
		v1.WorkloadIdLabel: name,
	}
	if query.DispatchCount > 0 {
		query.Filters[v1.WorkloadDispatchCntLabel] = strconv.Itoa(query.DispatchCount)
	}
	return query, startTime, nil
}

func (h *Handler) getWorkloadStartEndTime(ctx context.Context, workloadId string) (time.Time, time.Time, error) {
	if commonconfig.IsDBEnable() {
		workload, err := h.dbClient.GetWorkload(ctx, workloadId)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		startTime := dbutils.ParseNullTime(workload.CreateTime)
		endTime := dbutils.ParseNullTime(workload.EndTime)
		return startTime, endTime, nil
	} else {
		adminWorkload, err := h.getAdminWorkload(ctx, workloadId)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return adminWorkload.CreationTimestamp.Time, adminWorkload.EndTime(), nil
	}
}

func (h *Handler) searchLog(query *types.GetLogRequest, workloadId string) ([]byte, error) {
	body := buildSearchBody(query, workloadId)
	return h.searchClient.RequestByTimeRange(
		query.SinceTime, query.UntilTime, "/_search", http.MethodPost, body)
}

func buildSearchBody(query *types.GetLogRequest, workloadId string) []byte {
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

func buildFilter(req *commonsearch.OpenSearchRequest, query *types.GetLogRequest) {
	buildLabelFilter(req, query.Filters)
	buildNodeFilter(req, query)
	if query.PodName != "" {
		req.Query.Bool.Filter = append(req.Query.Bool.Filter, commonsearch.OpenSearchField{
			"term": map[string]interface{}{
				"kubernetes.pod_name.keyword": query.PodName,
			},
		})
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

func buildNodeFilter(req *commonsearch.OpenSearchRequest, query *types.GetLogRequest) {
	nodeNames := split(query.NodeNames, ",")
	if len(nodeNames) == 0 {
		return
	}
	var nodes []map[string]interface{}
	for _, name := range nodeNames {
		nodes = append(nodes, map[string]interface{}{
			"term": map[string]string{
				"kubernetes.host.keyword": name,
			},
		})
	}
	req.Query.Bool.Must = append(req.Query.Bool.Must, commonsearch.OpenSearchField{
		"bool": map[string]interface{}{
			"should": nodes,
		},
	})
}

func buildKeywords(req *commonsearch.OpenSearchRequest, query *types.GetLogRequest) {
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

func buildOutput(req *commonsearch.OpenSearchRequest, query *types.GetLogRequest, workloadId string) {
	req.Source = []string{
		commonsearch.TimeField, commonsearch.MessageField, "kubernetes.host",
	}
	if workloadId != "" {
		req.Source = append(req.Source, commonsearch.StreamField)
		key := strings.ReplaceAll(v1.WorkloadDispatchCntLabel, ".", "_")
		req.Source = append(req.Source, fmt.Sprintf("kubernetes.labels.%s", key))
	}
	if query.PodName == "" || strings.Contains(query.PodName, ",") {
		req.Source = append(req.Source, "kubernetes.pod_name")
	}
}

func genListContextQuery(query *types.GetLogRequest, startTime time.Time) ([]types.GetLogRequestWrapper, error) {
	if query.Since == "" {
		return nil, commonerrors.NewBadRequest("the since parameter is empty")
	}

	result := make([]types.GetLogRequestWrapper, 0, 2)
	// Query with a higher limit to ensure the specified logId is among the results
	query.Limit += 100
	// context search should disable keywords search
	query.Offset = 0
	query.Keywords = nil

	query2 := new(types.GetLogRequest)
	*query2 = *query
	query.Order = dbclient.ASC
	result = append(result, types.GetLogRequestWrapper{
		Query: query,
		Id:    0,
	})

	query2.Order = dbclient.DESC
	query2.UntilTime = query.SinceTime
	query2.SinceTime = startTime

	result = append(result, types.GetLogRequestWrapper{
		Query: query2,
		Id:    1,
	})
	return result, nil
}

func addContextDoc(result *commonsearch.OpenSearchResponse,
	response *commonsearch.OpenSearchResponse, logId string, isAsc bool, limit int) {
	id := 0
	for i := range response.Hits.Hits {
		if response.Hits.Hits[i].Id == logId {
			id = i + 1
			break
		}
	}

	count := 0
	for ; id < len(response.Hits.Hits) && count < limit; id++ {
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

func parseSearchLogQuery(req *http.Request, beginTime, endTime time.Time) (*types.GetLogRequest, error) {
	query := &types.GetLogRequest{}
	_, err := getBodyFromRequest(req, query)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.Limit <= 0 {
		query.Limit = 100
	} else if query.Limit > 10000 {
		query.Limit = 10000
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
