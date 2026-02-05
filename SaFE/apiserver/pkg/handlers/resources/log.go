/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

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
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// GetWorkloadLog retrieves logs for a workload from OpenSearch
func (h *Handler) GetWorkloadLog(c *gin.Context) {
	handle(c, h.getWorkloadLog)
}

// GetServiceLog retrieves service logs from the logging backend.
func (h *Handler) GetServiceLog(c *gin.Context) {
	handle(c, h.getServiceLog)
}

// GetWorkloadEvent retrieves events for a workload from OpenSearch
func (h *Handler) GetWorkloadEvent(c *gin.Context) {
	handle(c, h.getWorkloadEvent)
}

// GetWorkloadLogContext retrieves contextual log information for a workload.
func (h *Handler) GetWorkloadLogContext(c *gin.Context) {
	handle(c, h.getWorkloadLogContext)
}

// getWorkloadLog retrieves logs for a specific workload from OpenSearch.
// It will check whether the user has permission to access the logs.
// Returns the search results or an error if any step fails.
func (h *Handler) getWorkloadLog(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	name := c.GetString(common.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}
	workload, err := h.getWorkloadForAuth(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    c.Request.Context(),
		Resource:   workload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workload.Spec.Workspace},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	clusterId := v1.GetClusterId(workload)
	query, err := parseWorkloadLogQuery(c, workload)
	if err != nil {
		return nil, err
	}
	opensearchClient := commonsearch.GetOpensearchClient(clusterId)
	if opensearchClient == nil {
		return nil, commonerrors.NewInternalError("There is no OpenSearch in cluster " + clusterId)
	}
	return opensearchClient.SearchByTimeRange(query.SinceTime, query.UntilTime,
		"", "/_search", buildSearchBody(query, name))
}

// getServiceLog retrieves logs for a specific service from OpenSearch.
// Only system-adminer has permission to access the logs.
// Returns the search results or an error if any step fails.
func (h *Handler) getServiceLog(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	if err := h.accessController.AuthorizeSystemAdmin(authority.AccessInput{
		Context: c.Request.Context(),
		UserId:  c.GetString(common.UserId),
	}, true); err != nil {
		return nil, err
	}
	query, err := parseServiceLogQuery(c)
	if err != nil {
		return nil, err
	}
	opensearchClient := commonsearch.GetOpensearchClient("")
	if opensearchClient == nil {
		return nil, commonerrors.NewInternalError("There is no OpenSearch in cluster " + "")
	}
	return opensearchClient.SearchByTimeRange(query.SinceTime, query.UntilTime,
		"", "/_search", buildSearchBody(query, ""))
}

// getWorkloadEvent retrieves events for a specific workload from OpenSearch.
func (h *Handler) getWorkloadEvent(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	name := c.GetString(common.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}
	workload, err := h.getWorkloadForAuth(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    c.Request.Context(),
		Resource:   workload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workload.Spec.Workspace},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	clusterId := v1.GetClusterId(workload)
	query, err := parseEventLogQuery(c, workload)
	if err != nil {
		return nil, err
	}
	opensearchClient := commonsearch.GetOpensearchClient(clusterId)
	if opensearchClient == nil {
		return nil, commonerrors.NewInternalError("There is no OpenSearch in cluster " + clusterId)
	}
	return opensearchClient.SearchByTimeRange(query.SinceTime, query.UntilTime,
		"k8s-event-", "/_search", buildSearchBody(query, name))
}

// getWorkloadLogContext retrieves contextual logs for a specific workload from OpenSearch.
// It will check whether the user has permission to access the logs.
// Returns the contextual search results or an error if any step fails.
func (h *Handler) getWorkloadLogContext(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	name := c.GetString(common.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}
	workload, err := h.getWorkloadForAuth(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
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

// searchContextLog performs concurrent OpenSearch queries to retrieve contextual logs for a workload.
// It executes two parallel searches (before and after the target log) using the provided queries
// Returns the combined search results or an error if any search fails.
func (h *Handler) searchContextLog(queries []view.ListContextLogRequest, workloadId string) (*commonsearch.OpenSearchResponse, error) {
	startTime := time.Now().UTC()
	const count = 2
	ch := make(chan view.ListContextLogRequest, count)
	defer close(ch)
	for i := range queries {
		ch <- queries[i]
	}
	workload, err := h.getWorkloadForAuth(context.Background(), workloadId)
	if err != nil {
		return nil, err
	}
	clusterId := v1.GetClusterId(workload)
	opensearchClient := commonsearch.GetOpensearchClient(clusterId)
	if opensearchClient == nil {
		return nil, commonerrors.NewInternalError("There is no OpenSearch in cluster " + clusterId)
	}
	var response [count]commonsearch.OpenSearchResponse
	_, err = concurrent.Exec(count, func() error {
		wrapper := <-ch
		query := wrapper.Query
		resp, err := opensearchClient.SearchByTimeRange(query.SinceTime, query.UntilTime,
			"", "/_search", buildSearchBody(query, workloadId))
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
	if err = addContextDoc(result, queries[0], &response[0], true); err != nil {
		return nil, err
	}
	if err = addContextDoc(result, queries[1], &response[1], false); err != nil {
		return nil, err
	}
	result.Took = time.Since(startTime).Milliseconds()
	return result, nil
}

// buildSearchBody constructs the OpenSearch query body for log searching.
// It configures the query parameters including pagination, sorting by time,
// time range filtering, label filters, keyword searches, and output fields.
// Returns the serialized JSON byte array of the search request.
func buildSearchBody(query *view.ListLogRequest, workloadId string) []byte {
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

func buildFilter(req *commonsearch.OpenSearchRequest, query *view.ListLogRequest) {
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
	valueList := stringutil.Split(values, ",")
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

func buildKeywords(req *commonsearch.OpenSearchRequest, query *view.ListLogRequest) {
	// and search
	for _, key := range query.Keywords {
		words := stringutil.Split(key, " ")
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

func buildOutput(req *commonsearch.OpenSearchRequest, query *view.ListLogRequest, workloadId string) {
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

func parseWorkloadLogQuery(c *gin.Context, workload *v1.Workload) (*view.ListLogRequest, error) {
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

func parseServiceLogQuery(c *gin.Context) (*view.ListLogRequest, error) {
	name := c.GetString(common.Name)
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

func parseEventLogQuery(c *gin.Context, workload *v1.Workload) (*view.ListLogRequest, error) {
	query, err := parseLogQuery(c.Request, workload.CreationTimestamp.Time, workload.EndTime())
	if err != nil {
		klog.ErrorS(err, "failed to parse log query")
		return nil, err
	}
	return query, nil
}

// parseContextQuery parses the context log query parameters for a workload.
// It creates two queries - one for logs after the specified time (ascending order)
// and one for logs before the specified time (descending order, limited by workload creation time).
// This allows retrieving contextual logs around a specific log entry.
// Returns a slice of ListContextLogRequest containing both queries or an error if parsing fails.
func parseContextQuery(c *gin.Context, workload *v1.Workload) ([]view.ListContextLogRequest, error) {
	docId := c.Param(view.DocId)
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
	result := make([]view.ListContextLogRequest, 0, 2)
	// Query with a higher limit to ensure the specified logId is among the results
	query.Limit += 100
	// context search should disable keywords search
	query.Offset = 0
	query.Keywords = nil

	query2 := new(view.ListLogRequest)
	*query2 = *query
	query.Order = dbclient.ASC
	result = append(result, view.ListContextLogRequest{
		Query: query,
		Id:    0,
		Limit: limit,
		DocId: docId,
	})

	query2.Order = dbclient.DESC
	query2.UntilTime = query.SinceTime
	query2.SinceTime = workload.CreationTimestamp.Time
	result = append(result, view.ListContextLogRequest{
		Query: query2,
		Id:    1,
		Limit: limit,
		DocId: docId,
	})
	return result, nil
}

// parseLogQuery parses and validates the log query parameters from an HTTP request.
// It handles pagination, sorting order, time range validation, and ensures all parameters
// are within acceptable limits. For time range, it applies default values and constraints
// based on the provided beginTime and endTime parameters.
// Returns a validated ListLogRequest object or an error if validation fails.
func parseLogQuery(req *http.Request, beginTime, endTime time.Time) (*view.ListLogRequest, error) {
	klog.Infof("beginTime: %s, endTime: %s", beginTime.String(), endTime.String())
	query := &view.ListLogRequest{}
	_, err := apiutils.ParseRequestBody(req, &query.ListLogInput)
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

	if query.Since != "" {
		if query.SinceTime, err = timeutil.CvtStrToRFC3339Milli(query.Since); err != nil {
			return nil, err
		}
	}
	if query.Until != "" {
		if query.UntilTime, err = timeutil.CvtStrToRFC3339Milli(query.Until); err != nil {
			return nil, err
		}
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

// addContextDoc processes and adds contextual log documents to the search response.
// It finds the specified document ID in the response, then extracts logs before or after
// that document (based on isAsc flag) up to the specified limit. Each log entry is assigned
// a line number for context (positive for forward context, negative for backward context).
// The function updates the result response with the extracted documents and total count.
func addContextDoc(result *commonsearch.OpenSearchResponse,
	query view.ListContextLogRequest, response *commonsearch.OpenSearchResponse, isAsc bool) error {
	id := -1
	for i := range response.Hits.Hits {
		if response.Hits.Hits[i].Id == query.DocId {
			id = i + 1
			break
		}
	}
	if id == -1 {
		return commonerrors.NewInternalError(fmt.Sprintf("the docId %s is not found", query.DocId))
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
	return nil
}
