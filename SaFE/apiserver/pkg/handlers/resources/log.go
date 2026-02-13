/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
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

// DownloadWorkloadLog handles the request to download workload logs to a local path.
// It creates a DumpLog job, waits for completion, and downloads the result from S3.
func (h *Handler) DownloadWorkloadLog(c *gin.Context) {
	handle(c, h.downloadWorkloadLog)
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
func (h *Handler) searchContextLog(queries []view.ListContextLogRequest, workloadId string) (*commonsearch.OpenSearchLogResponse, error) {
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
	var response [count]commonsearch.OpenSearchLogResponse
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

	result := &commonsearch.OpenSearchLogResponse{}
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
	buildFilter(req, query)
	buildKeywords(req, query)
	buildOutput(req, query, workloadId)
	return jsonutils.MarshalSilently(req)
}

func buildFilter(req *commonsearch.OpenSearchRequest, query *view.ListLogRequest) {
	buildSingleTermFilter(req, query.TermFilters, !query.IsEventRequest, false)
	buildSingleTermFilter(req, query.PrefixFilters, !query.IsEventRequest, true)
	if query.PodNames != "" {
		buildMultiTermsFilter(req, "pod_name", query.PodNames)
	} else if query.NodeNames != "" {
		buildMultiTermsFilter(req, "host", query.NodeNames)
	}
}

func buildSingleTermFilter(req *commonsearch.OpenSearchRequest, filters map[string]string, isK8sLabel, isPrefixMatch bool) {
	for key, val := range filters {
		if key == "" || val == "" {
			continue
		}
		if isK8sLabel {
			// Use the same punctuation handling rules as OpenSearch.
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

func buildMultiTermsFilter(req *commonsearch.OpenSearchRequest, key, values string) {
	valueList := stringutil.Split(values, ",")
	if len(valueList) == 0 {
		return
	}
	var queries []map[string]interface{}
	termKey := fmt.Sprintf("kubernetes.%s.keyword", key)
	for _, val := range valueList {
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
	if query.IsEventRequest {
		return
	}
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
	startTime := getLogQueryStartTime(workload)
	query, err := parseLogQuery(c.Request, startTime, workload.EndTime())
	if err != nil {
		klog.ErrorS(err, "failed to parse log query")
		return nil, err
	}
	query.TermFilters = map[string]string{
		v1.WorkloadIdLabel: workload.Name,
	}
	if query.DispatchCount > 0 {
		query.TermFilters[v1.WorkloadDispatchCntLabel] = strconv.Itoa(query.DispatchCount)
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
	query.TermFilters = map[string]string{
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
	query.TermFilters = map[string]string{
		"involvedObject.namespace": workload.Spec.Workspace,
	}
	query.PrefixFilters = map[string]string{
		"involvedObject.name": workload.Name,
	}
	query.IsEventRequest = true
	// node or pod filtering is not supported
	query.NodeNames = ""
	query.PodNames = ""
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
func addContextDoc(result *commonsearch.OpenSearchLogResponse,
	query view.ListContextLogRequest, response *commonsearch.OpenSearchLogResponse, isAsc bool) error {
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

// getLogQueryStartTime calculates the adjusted start time for log queries.
// It ensures the returned time is not earlier than the workload's creation time.
func getLogQueryStartTime(workload *v1.Workload) time.Time {
	startTime := workload.CreationTimestamp.Time
	if workload.Status.StartTime != nil && !workload.Status.StartTime.IsZero() {
		startTime = workload.Status.StartTime.Time.Add(-time.Hour)
		if startTime.Before(workload.CreationTimestamp.Time) {
			startTime = workload.CreationTimestamp.Time
		}
	}
	return startTime
}

// downloadWorkloadLog implements the logic for downloading workload logs.
func (h *Handler) downloadWorkloadLog(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("the logging function is not enabled")
	}
	if !commonconfig.IsS3Enable() {
		return nil, commonerrors.NewInternalError("the S3 function is not enabled")
	}

	req := &view.DownloadWorkloadLogRequest{}
	if _, err := apiutils.ParseRequestBody(c.Request, req); err != nil {
		return nil, err
	}
	// Set default values
	if req.TimeoutSecond <= 0 {
		req.TimeoutSecond = 900 // 15 minutes
	}

	ctx := c.Request.Context()
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	// Step 1: Verify workload exists and authorize
	workload, err := h.getWorkloadForAuth(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workload.Spec.Workspace},
		User:       requestUser,
	}); err != nil {
		return nil, err
	}

	// Step 2: Create DumpLog job
	job, err := h.createDumpLogJobInternal(ctx, workload, requestUser, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create dumplog job: %w", err)
	}
	klog.Infof("DumpLog job created: %s for workload: %s", job.Name, workload.Name)

	// Step 3: Wait for job completion
	endpoint, err := h.waitForDumpLogJobCompletion(ctx, job.Name, req.TimeoutSecond)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for dumplog job completion: %w", err)
	}

	// Step 4: Download from S3 presigned URL
	localFilePath, err := h.downloadFromPresignedURL(endpoint, req.LocalPath, workload.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to download log file: %w", err)
	}
	klog.Infof("Log file downloaded to: %s", localFilePath)
	return nil, nil
}

// createDumpLogJobInternal creates a DumpLog OpsJob for the specified workload.
func (h *Handler) createDumpLogJobInternal(ctx context.Context,
	workload *v1.Workload, requestUser *v1.User, req *view.DownloadWorkloadLogRequest) (*v1.OpsJob, error) {
	// Build the OpsJob
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: workload.Name, // Use workloadId as job name
			Labels: map[string]string{
				v1.DisplayNameLabel: commonutils.GetBaseFromName(workload.Name),
				v1.WorkspaceIdLabel: workload.Spec.Workspace,
				v1.UserIdLabel:      requestUser.Name,
			},
		},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobDumpLogType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterWorkload, Value: workload.Name},
			},
			TimeoutSecond: req.TimeoutSecond,
		},
	}

	if err := h.Create(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

// waitForDumpLogJobCompletion polls the job status until it completes or times out.
// Returns the S3 endpoint URL on success.
func (h *Handler) waitForDumpLogJobCompletion(ctx context.Context, jobName string, timeoutSecond int) (string, error) {
	timeout := time.Duration(timeoutSecond) * time.Second
	pollInterval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timeout waiting for dumplog job completion after %d seconds", timeoutSecond)
		}

		job := &v1.OpsJob{}
		if err := h.Get(ctx, client.ObjectKey{Name: jobName}, job); err != nil {
			return "", fmt.Errorf("failed to get job status: %w", err)
		}

		switch job.Status.Phase {
		case v1.OpsJobSucceeded:
			output := job.GetParameter(v1.ParameterEndpoint)
			if output != nil {
				return output.Value, nil
			}
			return "", fmt.Errorf("job succeeded but no endpoint found in outputs")
		case v1.OpsJobFailed:
			message := "unknown error"
			// Get error message from conditions
			for _, cond := range job.Status.Conditions {
				if cond.Status == metav1.ConditionFalse && cond.Message != "" {
					message = cond.Message
					break
				}
			}
			return "", fmt.Errorf("dumplog job failed: %s", message)
		}

		// Job still running, wait and poll again
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(pollInterval):
			// continue polling
		}
	}
}

// downloadFromPresignedURL downloads a file from an S3 presigned URL to the local path.
func (h *Handler) downloadFromPresignedURL(workloadId, presignedURL, localPath string) (string, error) {
	resp, err := h.httpClient.Get(presignedURL)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file with status: %d", resp.StatusCode)
	}
	// Ensure directory exists
	if err = ensureDir(localPath); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	localFilePath := localPath
	if isDirectory(localPath) {
		localFilePath = filepath.Join(localPath, workloadId)
	}

	// Create local file
	file, err := os.Create(localFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy content
	if _, err = io.Copy(file, bytes.NewReader(resp.Body)); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return localFilePath, nil
}

// ensureDir ensures the directory for the given path exists.
func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if isDirectory(path) {
		dir = path
	}
	return os.MkdirAll(dir, 0755)
}

// isDirectory checks if the given path is a directory or looks like a directory path.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir()
	}
	// If path doesn't exist, check if it looks like a directory (ends with separator or no extension)
	return strings.HasSuffix(path, string(filepath.Separator)) || filepath.Ext(path) == ""
}
