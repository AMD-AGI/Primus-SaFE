//go:build integration

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonopsjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
)

// TestModelLifecycleEnvtest runs the Model controller against a real API server with the
// chart CRDs: a model without S3 is downloaded into workspace storage, reports Ready with
// its path and size, and on deletion keeps its finalizer through a failed cleanup until a
// cleanup succeeds. Run with KUBEBUILDER_ASSETS set and -tags integration.
func TestModelLifecycleEnvtest(t *testing.T) {
	setViper(t, map[string]any{"s3.enable": false})
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "charts", "primus-safe", "crds")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	require.NoError(t, err)
	defer func() { _ = env.Stop() }()

	s := clientscheme.Scheme
	require.NoError(t, v1.AddToScheme(s))
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: s, Metrics: metricsserver.Options{BindAddress: "0"}})
	require.NoError(t, err)
	require.NoError(t, SetupModelController(mgr))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = mgr.Start(ctx) }()
	cl, err := client.New(cfg, client.Options{Scheme: s})
	require.NoError(t, err)

	eventually := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	getModelNamed := func(name string) (*v1.Model, error) {
		m := &v1.Model{}
		return m, cl.Get(ctx, client.ObjectKey{Name: name}, m)
	}
	getModel := func() (*v1.Model, error) { return getModelNamed("m1") }
	// ageCleanupFailures moves the last cleanup failure of a model back past any backoff.
	ageCleanupFailures := func(name string) {
		t.Helper()
		require.NoError(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
			m, err := getModelNamed(name)
			if err != nil {
				return err
			}
			for i := range m.Status.LocalPaths {
				if at := m.Status.LocalPaths[i].LastCleanupFailureTime; at != nil {
					m.Status.LocalPaths[i].LastCleanupFailureTime = &metav1.Time{Time: at.Add(-cleanupMaxRetryInterval)}
				}
			}
			return cl.Status().Update(ctx, m)
		}))
	}
	gone := func(name string) func() bool {
		return func() bool {
			_, err := getModelNamed(name)
			return apierrors.IsNotFound(err)
		}
	}
	setPhase := func(name string, phase v1.OpsJobPhase, out string) {
		t.Helper()
		job := &v1.OpsJob{}
		require.NoError(t, cl.Get(ctx, client.ObjectKey{Name: name}, job))
		job.Status.Phase = phase
		if out != "" {
			job.Status.Outputs = []v1.Parameter{{Name: "result", Value: out}}
		}
		if phase == v1.OpsJobFailed {
			failed := failedJobCondition("[ERROR] rm failed\n")
			failed.LastTransitionTime = metav1.Now()
			job.Status.Conditions = []metav1.Condition{failed}
		}
		require.NoError(t, cl.Status().Update(ctx, job))
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "primus-safe"}}
	require.NoError(t, cl.Create(ctx, ns))
	require.NoError(t, cl.Create(ctx, lifecycleWorkspace("ws1", "c1", lifecycleRoot)))
	require.NoError(t, cl.Create(ctx, lifecycleModel("m1")))

	// Download straight into workspace storage.
	var download *v1.OpsJob
	eventually("download job", func() bool {
		jobs := &v1.OpsJobList{}
		if cl.List(ctx, jobs, client.MatchingLabels{v1.ModelIdLabel: "m1"}) != nil || len(jobs.Items) != 1 {
			return false
		}
		download = &jobs.Items[0]
		return true
	})
	require.Equal(t, v1.OpsJobDownloadType, download.Spec.Type)
	require.Equal(t, lifecyclePath, download.GetParameter(v1.ParameterDestPath).Value)
	require.Nil(t, download.GetParameter(v1.ParameterSecret))
	require.Equal(t, "alice", v1.GetUserId(download))
	m, err := getModel()
	require.NoError(t, err)
	require.Equal(t, v1.ModelPhaseDownloading, m.Status.Phase)

	// What the job-manager keeps of the download log: the marked lines, as a JSON array.
	setPhase(download.Name, v1.OpsJobSucceeded, commonopsjob.FilterResultLog([]byte(
		"Downloading org/repo\n[SUCCESS] "+modelSizeMarker+"12345\n")))
	eventually("Ready", func() bool {
		m, err = getModel()
		return err == nil && m.Status.Phase == v1.ModelPhaseReady
	})
	require.Equal(t, lifecyclePath, m.Status.LocalPaths[0].Path)
	require.Equal(t, int64(12345), m.Status.LocalPaths[0].SizeBytes, "sizeBytes must survive the CRD schema")

	// Delete: a failed cleanup keeps the model, a successful one releases it.
	require.NoError(t, cl.Delete(ctx, m))
	cleanupName := cleanupJobName(m, "ws1", lifecyclePath)
	eventually("cleanup job", func() bool {
		return cl.Get(ctx, client.ObjectKey{Name: cleanupName}, &v1.OpsJob{}) == nil
	})
	first := &v1.OpsJob{}
	require.NoError(t, cl.Get(ctx, client.ObjectKey{Name: cleanupName}, first))
	setPhase(cleanupName, v1.OpsJobFailed, "")
	eventually("failure recorded", func() bool {
		m, err = getModel()
		return err == nil && len(m.Status.LocalPaths) == 1 && m.Status.LocalPaths[0].CleanupFailures == 1
	})
	require.NotNil(t, m.Status.LocalPaths[0].LastCleanupFailureTime, "the failure time must survive the CRD schema")
	require.Contains(t, m.Status.Message, "retrying in")
	ageCleanupFailures("m1")
	eventually("failed cleanup replaced", func() bool {
		job := &v1.OpsJob{}
		err := cl.Get(ctx, client.ObjectKey{Name: cleanupName}, job)
		return err == nil && job.UID != first.UID
	})
	m, err = getModel()
	require.NoError(t, err, "the model must still exist after a failed cleanup")
	require.True(t, controllerutil.ContainsFinalizer(m, ModelFinalizer))
	require.Len(t, m.Status.LocalPaths, 1)

	setPhase(cleanupName, v1.OpsJobSucceeded, "")
	eventually("model gone", gone("m1"))

	// A model whose directory nobody can reach any more is released.
	createReady := func(name, workspace string) *v1.Model {
		t.Helper()
		m := lifecycleModel(name)
		require.NoError(t, cl.Create(ctx, m))
		eventually(name+" downloading", func() bool {
			m, err = getModelNamed(name)
			return err == nil && m.Status.Phase == v1.ModelPhaseDownloading
		})
		require.NoError(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if m, err = getModelNamed(name); err != nil {
				return err
			}
			m.Status.Phase = v1.ModelPhaseReady
			m.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: workspace, Path: lifecyclePath, Status: v1.LocalPathStatusReady}}
			return cl.Status().Update(ctx, m)
		}))
		return m
	}
	m2 := createReady("m2", "ws-gone")
	require.NoError(t, cl.Delete(ctx, m2))
	eventually("model with unreachable directory gone", gone("m2"))

	// A cleanup that keeps failing holds the model until an administrator abandons it.
	m3 := createReady("m3", "ws1")
	require.NoError(t, cl.Delete(ctx, m3))
	cleanup3 := cleanupJobName(m3, "ws1", lifecyclePath)
	eventually("m3 cleanup job", func() bool {
		return cl.Get(ctx, client.ObjectKey{Name: cleanup3}, &v1.OpsJob{}) == nil
	})
	setPhase(cleanup3, v1.OpsJobFailed, "")
	eventually("m3 failure recorded", func() bool {
		m, err = getModelNamed("m3")
		return err == nil && len(m.Status.LocalPaths) == 1 && m.Status.LocalPaths[0].CleanupFailures == 1
	})
	require.NoError(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if m, err = getModelNamed("m3"); err != nil {
			return err
		}
		m.Annotations[AbandonCleanupAnnotation] = v1.TrueStr
		return cl.Update(ctx, m)
	}))
	eventually("abandoned model gone", gone("m3"))
}
