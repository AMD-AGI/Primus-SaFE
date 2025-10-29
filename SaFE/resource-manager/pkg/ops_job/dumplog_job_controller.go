/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	commons3 "github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	contextTTL        = "10m"
	maxBatchNum       = 10000
	minBatchSize      = 5 * 1024 * 1024
	defaultConcurrent = 8
)

type workloadInfo struct {
	workloadId string
	cluster    string
	startTime  time.Time
	endTime    time.Time
}

type DumpLogJobReconciler struct {
	*OpsJobBaseReconciler
	s3Client commons3.Interface
	dbClient dbclient.Interface
	*controller.Controller[string]
}

// SetupDumpLogJobController initializes and registers the DumpLogJobReconciler with the controller manager
func SetupDumpLogJobController(ctx context.Context, mgr manager.Manager) error {
	if !commonconfig.IsS3Enable() || !commonconfig.IsOpenSearchEnable() {
		return nil
	}
	s3Client, err := commons3.NewClient(ctx, commons3.Option{
		Subdir: "log", ExpireDay: commonconfig.GetS3ExpireDay()})
	if err != nil {
		return err
	}

	r := &DumpLogJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
		s3Client: s3Client,
		dbClient: dbclient.NewClient(),
	}
	if r.dbClient == nil {
		return fmt.Errorf("failed to new db-client")
	}
	r.Controller = controller.NewController[string](r, defaultConcurrent)
	r.start(ctx)

	err = ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup DumpLog Controller successfully")
	return nil
}

// Reconcile is the main control loop for DumpLogJob resources
func (r *DumpLogJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r)
}

// observe checks the job status for dump log operations
func (r *DumpLogJobReconciler) observe(_ context.Context, _ *v1.OpsJob) (bool, error) {
	return false, nil
}

// filter determines if the job should be processed by this dump log reconciler
func (r *DumpLogJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobDumpLogType
}

// handle processes the dump log job by adding it to the work queue, trigger subsequent parallel processing using the Do interface.
func (r *DumpLogJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		if err := r.setJobPhase(ctx, job, v1.OpsJobRunning); err != nil {
			return ctrlruntime.Result{}, err
		}
		// ensure that job will be reconciled when it is timeout
		return newRequeueAfterResult(job), nil
	}
	r.Add(job.Name)
	return ctrlruntime.Result{}, nil
}

// start initializes and runs the worker routines for processing dump log jobs
func (r *DumpLogJobReconciler) start(ctx context.Context) {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
}

// Do processes a dump log job by retrieving logs and uploading to S3
func (r *DumpLogJobReconciler) Do(ctx context.Context, jobId string) (ctrlruntime.Result, error) {
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if job.IsEnd() {
		return ctrlruntime.Result{}, nil
	}

	result, err := r.processDumpLogJob(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to handle job", "job", jobId)
		if utils.IsNonRetryableError(err) {
			err = r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
		}
	}
	return result, err
}

// do performs the main dump log operation including search, upload, and status update
func (r *DumpLogJobReconciler) processDumpLogJob(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	workload, err := r.getInputWorkload(ctx, job)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	opensearchClient := commonsearch.GetOpensearchClient(workload.cluster)
	if opensearchClient == nil {
		return ctrlruntime.Result{}, commonerrors.NewInternalError("There is no OpenSearch in cluster " + workload.cluster)
	}
	searchResult, err := r.doSearch(opensearchClient, job, workload)
	if err != nil {
		return ctrlruntime.Result{}, err
	}

	// If the total number of documents is below the per-query threshold, perform a single upload.
	// Otherwise, split the data into multiple uploads
	if searchResult.Hits.Total.Value <= commonsearch.MaxDocsPerQuery {
		err = r.singleUpload(ctx, job, workload, searchResult)
	} else {
		err = r.multiUpload(ctx, opensearchClient, job, workload, searchResult)
	}
	r.clearScroll(opensearchClient, searchResult.ScrollId)
	if err != nil {
		if err2 := r.s3Client.DeleteObject(ctx, workload.workloadId, 0); err2 != nil {
			klog.ErrorS(err2, "failed to delete object", "object", workload.workloadId)
		}
		klog.Infof("failed to upload %s log", workload.workloadId)
		return ctrlruntime.Result{}, commonerrors.NewInternalError(err.Error())
	}

	if err = r.setOutput(ctx, job, workload.workloadId); err == nil {
		klog.Infof("Processing dumplog job %s for workload %s", job.Name, workload.workloadId)
		return ctrlruntime.Result{}, nil
	} else {
		klog.Error(err, "failed to update job status")
		return ctrlruntime.Result{}, commonerrors.NewInternalError(err.Error())
	}
}

// singleUpload uploads log data in a single S3 operation
func (r *DumpLogJobReconciler) singleUpload(ctx context.Context, job *v1.OpsJob,
	workload *workloadInfo, searchResult *commonsearch.OpenSearchResponse) error {
	content := serializeSearchResponse(searchResult)
	_, err := r.s3Client.PutObject(ctx, workload.workloadId, content, int64(job.Spec.TimeoutSecond))
	if err != nil {
		return err
	}
	klog.Infof("uploaded %s log Successfully", workload.workloadId)
	return nil
}

// multiUpload uploads large log data using S3 multipart upload
func (r *DumpLogJobReconciler) multiUpload(
	ctx context.Context,
	client *commonsearch.SearchClient,
	job *v1.OpsJob,
	workload *workloadInfo, searchResult *commonsearch.OpenSearchResponse) error {
	uploadId, err := r.s3Client.CreateMultiPartUpload(ctx, workload.workloadId, job.GetLeftTime())
	if err != nil {
		return err
	}

	logCh := make(chan *commonsearch.OpenSearchResponse, 10)
	errCh := make(chan error, 10)
	stopCh := make(chan struct{})
	defer func() {
		defer close(logCh)
		defer close(errCh)
		if !channel.IsChannelClosed(stopCh) {
			close(stopCh)
		}
	}()

	param := &commons3.MultiUploadParam{
		Key:      workload.workloadId,
		UploadId: uploadId,
		CompletedParts: make([]types.CompletedPart, 0,
			(searchResult.Hits.Total.Value/commonsearch.MaxDocsPerQuery)+1),
	}
	logCh <- searchResult
	go r.scroll(client, job, searchResult.ScrollId, logCh, errCh)
	go r.dump(ctx, job, param, logCh, errCh, stopCh)

	<-stopCh
	if len(errCh) > 0 {
		err = r.s3Client.AbortMultiPartUpload(ctx, param, 0)
		if err != nil {
			klog.ErrorS(err, "failed to abort multi-part upload", "job", job.Name)
		}
		err = <-errCh
		return err
	}
	output, err := r.s3Client.CompleteMultiPartUpload(ctx, param, job.GetLeftTime())
	if err != nil {
		return err
	}
	location := ""
	if output != nil && output.Location != nil {
		location = *output.Location
	}
	klog.Infof("uploaded %s log Successfully, output: %s", workload.workloadId, location)
	return nil
}

// getInputWorkload retrieves workload information from job parameters
func (r *DumpLogJobReconciler) getInputWorkload(ctx context.Context, job *v1.OpsJob) (*workloadInfo, error) {
	param := job.GetParameter(v1.ParameterWorkload)
	if param == nil || param.Value == "" {
		return nil, commonerrors.NewBadRequest("the workload param is empty")
	}
	result := &workloadInfo{
		workloadId: param.Value,
	}
	if commonconfig.IsDBEnable() {
		workload, err := r.dbClient.GetWorkload(ctx, param.Value)
		if err != nil {
			return nil, err
		}
		result.cluster = workload.Cluster
		result.startTime = dbutils.ParseNullTime(workload.CreateTime)
		result.endTime = dbutils.ParseNullTime(workload.EndTime)
	} else {
		workload := &v1.Workload{}
		if err := r.Get(ctx, client.ObjectKey{Name: param.Value}, workload); err != nil {
			return nil, err
		}
		result.cluster = v1.GetClusterId(workload)
		result.startTime = workload.CreationTimestamp.Time
		result.endTime = workload.EndTime()
	}
	if result.endTime.IsZero() {
		result.endTime = time.Now().UTC()
	}
	return result, nil
}

// doSearch performs log search in OpenSearch based on job and workload parameters
func (r *DumpLogJobReconciler) doSearch(client *commonsearch.SearchClient, job *v1.OpsJob, workload *workloadInfo) (*commonsearch.OpenSearchResponse, error) {
	body := buildSearchBody(job, workload)

	data, err := client.SearchByTimeRange(workload.startTime, workload.endTime,
		fmt.Sprintf("/_search?scroll=%s", contextTTL), body)
	if err != nil {
		return nil, commonerrors.NewInternalError(err.Error())
	}
	result := &commonsearch.OpenSearchResponse{}
	if err = json.Unmarshal(data, result); err != nil {
		return nil, commonerrors.NewInternalError(err.Error())
	}
	if result.Hits.Total.Value == 0 {
		return nil, commonerrors.NewNotFoundWithMessage(
			fmt.Sprintf("the log of workload(%s) is not found", workload.workloadId))
	}
	klog.Infof("workload: %s, total log count: %d", workload.workloadId, result.Hits.Total.Value)
	return result, nil
}

// buildSearchBody constructs the OpenSearch query body for log retrieval
func buildSearchBody(job *v1.OpsJob, workload *workloadInfo) []byte {
	searchRequest := &commonsearch.OpenSearchRequest{
		Size: commonsearch.MaxDocsPerQuery,
		Sort: []commonsearch.OpenSearchField{{
			"@timestamp": map[string]interface{}{
				"order": "asc",
			}},
		},
	}
	searchRequest.Query.Bool.Must = []commonsearch.OpenSearchField{{
		"range": map[string]interface{}{
			"@timestamp": map[string]string{
				"gte": workload.startTime.Format(timeutil.TimeRFC3339Milli),
				"lte": workload.endTime.Format(timeutil.TimeRFC3339Milli),
			},
		},
	}}

	dispatchCntKey := strings.ReplaceAll(v1.WorkloadDispatchCntLabel, ".", "_")
	searchRequest.Source = []string{
		commonsearch.TimeField, commonsearch.MessageField, commonsearch.StreamField, "kubernetes.host", "kubernetes.pod_name",
		"kubernetes.labels.training_kubeflow_org/replica-index", "kubernetes.labels.training_kubeflow_org/replica-type",
		fmt.Sprintf("kubernetes.labels.%s", dispatchCntKey),
	}

	workloadIdKey := strings.ReplaceAll(v1.WorkloadIdLabel, ".", "_")
	searchRequest.Query.Bool.Filter = append(searchRequest.Query.Bool.Filter, commonsearch.OpenSearchField{
		"term": map[string]interface{}{
			fmt.Sprintf("kubernetes.labels.%s.keyword", workloadIdKey): workload.workloadId,
		},
	})

	var nodes []map[string]interface{}
	for _, param := range job.Spec.Inputs {
		if param.Name == v1.ParameterNode {
			nodes = append(nodes, map[string]interface{}{
				"term": map[string]string{
					"kubernetes.host.keyword": param.Value,
				},
			})
		}
	}
	if len(nodes) > 0 {
		searchRequest.Query.Bool.Must = append(searchRequest.Query.Bool.Must, commonsearch.OpenSearchField{
			"bool": map[string]interface{}{
				"should": nodes,
			},
		})
	}

	return jsonutils.MarshalSilently(searchRequest)
}

// scroll retrieves log data using OpenSearch scroll API
func (r *DumpLogJobReconciler) scroll(client *commonsearch.SearchClient, job *v1.OpsJob, scrollId string,
	logCh chan<- *commonsearch.OpenSearchResponse, errCh chan<- error) {
	request := &commonsearch.OpenSearchScrollRequest{
		Scroll:   contextTTL,
		ScrollId: scrollId,
	}
	body := jsonutils.MarshalSilently(request)

	for {
		data, err := client.Request("/_search/scroll", http.MethodPost, body)
		response := new(commonsearch.OpenSearchResponse)
		if err == nil {
			err = json.Unmarshal(data, response)
		}
		if err != nil {
			klog.ErrorS(err, "failed to scroll")
			logCh <- nil
			errCh <- err
			break
		}
		if len(response.Hits.Hits) > 0 {
			logCh <- response
		}
		// Reached the end of results. Exiting
		if len(response.Hits.Hits) < commonsearch.MaxDocsPerQuery {
			logCh <- nil
			break
		}
		if job.IsTimeout() {
			errCh <- fmt.Errorf("job is timeout")
			logCh <- nil
			break
		}
	}
}

// dump processes and uploads log data through S3 multipart upload
func (r *DumpLogJobReconciler) dump(ctx context.Context, job *v1.OpsJob, param *commons3.MultiUploadParam,
	logCh <-chan *commonsearch.OpenSearchResponse, errCh chan<- error, stopCh chan struct{}) {
	param.PartNumber = 1
	param.Value = ""
	hasError := false
	defer close(stopCh)

	for {
		logContent := <-logCh
		if logContent == nil {
			break
		}
		if job.IsTimeout() {
			errCh <- fmt.Errorf("job is timeout")
			hasError = true
			break
		}
		param.Value += serializeSearchResponse(logContent)
		if len(param.Value) < minBatchSize {
			continue
		}
		err := r.s3Client.MultiPartUpload(ctx, param, job.GetLeftTime())
		if err != nil {
			klog.ErrorS(err, "failed to multi-upload", "partNumber", param.PartNumber)
			errCh <- err
			hasError = true
			break
		}
		param.Value = ""
		param.PartNumber++
		if param.PartNumber >= maxBatchNum {
			break
		}
	}
	if param.Value != "" && !hasError {
		if param.PartNumber == 1 {
			if _, err := r.s3Client.PutObject(ctx, param.Key, param.Value, job.GetLeftTime()); err != nil {
				klog.ErrorS(err, "failed to put object")
				errCh <- err
			}
		} else {
			err := r.s3Client.MultiPartUpload(ctx, param, job.GetLeftTime())
			if err != nil {
				klog.ErrorS(err, "failed to multi-upload", "partNumber", param.PartNumber)
				errCh <- err
			}
		}
	}
}

// clearScroll cleans up OpenSearch scroll context
func (r *DumpLogJobReconciler) clearScroll(client *commonsearch.SearchClient, scrollId string) {
	req := &commonsearch.OpenSearchScrollRequest{
		ScrollId: scrollId,
	}
	body := jsonutils.MarshalSilently(req)
	_, err := client.Request("/_search/scroll", http.MethodDelete, body)
	if err != nil {
		klog.ErrorS(err, "failed to clear scroll")
	}
}

// setOutput updates job status with the S3 presigned URL for log access
func (r *DumpLogJobReconciler) setOutput(ctx context.Context, job *v1.OpsJob, workloadId string) error {
	var expireDay int32 = 1
	if commonconfig.GetS3ExpireDay() > 0 && expireDay > commonconfig.GetS3ExpireDay() {
		expireDay = commonconfig.GetS3ExpireDay()
	}
	endpoint, err := r.s3Client.GeneratePresignedURL(ctx, workloadId, expireDay)
	if err != nil {
		return err
	}

	outputs := []v1.Parameter{{Name: v1.ParameterEndpoint, Value: endpoint}}
	maxRetry := 3
	if err = backoff.ConflictRetry(func() error {
		err = r.setJobCompleted(ctx, job, v1.OpsJobSucceeded, "", outputs)
		if err == nil {
			return nil
		}
		if apierrors.IsConflict(err) {
			r.Get(ctx, client.ObjectKey{Name: job.Name}, job)
		}
		return err
	}, maxRetry, time.Millisecond*100); err != nil {
		return err
	}
	return nil
}

// serializeSearchResponse converts OpenSearch response to formatted log string
func serializeSearchResponse(data *commonsearch.OpenSearchResponse) string {
	var logBuffer strings.Builder
	for _, doc := range data.Hits.Hits {
		logBuffer.WriteString(doc.Source.Timestamp)
		logBuffer.WriteString(" ")
		logBuffer.WriteString(doc.Source.Kubernetes.Host)
		logBuffer.WriteString(" ")
		if doc.Source.Kubernetes.Labels.ReplicaType != "" && doc.Source.Kubernetes.Labels.ReplicaIndex != "" {
			logBuffer.WriteString(doc.Source.Kubernetes.Labels.ReplicaType + "-" + doc.Source.Kubernetes.Labels.ReplicaIndex)
			logBuffer.WriteString(" ")
		}
		logBuffer.WriteString(doc.Source.Kubernetes.Labels.DispatchCount)
		logBuffer.WriteString(" ")
		logBuffer.WriteString(doc.Source.Message)
		logBuffer.WriteString("\n")
	}
	return logBuffer.String()
}
