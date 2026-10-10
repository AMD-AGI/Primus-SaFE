/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// fakeHarborAPI keeps projects and robot accounts the way Harbor's v2 API does.
type fakeHarborAPI struct {
	mu       sync.Mutex
	projects map[string]map[string]any // name -> created payload, with project_id
	robots   []map[string]any          // with id, name (prefixed) and secret
	writes   []string
	failSet  bool
}

func newFakeHarborAPI() *fakeHarborAPI {
	return &fakeHarborAPI{projects: map[string]map[string]any{}}
}

func (f *fakeHarborAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "pw" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		f.writes = append(f.writes, r.Method+" "+r.URL.Path)
	}
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v2.0/projects/"):
		p, ok := f.projects[strings.TrimPrefix(r.URL.Path, "/api/v2.0/projects/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(p)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v2.0/projects":
		name := in["project_name"].(string)
		if _, ok := f.projects[name]; ok {
			w.WriteHeader(http.StatusConflict)
			return
		}
		in["project_id"] = len(f.projects) + 7
		f.projects[name] = in
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v2.0/robots":
		var out []map[string]any
		for _, rb := range f.robots {
			if r.URL.Query().Get("q") == fmt.Sprintf("Level=project,ProjectID=%v", rb["project_id"]) {
				out = append(out, map[string]any{"id": rb["id"], "name": rb["name"]})
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v2.0/robots":
		perms := in["permissions"].([]any)[0].(map[string]any)
		project := f.projects[perms["namespace"].(string)]
		rb := map[string]any{
			"id": len(f.robots) + 1, "name": "robot$" + perms["namespace"].(string) + "+" + in["name"].(string),
			"secret": "generated", "project_id": project["project_id"], "request": in,
		}
		f.robots = append(f.robots, rb)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": rb["id"], "name": rb["name"], "secret": "generated"})
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v2.0/robots/"):
		if f.failSet {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, rb := range f.robots {
			if fmt.Sprint(rb["id"]) == strings.TrimPrefix(r.URL.Path, "/api/v2.0/robots/") {
				rb["secret"] = in["secret"]
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func stagingSecret(t *testing.T, cl client.Client) (*corev1.Secret, error) {
	t.Helper()
	s := &corev1.Secret{}
	err := cl.Get(context.Background(), client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: common.SaveImageStagingSecretName}, s)
	return s, err
}

func TestEnsureStagingCreatesEverythingOnceAndAgrees(t *testing.T) {
	api := newFakeHarborAPI()
	ts := httptest.NewServer(api)
	defer ts.Close()
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	ctx := context.Background()

	require.NoError(t, h.ensureStaging(ctx, "harbor.example.com", hostFromServer(ts), "admin", "pw"))
	// A private project.
	require.Contains(t, api.projects, SaveImageStagingProject)
	assert.Equal(t, map[string]any{"public": "false"}, api.projects[SaveImageStagingProject]["metadata"])
	// A robot that can push to and pull from that project alone, and does not expire.
	require.Len(t, api.robots, 1)
	req := api.robots[0]["request"].(map[string]any)
	assert.Equal(t, "project", req["level"])
	assert.EqualValues(t, -1, req["duration"])
	assert.JSONEq(t, `[{"kind":"project","namespace":"save-staging","access":[
		{"resource":"repository","action":"push"},{"resource":"repository","action":"pull"}]}]`, mustJSON(req["permissions"]))
	// The Secret holds the robot's name and the secret Harbor was given.
	s, err := stagingSecret(t, cl)
	require.NoError(t, err)
	assert.Equal(t, corev1.SecretTypeDockerConfigJson, s.Type)
	var cfg struct {
		Auths map[string]struct{ Username, Password, Auth string } `json:"auths"`
	}
	require.NoError(t, json.Unmarshal(s.Data[corev1.DockerConfigJsonKey], &cfg))
	a := cfg.Auths["harbor.example.com"]
	assert.Equal(t, "robot$save-staging+save-image-staging", a.Username)
	assert.Equal(t, api.robots[0]["secret"], a.Password)
	assert.NotEqual(t, "generated", a.Password)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(a.Username+":"+a.Password)), a.Auth)

	// Again: nothing is written, and the Secret stays.
	writes := len(api.writes)
	require.NoError(t, h.ensureStaging(ctx, "harbor.example.com", hostFromServer(ts), "admin", "pw"))
	assert.Len(t, api.writes, writes)
	again, err := stagingSecret(t, cl)
	require.NoError(t, err)
	assert.Equal(t, s.Data, again.Data)
}

// A Secret that exists, an administrator's for instance, is never overwritten.
func TestEnsureStagingLeavesAnExistingSecret(t *testing.T) {
	api := newFakeHarborAPI()
	ts := httptest.NewServer(api)
	defer ts.Close()
	mine := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: common.PrimusSafeNamespace, Name: common.SaveImageStagingSecretName},
		Data:       map[string][]byte{"config.json": []byte(`{"auths":{}}`)},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).WithObjects(mine).Build()
	h := &ImageHandler{Client: cl}
	require.NoError(t, h.ensureStaging(context.Background(), "harbor.example.com", hostFromServer(ts), "admin", "pw"))
	assert.Empty(t, api.robots)
	s, err := stagingSecret(t, cl)
	require.NoError(t, err)
	assert.Equal(t, mine.Data, s.Data)
}

// The robot already exists (its Secret was deleted): its secret is replaced with one
// that is then stored.
func TestEnsureStagingReusesTheRobot(t *testing.T) {
	api := newFakeHarborAPI()
	ts := httptest.NewServer(api)
	defer ts.Close()
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	ctx := context.Background()
	require.NoError(t, h.ensureStaging(ctx, "harbor.example.com", hostFromServer(ts), "admin", "pw"))
	s, err := stagingSecret(t, cl)
	require.NoError(t, err)
	require.NoError(t, cl.Delete(ctx, s))

	require.NoError(t, h.ensureStaging(ctx, "harbor.example.com", hostFromServer(ts), "admin", "pw"))
	assert.Len(t, api.robots, 1)
	s, err = stagingSecret(t, cl)
	require.NoError(t, err)
	assert.Contains(t, string(s.Data[corev1.DockerConfigJsonKey]), api.robots[0]["secret"].(string))
}

// A robot whose secret could not be set leaves no Secret behind: the next start tries
// again instead of keeping a credential Harbor does not know.
func TestEnsureStagingRemovesTheSecretWhenTheRobotCannotBeSet(t *testing.T) {
	api := newFakeHarborAPI()
	api.failSet = true
	ts := httptest.NewServer(api)
	defer ts.Close()
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	require.Error(t, h.ensureStaging(context.Background(), "harbor.example.com", hostFromServer(ts), "admin", "pw"))
	_, err := stagingSecret(t, cl)
	assert.True(t, client.IgnoreNotFound(err) == nil && err != nil, "no secret")
}

func TestEnsureStagingReportsAnUnreachableHarbor(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	err := h.ensureStaging(context.Background(), "harbor.example.com", hostFromServer(ts), "admin", "pw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

// The project saved images are pushed to is made by the same retried step as the staging
// project: a Harbor that is down when the apiserver starts gets it on a later attempt.
func TestEnsureStagingCreatesTheExportProjectOnceHarborIsUp(t *testing.T) {
	api := newFakeHarborAPI()
	down := true
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		api.ServeHTTP(w, r)
	}))
	defer ts.Close()
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	ctx := context.Background()

	require.Error(t, h.ensureStaging(ctx, "harbor.example.com", hostFromServer(ts), "admin", "pw"))
	down = false
	require.NoError(t, h.ensureStaging(ctx, "harbor.example.com", hostFromServer(ts), "admin", "pw"))
	require.Contains(t, api.projects, common.ExportImageProject)
	assert.Equal(t, map[string]any{"public": "true"}, api.projects[common.ExportImageProject]["metadata"])
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// A built-in Harbor that is not installed yet when the apiserver starts looks, at that
// moment, exactly like none at all: its ConfigMap is missing. The step is retried in both
// cases, so the staging project, robot and Secret are made once Harbor comes up, without
// restarting the apiserver.
func TestKeepEnsuringStagingRetriesWhileHarborIsMissing(t *testing.T) {
	defer func(w time.Duration) { saveStagingFirstWait = w }(saveStagingFirstWait)
	saveStagingFirstWait = time.Millisecond
	var mu sync.Mutex
	lookups := 0
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if key.Namespace == "harbor" && key.Name == "harbor-core" {
				if _, ok := obj.(*corev1.ConfigMap); ok {
					mu.Lock()
					lookups++
					mu.Unlock()
				}
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	h := &ImageHandler{Client: cl}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.keepEnsuringSaveImageStaging(ctx); close(done) }()
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return lookups >= 3
	}, 5*time.Second, time.Millisecond, "Harbor is looked for again after it was missing")
	select {
	case <-done:
		t.Fatal("the loop gave up while Harbor was missing")
	default:
	}
	cancel()
	<-done
}

// A built-in Harbor that comes up after the apiserver is set up whole once it is there:
// it is registered as the default registry (which saving an image pushes to) as well as
// given the staging project, robot and Secret, without restarting the apiserver.
func TestKeepSettingUpHarborRegistersAHarborThatComesUpLater(t *testing.T) {
	defer func(w time.Duration) { saveStagingFirstWait = w }(saveStagingFirstWait)
	saveStagingFirstWait = time.Millisecond
	var mu sync.Mutex
	up, attempts, registeredUp := false, 0, 0
	register := func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		if up {
			registeredUp++
		}
		return nil // initHarbor does nothing without a Harbor
	}
	staging := func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if !up {
			return errNoBuiltinHarbor
		}
		return nil
	}
	done := make(chan struct{})
	go func() { keepSettingUpHarbor(context.Background(), register, staging); close(done) }()
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return attempts >= 2
	}, 5*time.Second, time.Millisecond)
	mu.Lock()
	up = true
	mu.Unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the built-in Harbor was never set up")
	}
	assert.Equal(t, 1, registeredUp, "registered once Harbor is there")
}
