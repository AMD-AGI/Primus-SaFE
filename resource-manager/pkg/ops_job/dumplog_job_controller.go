/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

	"github.com/aws/aws-sdk-go/service/s3"
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
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	contextTTL      = "10m"
	maxBatchNum     = 10000
	maxDocsPerQuery = 10000
	minBatchSize    = 5 * 1024 * 1024
	concurrent      = 8
)

type workloadInfo struct {
	workloadId string
	cluster    string
	startTime  time.Time
	endTime    time.Time
}

type DumpLogJobReconciler struct {
	*OpsJobBaseReconciler
	s3Client     commons3.Interface
	dbClient     dbclient.Interface
	searchClient *commonsearch.SearchClient
	*controller.Controller[string]
}

func SetupDumpLogJobController(ctx context.Context, mgr manager.Manager) error {
	if !commonconfig.IsS3Enable() || !commonconfig.IsLogEnable() {
		return nil
	}
	r := &DumpLogJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
		s3Client:     commons3.NewClient(ctx),
		dbClient:     dbclient.NewClient(),
		searchClient: commonsearch.NewClient(),
	}
	if r.s3Client == nil {
		return fmt.Errorf("failed to new s3-client")
	}
	if r.dbClient == nil {
		return fmt.Errorf("failed to new db-client")
	}
	if r.searchClient == nil {
		return fmt.Errorf("failed to new search-client")
	}
	r.Controller = controller.NewController[string](r, concurrent)
	r.start(ctx)

	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, jobPhaseChangedPredicate()))).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup DumpLog Controller successfully")
	return nil
}

func (r *DumpLogJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, nil)
}

func (r *DumpLogJobReconciler) observe(_ context.Context, _ *v1.OpsJob) (bool, error) {
	return false, nil
}

func (r *DumpLogJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobDumplogType
}

func (r *DumpLogJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		return r.setJobRunning(ctx, job)
	}
	r.Add(job.Name)
	return ctrlruntime.Result{}, nil
}

func (r *DumpLogJobReconciler) start(ctx context.Context) {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
}

func (r *DumpLogJobReconciler) Do(ctx context.Context, jobId string) (ctrlruntime.Result, error) {
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if job.IsEnd() {
		return ctrlruntime.Result{}, nil
	}

	result, err := r.do(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to handle job", "job", jobId)
		if utils.IsNonRetryableError(err) {
			err = r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
		}
	}
	return result, err
}

func (r *DumpLogJobReconciler) do(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	workload, err := r.getInputWorkload(ctx, job)
	if err != nil {
		return ctrlruntime.Result{}, err
	}

	searchResult, err := r.doSearch(job, workload)
	if err != nil {
		return ctrlruntime.Result{}, err
	}

	// If the total number of documents is below the per-query threshold, perform a single upload.
	// Otherwise, split the data into multiple uploads
	if searchResult.Hits.Total.Value <= maxDocsPerQuery {
		err = r.singleUpload(ctx, job, workload, searchResult)
	} else {
		err = r.multiUpload(ctx, job, workload, searchResult)
	}
	r.clearScroll(searchResult.ScrollId)

	if err != nil {
		r.s3Client.DeleteObject(ctx, workload.workloadId, 0)
		return ctrlruntime.Result{}, commonerrors.NewInternalError(err.Error())
	}

	endpoint := strings.TrimSuffix(commonconfig.GetS3Endpoint(), "/") + "/" +
		strings.TrimSuffix(commonconfig.GetS3Bucket(), "/") + "/" + workload.workloadId
	outputs := []v1.Parameter{{Name: v1.ParameterEndpoint, Value: endpoint}}
	r.setJobCompleted(ctx, job, v1.OpsJobSucceeded, "", outputs)
	return ctrlruntime.Result{}, nil
}

func (r *DumpLogJobReconciler) singleUpload(ctx context.Context, job *v1.OpsJob,
	workload *workloadInfo, searchResult *commonsearch.OpenSearchResponse) error {
	content := serializeSearchResponse(searchResult)
	err := r.s3Client.PutObject(ctx, workload.workloadId, content, int64(job.Spec.TimeoutSecond))
	return err
}

func (r *DumpLogJobReconciler) multiUpload(ctx context.Context, job *v1.OpsJob,
	workload *workloadInfo, searchResult *commonsearch.OpenSearchResponse) error {
	s3ClientInner, uploadId, err := r.s3Client.CreateMultiPartUpload(ctx, workload.workloadId, job.GetLeftTime())
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
		S3Client:       s3ClientInner,
		Key:            workload.workloadId,
		UploadId:       uploadId,
		CompletedParts: make([]*s3.CompletedPart, 0, (searchResult.Hits.Total.Value/maxDocsPerQuery)+1),
	}
	logCh <- searchResult
	go r.scroll(job, searchResult.ScrollId, logCh, errCh)
	go r.dump(ctx, job, param, logCh, errCh, stopCh)

	<-stopCh
	if len(errCh) > 0 {
		r.s3Client.AbortMultiPartUpload(ctx, param, 0)
		err = <-errCh
		return err
	}
	return r.s3Client.CompleteMultiPartUpload(ctx, param, job.GetLeftTime())
}

func (r *DumpLogJobReconciler) getInputWorkload(ctx context.Context, job *v1.OpsJob) (*workloadInfo, error) {
	param := job.GetParameter(v1.ParameterWorkload)
	if param == nil || param.Value == "" {
		return nil, commonerrors.NewBadRequest("the workload param is empty")
	}
	result := &workloadInfo{
		workloadId: param.Value,
	}
	if commonconfig.IsDBEnable() {
		workload, err := commonworkload.GetWorkloadFromDb(ctx, r.dbClient, param.Value)
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

func (r *DumpLogJobReconciler) doSearch(job *v1.OpsJob, workload *workloadInfo) (*commonsearch.OpenSearchResponse, error) {
	body := buildSearchBody(job, workload)
	data, err := r.searchClient.RequestByTimeRange(workload.startTime, workload.endTime,
		fmt.Sprintf("/_search?scroll=%s", contextTTL), http.MethodPost, body)
	if err != nil {
		return nil, err
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

func buildSearchBody(job *v1.OpsJob, workload *workloadInfo) []byte {
	req := &commonsearch.OpenSearchRequest{
		Size: maxDocsPerQuery,
		Sort: []commonsearch.OpenSearchField{{
			"@timestamp": map[string]interface{}{
				"order": "asc",
			}},
		},
	}
	req.Query.Bool.Must = []commonsearch.OpenSearchField{{
		"range": map[string]interface{}{
			"@timestamp": map[string]string{
				"gte": workload.startTime.Format(timeutil.TimeRFC3339Milli),
				"lte": workload.endTime.Format(timeutil.TimeRFC3339Milli),
			},
		},
	}}

	dispatchCntKey := strings.ReplaceAll(v1.WorkloadDispatchCntLabel, ".", "_")
	req.Source = []string{
		commonsearch.TimeField, commonsearch.MessageField, commonsearch.StreamField, "kubernetes.host", "kubernetes.pod_name",
		"kubernetes.labels.training_kubeflow_org/replica-index", "kubernetes.labels.training_kubeflow_org/replica-type",
		fmt.Sprintf("kubernetes.labels.%s", dispatchCntKey),
	}

	workloadIdKey := strings.ReplaceAll(v1.WorkloadIdLabel, ".", "_")
	req.Query.Bool.Filter = append(req.Query.Bool.Filter, commonsearch.OpenSearchField{
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
		req.Query.Bool.Must = append(req.Query.Bool.Must, commonsearch.OpenSearchField{
			"bool": map[string]interface{}{
				"should": nodes,
			},
		})
	}

	return jsonutils.MarshalSilently(req)
}

func (r *DumpLogJobReconciler) scroll(job *v1.OpsJob, scrollId string,
	logCh chan<- *commonsearch.OpenSearchResponse, errCh chan<- error) {
	request := &commonsearch.OpenSearchScrollRequest{
		Scroll:   contextTTL,
		ScrollId: scrollId,
	}
	body := jsonutils.MarshalSilently(request)

	for {
		data, err := r.searchClient.Request("/_search/scroll", http.MethodPost, body)
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
		if len(response.Hits.Hits) < maxDocsPerQuery {
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
		part, err := r.s3Client.MultiPartUpload(ctx, param, job.GetLeftTime())
		if err != nil {
			klog.ErrorS(err, "failed to multi-upload", "partNumber", param.PartNumber)
			errCh <- err
			hasError = true
			break
		}
		param.Value = ""
		param.CompletedParts = append(param.CompletedParts, part)
		param.PartNumber++
		if param.PartNumber >= maxBatchNum {
			break
		}
	}
	if param.Value != "" && !hasError {
		if param.PartNumber == 1 {
			if err := r.s3Client.PutObject(ctx, param.Key, param.Value, job.GetLeftTime()); err != nil {
				klog.ErrorS(err, "failed to put object")
				errCh <- err
			}
		} else {
			part, err := r.s3Client.MultiPartUpload(ctx, param, job.GetLeftTime())
			if err != nil {
				klog.ErrorS(err, "failed to multi-upload", "partNumber", param.PartNumber)
				errCh <- err
			} else {
				param.CompletedParts = append(param.CompletedParts, part)
			}
		}
	}
}

func (r *DumpLogJobReconciler) clearScroll(scrollId string) {
	req := &commonsearch.OpenSearchScrollRequest{
		ScrollId: scrollId,
	}
	body := jsonutils.MarshalSilently(req)
	_, err := r.searchClient.Request("/_search/scroll", http.MethodDelete, body)
	if err != nil {
		klog.ErrorS(err, "failed to clear scroll")
	}
}

func serializeSearchResponse(data *commonsearch.OpenSearchResponse) string {
	var sb strings.Builder
	for _, doc := range data.Hits.Hits {
		sb.WriteString(doc.Source.Timestamp)
		sb.WriteString(" ")
		sb.WriteString(doc.Source.Kubernetes.Host)
		sb.WriteString(" ")
		if doc.Source.Kubernetes.Labels.ReplicaType != "" && doc.Source.Kubernetes.Labels.ReplicaIndex != "" {
			sb.WriteString(doc.Source.Kubernetes.Labels.ReplicaType + "-" + doc.Source.Kubernetes.Labels.ReplicaIndex)
			sb.WriteString(" ")
		}
		sb.WriteString(doc.Source.Kubernetes.Labels.DispatchCount)
		sb.WriteString(" ")
		sb.WriteString(doc.Source.Message)
		sb.WriteString("\n")
	}
	return sb.String()
}
