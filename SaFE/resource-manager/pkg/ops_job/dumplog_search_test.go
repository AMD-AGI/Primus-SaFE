/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	commons3 "github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
)

type commons3iface = commons3.Interface

func TestDumpLogDoSearch(t *testing.T) {
	respBody := `{"hits":{"total":{"value":1},"hits":[{"_id":"1","_source":{"@timestamp":"t1","message":"hello"}}]}}`
	sc := commonsearch.NewTestSearchClient(func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
		return []byte(respBody), nil
	})
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{}}
	wl := &workloadInfo{workloadId: "wl1", startTime: time.Now().Add(-time.Hour), endTime: time.Now()}
	res, err := r.doSearch(sc, job, wl)
	assert.NoError(t, err)
	assert.Equal(t, 1, res.Hits.Total.Value)
}

// stubS3 implements s3.Interface; only the methods used by the dumplog
// single-upload path are functional, the rest are inherited as no-ops via
// the embedded nil interface (never called in these tests).
type stubS3 struct {
	commons3iface
}

func (stubS3) PutObject(_ context.Context, _, _ string, _ int64) (*awss3.PutObjectOutput, error) {
	return &awss3.PutObjectOutput{}, nil
}
func (stubS3) GeneratePresignedURL(_ context.Context, key string, _ int32) (string, error) {
	return "http://download/" + key, nil
}
func (stubS3) DeleteObject(_ context.Context, _ string, _ int64) error { return nil }

func TestDumpLogProcessSingleUpload(t *testing.T) {
	respBody := `{"hits":{"total":{"value":1},"hits":[{"_id":"1","_source":{"@timestamp":"t1","message":"hello"}}]}}`
	sc := commonsearch.NewTestSearchClient(func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
		return []byte(respBody), nil
	})
	cleanup := commonsearch.RegisterClientForTest("c1", sc)
	defer cleanup()

	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterWorkload, Value: "wl1"}}}}
	base := newBaseWithObjs(t, wl, job)
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: base, s3Client: stubS3{}}

	_, err := r.processDumpLogJob(context.Background(), job)
	assert.NoError(t, err)
}

func TestDumpLogDoFull(t *testing.T) {
	respBody := `{"hits":{"total":{"value":1},"hits":[{"_id":"1","_source":{"@timestamp":"t1","message":"hi"}}]}}`
	sc := commonsearch.NewTestSearchClient(func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
		return []byte(respBody), nil
	})
	cleanup := commonsearch.RegisterClientForTest("c1", sc)
	defer cleanup()

	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterWorkload, Value: "wl1"}}}}
	base := newBaseWithObjs(t, wl, job)
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: base, s3Client: stubS3{}}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
}

func TestDumpLogSingleUpload(t *testing.T) {
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t), s3Client: stubS3{}}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{TimeoutSecond: 60}}
	wl := &workloadInfo{workloadId: "wl1"}
	resp := &commonsearch.OpenSearchLogResponse{}
	assert.NoError(t, r.singleUpload(context.Background(), job, wl, resp))
}

func TestDumpLogScroll(t *testing.T) {
	respBody := `{"hits":{"total":{"value":1},"hits":[{"_id":"1","_source":{"@timestamp":"t1","message":"hello"}}]}}`
	sc := commonsearch.NewTestSearchClient(func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
		return []byte(respBody), nil
	})
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{TimeoutSecond: 600}}
	logCh := make(chan *commonsearch.OpenSearchLogResponse, 8)
	errCh := make(chan error, 8)
	r.scroll(sc, job, "sid", logCh, errCh)
	// At least one response plus a terminating nil were enqueued.
	assert.Positive(t, len(logCh))
}

func TestDumpLogDump(t *testing.T) {
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t), s3Client: stubS3{}}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{TimeoutSecond: 600}}
	resp := &commonsearch.OpenSearchLogResponse{}
	doc := commonsearch.OpenSearchLogDoc{Id: "1"}
	doc.Source.Timestamp = "t1"
	doc.Source.Message = "hello"
	resp.Hits.Hits = []commonsearch.OpenSearchLogDoc{doc}
	logCh := make(chan *commonsearch.OpenSearchLogResponse, 4)
	errCh := make(chan error, 4)
	stopCh := make(chan struct{})
	logCh <- resp
	logCh <- nil
	param := &commons3.MultiUploadParam{Key: "k"}
	r.dump(context.Background(), job, param, logCh, errCh, stopCh)
	<-stopCh
}

func TestDumpLogClearScroll(t *testing.T) {
	sc := commonsearch.NewTestSearchClient(func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
		return nil, nil
	})
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	// clusterClient nil -> Request logs error, no panic.
	r.clearScroll(sc, "scroll-1")
}

func TestDumpLogDoSearchEmpty(t *testing.T) {
	sc := commonsearch.NewTestSearchClient(func(_, _ time.Time, _, _ string, _ []byte) ([]byte, error) {
		return []byte(`{"hits":{"total":{"value":0},"hits":[]}}`), nil
	})
	r := &DumpLogJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{}}
	wl := &workloadInfo{workloadId: "wl1"}
	_, err := r.doSearch(sc, job, wl)
	assert.Error(t, err) // not found
}

