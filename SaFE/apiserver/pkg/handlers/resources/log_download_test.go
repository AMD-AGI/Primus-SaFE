/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// withS3 enables both OpenSearch and S3 for the duration of the test.
func withS3(t *testing.T) {
	t.Helper()
	commonconfig.SetValue("opensearch.enable", "true")
	commonconfig.SetValue("s3.enable", "true")
	t.Cleanup(func() {
		commonconfig.SetValue("opensearch.enable", "false")
		commonconfig.SetValue("s3.enable", "false")
	})
}

func succeededDumpLogJob(name, endpoint string) *v1.OpsJob {
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: name}}
	job.Status.Phase = v1.OpsJobSucceeded
	job.Status.Outputs = []v1.Parameter{{Name: v1.ParameterEndpoint, Value: endpoint}}
	return job
}

func TestCreateDumpLogJobInternal(t *testing.T) {
	wl := newWorkloadForLog("wl-1", "c1", "ws-1")
	h, user := newAdminHandlerWithObjects(wl)

	job, err := h.createDumpLogJobInternal(context.Background(), wl, user, &view.DownloadWorkloadLogRequest{TimeoutSecond: 60})
	assert.NoError(t, err)
	assert.Contains(t, job.Name, "down-")

	// Idempotent: second call returns the existing job.
	job2, err := h.createDumpLogJobInternal(context.Background(), wl, user, &view.DownloadWorkloadLogRequest{TimeoutSecond: 60})
	assert.NoError(t, err)
	assert.Equal(t, job.Name, job2.Name)
}

func TestWaitForDumpLogJobCompletion(t *testing.T) {
	// Succeeded -> returns endpoint URL immediately.
	h, _ := newAdminHandlerWithObjects(succeededDumpLogJob("job-ok", "https://s3/log.tar.gz"))
	url, err := h.waitForDumpLogJobCompletion(context.Background(), "job-ok", 60)
	assert.NoError(t, err)
	assert.Equal(t, "https://s3/log.tar.gz", url)

	// Failed -> error.
	failed := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "job-fail"}}
	failed.Status.Phase = v1.OpsJobFailed
	failed.Status.Conditions = []metav1.Condition{{Status: metav1.ConditionFalse, Message: "boom"}}
	h2, _ := newAdminHandlerWithObjects(failed)
	_, err = h2.waitForDumpLogJobCompletion(context.Background(), "job-fail", 60)
	assert.Error(t, err)

	// Timeout (deadline already passed) -> error, no real sleeping.
	h3, _ := newAdminHandlerWithObjects()
	_, err = h3.waitForDumpLogJobCompletion(context.Background(), "missing", 0)
	assert.Error(t, err)
}

func TestDownloadWorkloadLogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("s3 disabled", func(t *testing.T) {
		commonconfig.SetValue("opensearch.enable", "true")
		t.Cleanup(func() { commonconfig.SetValue("opensearch.enable", "false") })
		h, user := newAdminHandlerWithObjects(newWorkloadForLog("wl-1", "c1", "ws-1"))
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "wl-1")
		_, err := h.downloadWorkloadLog(c)
		assert.Error(t, err)
	})

	t.Run("success with pre-seeded succeeded job", func(t *testing.T) {
		withS3(t)
		wl := newWorkloadForLog("wl-1", "c1", "ws-1")
		// Pre-seed the dump-log job so createDumpLogJobInternal returns it and
		// waitForDumpLogJobCompletion immediately reads the endpoint output.
		dumpJob := succeededDumpLogJob("down-wl-1", "https://s3/wl-1.tar.gz")
		h, user := newAdminHandlerWithObjects(wl, dumpJob)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"timeoutSecond":60}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "wl-1")
		resp, err := h.downloadWorkloadLog(c)
		assert.NoError(t, err)
		assert.Equal(t, "https://s3/wl-1.tar.gz", resp.DownloadURL)
	})
}
