/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func persistedCICDFailure(t *testing.T) (*SyncerReconciler, *v1.Workload, *cicdFailureSnapshot) {
	t.Helper()
	r, w, msg, _ := runnerFailureFixture(t, runnerSetRegistrationTimeout, nil)
	v1.SetAnnotation(w, v1.CICDProxyManagedAnnotation, v1.TrueStr)
	w.Spec.Env = map[string]string{common.ProxyUrl: "http://proxy.example.com"}
	assert.NilError(t, r.Update(context.Background(), w))
	_, err := r.handleJob(context.Background(), msg, monkeyClientSets())
	assert.NilError(t, err)
	assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), w))
	return r, w, cicdSnapshot(t, w)
}

func cicdSnapshot(t *testing.T, w *v1.Workload) *cicdFailureSnapshot {
	t.Helper()
	since, err := timeutil.CvtStrToRFC3339Milli(v1.GetAnnotation(w, v1.WorkloadDispatchedAnnotation))
	assert.NilError(t, err)
	index := commonworkload.WorkloadFailureConditionIndex(w.Status.Conditions, v1.GetWorkloadDispatchCnt(w))
	assert.Assert(t, index >= 0)
	return &cicdFailureSnapshot{workload: w.DeepCopy(), condition: w.Status.Conditions[index], since: since, until: w.Status.EndTime.Time}
}

func cicdErrorResponse(t *testing.T, w *v1.Workload, when time.Time, detail string) []byte {
	t.Helper()
	line, err := json.Marshal(map[string]interface{}{"level": "error", "controllerKind": w.SpecKind(), "namespace": w.Spec.Workspace, "name": w.Name, "error": detail})
	assert.NilError(t, err)
	source := map[string]interface{}{"@timestamp": when.Format(time.RFC3339Nano), "log": string(line), "kubernetes": map[string]interface{}{
		"namespace_name": common.CICDArcNamespace, "pod_name": commonconfig.GetCICDControllerName() + "-example"}}
	body, err := json.Marshal(map[string]interface{}{"hits": map[string]interface{}{"hits": []interface{}{map[string]interface{}{"_source": source}}}})
	assert.NilError(t, err)
	return body
}

func installCICDLogSearch(t *testing.T, cluster string, search opensearch.SearchFunc) {
	t.Helper()
	wasEnabled := commonconfig.IsOpenSearchEnable()
	commonconfig.SetValue("opensearch.enable", "true")
	t.Cleanup(func() {
		if wasEnabled {
			commonconfig.SetValue("opensearch.enable", "true")
		} else {
			commonconfig.SetValue("opensearch.enable", "false")
		}
	})
	t.Cleanup(opensearch.RegisterClientForTest(cluster, opensearch.NewTestSearchClient(search)))
}

func TestCICDFailureEnrichment_RetrievedError(t *testing.T) {
	for _, mode := range []string{"timeout", common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind} {
		t.Run(mode, func(t *testing.T) {
			var r *SyncerReconciler
			var w *v1.Workload
			var snapshot *cicdFailureSnapshot
			if mode == "timeout" {
				r, w, snapshot = persistedCICDFailure(t)
			} else {
				r, w, snapshot = controllerCICDFailure(t, mode)
			}
			fallback := w.Status.Message
			before := w.DeepCopy()
			response := cicdErrorResponse(t, w, snapshot.until, "proxy connection refused by example policy")
			var calls atomic.Int32
			installCICDLogSearch(t, v1.GetClusterId(w), func(start, end time.Time, index, uri string, body []byte) ([]byte, error) {
				calls.Add(1)
				return response, nil
			})
			assert.NilError(t, r.enrichCICDFailureMessage(context.Background(), snapshot))
			current := &v1.Workload{}
			assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), current))
			assert.Assert(t, strings.HasPrefix(current.Status.Message, fallback+" ARC controller: "))
			assert.Assert(t, strings.Contains(current.Status.Message, "proxy connection refused by example policy"))
			assert.Equal(t, current.Status.Conditions[0].Message, current.Status.Message)
			assert.Equal(t, current.Status.Phase, before.Status.Phase)
			assert.DeepEqual(t, current.Status.EndTime, before.Status.EndTime)
			assert.Equal(t, current.Status.RunnerScaleSetId, before.Status.RunnerScaleSetId)
			assert.DeepEqual(t, current.Status.Conditions[0].LastTransitionTime, before.Status.Conditions[0].LastTransitionTime)
			assert.Equal(t, commonworkload.GetWorkloadFailureMessage(current.Status.Conditions, v1.GetWorkloadDispatchCnt(current)), current.Status.Message)
			assert.Assert(t, calls.Load() > 0)
		})
	}
}

func controllerCICDFailure(t *testing.T, kind string) (*SyncerReconciler, *v1.Workload, *cicdFailureSnapshot) {
	t.Helper()
	conditions := []interface{}{map[string]interface{}{"type": "Failed", "status": "True", "message": "ARC reported registration failure"}}
	r, w, msg, obj := runnerFailureFixture(t, time.Minute, conditions)
	if kind == common.CICDEphemeralRunnerKind {
		rt := jobutils.TestCICDRunnerResourceTemplate.DeepCopy()
		rt.Name = "child-resource-template"
		assert.NilError(t, r.Create(context.Background(), rt))
		w.Spec.Kind = kind
		assert.NilError(t, r.Update(context.Background(), w))
		msg.gvk = rt.ToSchemaGVK()
		obj.Object["status"] = map[string]interface{}{"phase": "Failed", "message": "ARC reported registration failure"}
	}
	result, err := r.updateAdminWorkloadByJob(context.Background(), monkeyClientSets(), w, msg)
	assert.NilError(t, err)
	assert.Equal(t, result.Status.Phase, v1.WorkloadFailed)
	assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 1)
	return r, result, cicdSnapshot(t, result)
}

func TestCICDFailureEnrichment_Fallback(t *testing.T) {
	for _, mode := range []string{"disabled", "missing client", "query error", "decode error", "empty", "wrong workload", "blank", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			r, w, snapshot := persistedCICDFailure(t)
			before := w.DeepCopy()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			if mode == "disabled" {
				commonconfig.SetValue("opensearch.enable", "false")
			} else {
				installCICDLogSearch(t, v1.GetClusterId(w), func(time.Time, time.Time, string, string, []byte) ([]byte, error) {
					switch mode {
					case "query error":
						return nil, errors.New("backend unavailable")
					case "decode error":
						return []byte("invalid json"), nil
					case "wrong workload":
						other := w.DeepCopy()
						other.Name += "-sibling"
						return cicdErrorResponse(t, other, snapshot.until, "unrelated failure"), nil
					case "blank":
						return []byte(`{"hits":{"hits":[{"_source":{"message":" "}}]}}`), nil
					default:
						return []byte(`{"hits":{"hits":[]}}`), nil
					}
				})
			}
			if mode == "missing client" {
				snapshot.workload.Labels[v1.ClusterIdLabel] = "unconfigured-cluster"
			}
			_ = r.enrichCICDFailureMessage(ctx, snapshot)
			current := &v1.Workload{}
			assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), current))
			assert.DeepEqual(t, current.Status, before.Status)
		})
	}
}

func TestCICDFailureEnrichment_DoesNotDelayFailed(t *testing.T) {
	r, w, msg, _ := runnerFailureFixture(t, runnerSetRegistrationTimeout, nil)
	entered, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		close(entered)
		<-request.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	commonconfig.SetValue("opensearch.enable", "true")
	defer commonconfig.SetValue("opensearch.enable", "false")
	commonconfig.SetValue("cicd.failure_enrich_timeout_seconds", "1")
	defer commonconfig.SetValue("cicd.failure_enrich_timeout_seconds", "30")
	rc := robustclient.NewClient(robustclient.DefaultClientConfig())
	rc.RegisterCluster(v1.GetClusterId(w), server.URL)
	cleanup := opensearch.RegisterClientForTest(v1.GetClusterId(w), opensearch.NewClient(opensearch.SearchClientConfig{DefaultIndex: "logs-"}, rc.ForCluster(v1.GetClusterId(w))))
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer r.cicdFailureLogs.ShutDown()
	r.cicdFailureLogs.Run(ctx)
	r.cicdFailureLogs.Run(ctx)
	returned := make(chan error, 1)
	go func() { _, err := r.handleJob(context.Background(), msg, monkeyClientSets()); returned <- err }()
	select {
	case err := <-returned:
		assert.NilError(t, err)
	case <-time.After(time.Second):
		t.Fatal("status handler waited for logs")
	}
	current := &v1.Workload{}
	assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), current))
	assert.Equal(t, current.Status.Phase, v1.WorkloadFailed)
	fallback := current.Status.Message
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("enrichment did not reach HTTP")
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("configured enrichment deadline did not cancel HTTP")
	}
	assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), current))
	assert.Equal(t, current.Status.Message, fallback)
}

func TestCICDFailureEnrichment_ConflictAndRestart(t *testing.T) {
	r, w, snapshot := persistedCICDFailure(t)
	response := cicdErrorResponse(t, w, snapshot.until, "proxy registration rejected")
	installCICDLogSearch(t, v1.GetClusterId(w), func(time.Time, time.Time, string, string, []byte) ([]byte, error) { return response, nil })
	patches := 0
	r.Client = interceptor.NewClient(r.Client.(client.WithWatch), interceptor.Funcs{SubResourcePatch: func(ctx context.Context, cli client.Client, subresource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
		patches++
		if patches == 1 {
			return apierrors.NewConflict(schema.GroupResource{Resource: "workloads"}, w.Name, errors.New("concurrent update"))
		}
		return cli.SubResource(subresource).Patch(ctx, obj, patch, opts...)
	}})
	assert.NilError(t, r.enrichCICDFailureMessage(context.Background(), snapshot))
	assert.Equal(t, patches, 2)
	assert.NilError(t, r.enrichCICDFailureMessage(context.Background(), snapshot))
	assert.Equal(t, patches, 2)
	current := &v1.Workload{}
	assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), current))
	assert.Equal(t, strings.Count(current.Status.Message, " ARC controller: "), 1)
	r.enqueueCICDFailureEnrichment(w)
	r.enqueueCICDFailureEnrichment(w)
	assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 1)
	restarted := &SyncerReconciler{Client: r.Client}
	restarted.newCICDFailureWorker()
	assert.Equal(t, restarted.cicdFailureLogs.GetQueueSize(), 0)
	assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), current))
	assert.Assert(t, strings.TrimSpace(current.Status.Message) != "")
}

func TestCICDFailureEnrichment_DedupSurvivesFailedReconcile(t *testing.T) {
	r, workload, snapshot := persistedCICDFailure(t)

	attempts, remembered := r.cicdFailureAttempts.Load(workload.Name)
	assert.Assert(t, remembered, "the failed reconcile must retain its enrichment attempts")
	_, remembered = attempts.(*sync.Map).Load(cicdFailureKey(snapshot))
	assert.Assert(t, remembered, "the failed reconcile must retain the dispatch dedup key")

	queued := r.cicdFailureLogs.GetQueueSize()
	r.enqueueCICDFailureEnrichment(workload)
	assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), queued)
}

func TestCICDFailureAttemptsCleanup(t *testing.T) {
	for _, mode := range []string{"failed", "no pods", "deleted", "deleting", "undispatched", "missing", "fresh ended"} {
		t.Run(mode, func(t *testing.T) {
			r, w, _ := controllerCICDFailure(t, common.CICDScaleRunnerSetKind)
			defer r.cicdFailureLogs.ShutDown()
			w.Status.Pods = []v1.WorkloadPod{podRecord("example-pod", "example-node", corev1.PodRunning, time.Hour)}
			assert.NilError(t, r.Status().Update(context.Background(), w))
			prior := w.DeepCopy()
			v1.SetAnnotation(prior, v1.WorkloadDispatchedAnnotation, timeutil.FormatRFC3339(time.Now().UTC().Add(-time.Hour)))
			r.enqueueCICDFailureEnrichment(prior)
			r.enqueueCICDFailureEnrichment(w)
			assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 2)
			var keys []any
			r.cicdFailureAttempts.Range(func(key, _ any) bool {
				keys = append(keys, key)
				return true
			})
			assert.Assert(t, len(keys) > 0)
			other := w.DeepCopy()
			other.Name += "-other"
			other.UID += "-other"
			r.enqueueCICDFailureEnrichment(other)
			assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 3)
			message := &resourceMessage{action: ResourceUpdate}
			r.vanishedPodsChecked.Store(w.Name, struct{}{})
			switch mode {
			case "no pods":
				w.Status.Pods = nil
			case "deleted", "deleting":
				w.Status.Phase = v1.WorkloadRunning
				message.action = ResourceDel
				if mode == "deleting" {
					message.action = ResourceDeleting
				}
			case "undispatched":
				w.Status.Phase = v1.WorkloadRunning
				v1.RemoveAnnotation(w, v1.WorkloadDispatchedAnnotation)
			case "missing", "fresh ended":
				w.Status.Phase = v1.WorkloadRunning
				r.vanishedPodsChecked.Delete(w.Name)
				if mode == "missing" {
					assert.NilError(t, r.Delete(context.Background(), w))
				}
			}

			assert.NilError(t, r.reconcileVanishedPods(context.Background(), nil, w, message))
			_, remembered := r.vanishedPodsChecked.Load(w.Name)
			assert.Equal(t, remembered, false)
			retained := mode == "failed" || mode == "no pods" || mode == "fresh ended"
			for _, key := range keys {
				_, remembered = r.cicdFailureAttempts.Load(key)
				assert.Equal(t, remembered, retained)
			}
			remaining := 0
			r.cicdFailureAttempts.Range(func(_, _ any) bool {
				remaining++
				return true
			})
			expectedRemaining := 1
			if retained {
				expectedRemaining++
			}
			assert.Equal(t, remaining, expectedRemaining, "only deleted or reset workload attempts are evicted")
		})
	}
}

func TestCICDFailureEnrichment_DiscardsStaleResults(t *testing.T) {
	for _, mode := range []string{"uid", "dispatch count", "dispatch time", "condition", "phase", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			r, w, snapshot := persistedCICDFailure(t)
			switch mode {
			case "uid":
				w.UID = "new-uid"
				assert.NilError(t, r.Update(context.Background(), w))
			case "dispatch count":
				v1.SetLabel(w, v1.WorkloadDispatchCntLabel, "2")
				assert.NilError(t, r.Update(context.Background(), w))
			case "dispatch time":
				v1.SetAnnotation(w, v1.WorkloadDispatchedAnnotation, timeutil.FormatRFC3339(time.Now()))
				assert.NilError(t, r.Update(context.Background(), w))
			case "condition":
				w.Status.Conditions[0].Message = "new diagnosis"
				assert.NilError(t, r.Status().Update(context.Background(), w))
			case "phase":
				w.Status.Phase = v1.WorkloadRunning
				assert.NilError(t, r.Status().Update(context.Background(), w))
			case "deleted":
				assert.NilError(t, r.Delete(context.Background(), w))
			}
			assert.NilError(t, r.patchCICDFailureMessage(context.Background(), snapshot, " ARC controller: stale detail"))
			if mode != "deleted" {
				current := &v1.Workload{}
				assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), current))
				assert.Assert(t, !strings.Contains(current.Status.Message, "stale detail"))
			}
		})
	}
}

func TestCICDFailureEnrichment_ClientWiring(t *testing.T) {
	commonconfig.SetValue("opensearch.enable", "true")
	defer commonconfig.SetValue("opensearch.enable", "false")
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "discovered-cluster", Annotations: map[string]string{"primus-safe.amd.com/robust-api-endpoint": "http://127.0.0.1:1"}}}
	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	cli := fake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(cluster).Build()
	r := &SyncerReconciler{Client: cli}
	worker := r.newCICDFailureWorker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- worker.Start(ctx) }()
	deadline := time.After(time.Second)
	for opensearch.GetOpensearchClient(cluster.Name) == nil {
		select {
		case <-deadline:
			t.Fatal("discovery did not supply an OpenSearch client")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-stopped:
		assert.NilError(t, err)
	case <-time.After(time.Second):
		t.Fatal("worker shutdown waited for backend")
	}
}

func TestCICDFailureEnrichment_FailedPatchDoesNotEnqueue(t *testing.T) {
	r, w, msg, _ := runnerFailureFixture(t, runnerSetRegistrationTimeout, nil)
	r.Client = interceptor.NewClient(r.Client.(client.WithWatch), interceptor.Funcs{SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
		return apierrors.NewConflict(schema.GroupResource{Resource: "workloads"}, w.Name, errors.New("concurrent status update"))
	}})
	_, err := r.handleJob(context.Background(), msg, monkeyClientSets())
	assert.Assert(t, apierrors.IsConflict(err))
	assert.Equal(t, r.cicdFailureLogs.GetQueueSize(), 0)
}

func TestCICDFailureEnrichment_BoundsDiagnostic(t *testing.T) {
	r, w, snapshot := persistedCICDFailure(t)
	response := cicdErrorResponse(t, w, snapshot.until, strings.Repeat("\u2603", 4096))
	installCICDLogSearch(t, v1.GetClusterId(w), func(time.Time, time.Time, string, string, []byte) ([]byte, error) { return response, nil })
	assert.NilError(t, r.enrichCICDFailureMessage(context.Background(), snapshot))
	current := &v1.Workload{}
	assert.NilError(t, r.Get(context.Background(), client.ObjectKeyFromObject(w), current))
	assert.Equal(t, len([]rune(strings.TrimPrefix(current.Status.Message, w.Status.Message))), 2048)
	assert.Equal(t, current.Status.Conditions[0].Message, current.Status.Message)
}
