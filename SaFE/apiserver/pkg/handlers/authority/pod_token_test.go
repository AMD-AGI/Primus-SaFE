/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

const (
	ptWorkspace = "ws-a"
	ptCluster   = "cluster-a"
	ptPod       = "runner-set-abc-runner-xyz"
	ptPodUID    = "pod-uid-1"
	ptScaleSet  = "scale-set-1"
	ptOwner     = "owner-id"
)

// ptEnv is a cluster that answers TokenReviews, a runner pod in it, and the admin cluster's
// records of the workspace and of the CI/CD workload the pod belongs to.
type ptEnv struct {
	t       *testing.T
	review  func(token string) (*authenticationv1.TokenReview, error)
	reviews atomic.Int32
	cs      *kubefake.Clientset
	objs    []client.Object
	funcs   interceptor.Funcs
	cluster func(string) (kubernetes.Interface, error)
	// workloadGets are the names of the workloads read from the admin cluster; podGets counts
	// the reads of a pod from the cluster.
	workloadGets []string
	podGets      atomic.Int32
}

func ptScaleSetWorkload() *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: ptScaleSet, Labels: map[string]string{v1.UserIdLabel: ptOwner}},
		Spec: v1.WorkloadSpec{
			Workspace:        ptWorkspace,
			GroupVersionKind: v1.GroupVersionKind{Kind: common.CICDScaleRunnerSetKind, Version: "v1"},
		},
	}
}

func ptChild(name, pod, owner string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: owner}},
		Spec: v1.WorkloadSpec{
			Workspace:        ptWorkspace,
			GroupVersionKind: v1.GroupVersionKind{Kind: common.CICDEphemeralRunnerKind, Version: "v1"},
			Env:              map[string]string{common.ScaleRunnerID: pod},
		},
	}
}

func ptPodObject() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: ptPod, Namespace: ptWorkspace, UID: types.UID(ptPodUID)},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "unified-job", Env: []corev1.EnvVar{{Name: common.ScaleRunnerSetID}}}, // set empty: skipped
			{Name: "runner", Env: []corev1.EnvVar{{Name: "POD_NAME", Value: "x"},
				{Name: common.ScaleRunnerSetID, Value: ptScaleSet}}},
		}},
	}
}

func ptAccepted(namespace, pod, uid string) *authenticationv1.TokenReview {
	return &authenticationv1.TokenReview{Status: authenticationv1.TokenReviewStatus{
		Authenticated: true,
		Audiences:     []string{common.PodTokenAudience},
		User: authenticationv1.UserInfo{
			Username: "system:serviceaccount:" + namespace + ":gha-rs-manager-no-permission",
			Extra: map[string]authenticationv1.ExtraValue{
				podNameExtra: {pod}, podUIDExtra: {uid},
			},
		},
	}}
}

// newPtEnv builds the environment and installs the pod token authentication for the test.
func newPtEnv(t *testing.T, extra ...client.Object) *ptEnv {
	t.Helper()
	e := &ptEnv{t: t}
	e.review = func(string) (*authenticationv1.TokenReview, error) {
		return ptAccepted(ptWorkspace, ptPod, ptPodUID), nil
	}
	e.objs = append([]client.Object{
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: ptWorkspace}, Spec: v1.WorkspaceSpec{Cluster: ptCluster}},
		ptScaleSetWorkload(),
	}, extra...)
	return e
}

func (e *ptEnv) install(pod *corev1.Pod) {
	e.t.Helper()
	e.cs = kubefake.NewSimpleClientset()
	if pod != nil {
		_, err := e.cs.CoreV1().Pods(ptWorkspace).Create(context.Background(), pod, metav1.CreateOptions{})
		require.NoError(e.t, err)
	}
	e.cs.PrependReactor("create", "tokenreviews", func(action clienttesting.Action) (bool, runtime.Object, error) {
		e.reviews.Add(1)
		tr := action.(clienttesting.CreateAction).GetObject().(*authenticationv1.TokenReview)
		require.Equal(e.t, []string{common.PodTokenAudience}, tr.Spec.Audiences, "the review names the audience")
		rsp, err := e.review(tr.Spec.Token)
		return true, rsp, err
	})
	scheme := runtime.NewScheme()
	require.NoError(e.t, v1.AddToScheme(scheme))
	e.cs.PrependReactor("get", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		e.podGets.Add(1)
		return false, nil, nil
	})
	funcs := e.funcs
	inner := funcs.Get
	funcs.Get = func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object,
		opts ...client.GetOption) error {
		if _, ok := obj.(*v1.Workload); ok {
			e.workloadGets = append(e.workloadGets, key.Name)
		}
		if inner != nil {
			return inner(ctx, c, key, obj, opts...)
		}
		return c.Get(ctx, key, obj, opts...)
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(e.objs...).WithInterceptorFuncs(funcs).Build()
	clusters := func(id string) (kubernetes.Interface, error) {
		if e.cluster != nil {
			return e.cluster(id)
		}
		require.Equal(e.t, ptCluster, id, "the review goes to the cluster of the workspace")
		return e.cs, nil
	}
	InitPodTokenAuth(reader, clusters)
	e.t.Cleanup(func() { podTokenState.Store(nil) })
}

// ptToken makes the shape of a ServiceAccount token. Its signature is never checked here: the
// cluster's answer to the review is what decides.
func ptToken(namespace string, aud ...string) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	claims := map[string]any{"aud": aud, "kubernetes.io": map[string]any{"namespace": namespace}}
	return enc(map[string]string{"alg": "RS256"}) + "." + enc(claims) + "." + enc("signature")
}

func goodToken() string { return ptToken(ptWorkspace, common.PodTokenAudience) }

// ptRouter registers the routes of the workloads group (and one more) behind ParseToken, the
// way the apiserver does, and answers with what the request was authenticated as.
func ptRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	g := e.Group(common.PrimusRouterCustomRootPath, func(c *gin.Context) {
		if err := ParseToken(c); err != nil {
			code := http.StatusInternalServerError
			var st apierrors.APIStatus
			if errors.As(err, &st) {
				code = int(st.Status().Code)
			}
			c.AbortWithStatusJSON(code, gin.H{"error": err.Error()})
			return
		}
	})
	echo := func(c *gin.Context) {
		out := gin.H{"user": c.GetString(common.UserId), "type": c.GetString(common.UserType),
			"actor": c.GetString(common.ActorClientId)}
		if id := PodIdentityFrom(c); id != nil {
			out["pod"] = id.Pod
		}
		c.JSON(http.StatusOK, out)
	}
	g.POST("/workloads", echo)
	g.GET("/workloads", echo)
	g.GET("/workloads/:name", echo)
	g.DELETE("/workloads/:name", echo)
	g.POST("/workloads/:name/stop", echo)
	g.POST("/workloads/:name/resume", echo)
	g.GET("/users", echo)
	return e
}

func ptDo(method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	ptRouter().ServeHTTP(rec, req)
	return rec
}

func ptBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

func TestPodTokenActsAsTheOwnerOfTheScaleSet(t *testing.T) {
	e := newPtEnv(t, ptChild("child-1", ptPod, ptOwner))
	e.install(ptPodObject())

	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/workloads"},
		{"GET", "/api/v1/workloads/child-1"},
		{"POST", "/api/v1/workloads/child-1/stop"},
	} {
		rec := ptDo(tc.method, tc.path, goodToken())
		require.Equal(t, http.StatusOK, rec.Code, tc.method+" "+tc.path+": "+rec.Body.String())
		body := ptBody(t, rec)
		require.Equal(t, ptOwner, body["user"])
		require.Equal(t, UserTypePod, body["type"])
		require.Equal(t, ptPod, body["pod"])
		require.Equal(t, "pod:"+ptWorkspace+"/"+ptPod, body["actor"], "the audit log shows which pod acted")
	}
}

func TestPodTokenIsHeldToTheThreeRoutes(t *testing.T) {
	e := newPtEnv(t, ptChild("child-1", ptPod, ptOwner))
	e.install(ptPodObject())
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/workloads"},
		{"DELETE", "/api/v1/workloads/child-1"},
		{"POST", "/api/v1/workloads/child-1/resume"},
		{"GET", "/api/v1/users"},
	} {
		rec := ptDo(tc.method, tc.path, goodToken())
		require.Equal(t, http.StatusForbidden, rec.Code, tc.method+" "+tc.path)
	}
	require.Zero(t, e.reviews.Load(), "a route outside the three is refused before any review")
}

func TestPodTokenReadsAndStopsOnlyWhatItsOwnPodCreated(t *testing.T) {
	e := newPtEnv(t,
		ptChild("other-pod", "another-runner", ptOwner),
		ptChild("other-owner", ptPod, "someone-else"),
		&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "unmarked", Labels: map[string]string{v1.UserIdLabel: ptOwner}}},
	)
	e.install(ptPodObject())
	for _, name := range []string{"other-pod", "other-owner", "unmarked"} {
		require.Equal(t, http.StatusForbidden, ptDo("GET", "/api/v1/workloads/"+name, goodToken()).Code, name)
		require.Equal(t, http.StatusForbidden, ptDo("POST", "/api/v1/workloads/"+name+"/stop", goodToken()).Code, name)
	}
	// A workload that does not exist is the handler's to answer (not found).
	require.Equal(t, http.StatusOK, ptDo("GET", "/api/v1/workloads/gone", goodToken()).Code)
}

func TestPodTokenRefusals(t *testing.T) {
	notScaleSet := ptScaleSetWorkload()
	notScaleSet.Spec.Kind = common.CICDEphemeralRunnerKind
	otherWorkspace := ptScaleSetWorkload()
	otherWorkspace.Spec.Workspace = "ws-b"
	noOwner := ptScaleSetWorkload()
	noOwner.Labels = nil
	unlabeled := ptPodObject()
	unlabeled.Spec.Containers = []corev1.Container{{Name: "runner", Env: []corev1.EnvVar{
		{Name: common.ScaleRunnerSetID}, // not set
		{Name: "SOMETHING_ELSE", Value: ptScaleSet},
	}}}
	otherUID := ptPodObject()
	otherUID.UID = "another-uid"

	review := func(f func(*authenticationv1.TokenReview)) func(string) (*authenticationv1.TokenReview, error) {
		return func(string) (*authenticationv1.TokenReview, error) {
			r := ptAccepted(ptWorkspace, ptPod, ptPodUID)
			f(r)
			return r, nil
		}
	}
	cases := []struct {
		name    string
		review  func(string) (*authenticationv1.TokenReview, error)
		pod     *corev1.Pod
		replace *v1.Workload
		token   string
		// noPodRead and noWorkloadRead say the refusal must come before the pod, and the
		// workload of the pod, are read: the check that refuses is the one named, not a
		// later one that would have refused for another reason.
		noPodRead, noWorkloadRead bool
	}{
		{name: "the cluster rejects the token", pod: ptPodObject(),
			review: review(func(r *authenticationv1.TokenReview) { r.Status.Authenticated = false })},
		{name: "the review carries an error", pod: ptPodObject(),
			review: review(func(r *authenticationv1.TokenReview) { r.Status.Error = "boom" })},
		{name: "the review does not confirm the audience", pod: ptPodObject(),
			review: review(func(r *authenticationv1.TokenReview) { r.Status.Audiences = []string{"other"} })},
		{name: "the token is not bound to a pod", pod: ptPodObject(), noPodRead: true,
			review: review(func(r *authenticationv1.TokenReview) { r.Status.User.Extra = nil })},
		{name: "the token has no pod uid", pod: ptPodObject(), noPodRead: true,
			review: review(func(r *authenticationv1.TokenReview) { delete(r.Status.User.Extra, podUIDExtra) })},
		{name: "the ServiceAccount is of another namespace", pod: ptPodObject(),
			review: review(func(r *authenticationv1.TokenReview) {
				r.Status.User.Username = "system:serviceaccount:kube-system:default"
			})},
		{name: "the user is not a ServiceAccount", pod: ptPodObject(),
			review: review(func(r *authenticationv1.TokenReview) { r.Status.User.Username = "admin" })},
		{name: "the pod is gone", pod: nil},
		{name: "the pod is another one than the token is bound to", pod: otherUID, noWorkloadRead: true},
		{name: "the pod belongs to no workload", pod: unlabeled, noWorkloadRead: true},
		{name: "the workload of the pod is not a scale set", pod: ptPodObject(), replace: notScaleSet},
		{name: "the scale set is of another workspace", pod: ptPodObject(), replace: otherWorkspace},
		{name: "the scale set has no owner", pod: ptPodObject(), replace: noOwner},
		{name: "the workspace is unknown", pod: ptPodObject(), token: ptToken("ws-unknown", common.PodTokenAudience)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newPtEnv(t)
			if tc.review != nil {
				e.review = tc.review
			}
			if tc.replace != nil {
				e.objs[1] = tc.replace
			}
			e.install(tc.pod)
			token := goodToken()
			if tc.token != "" {
				token = tc.token
			}
			rec := ptDo("POST", "/api/v1/workloads", token)
			require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			if tc.noPodRead {
				require.Zero(t, e.podGets.Load())
			}
			if tc.noWorkloadRead {
				require.Empty(t, e.workloadGets)
			}
		})
	}
}

func TestPodTokenNeverPassesWhenItCannotBeVerified(t *testing.T) {
	t.Run("the review fails", func(t *testing.T) {
		e := newPtEnv(t)
		e.review = func(string) (*authenticationv1.TokenReview, error) { return nil, errors.New("connection refused") }
		e.install(ptPodObject())
		require.Equal(t, http.StatusServiceUnavailable, ptDo("POST", "/api/v1/workloads", goodToken()).Code)
	})
	t.Run("the cluster has no client", func(t *testing.T) {
		e := newPtEnv(t)
		e.cluster = func(string) (kubernetes.Interface, error) { return nil, errors.New("the client is not found") }
		e.install(ptPodObject())
		require.Equal(t, http.StatusServiceUnavailable, ptDo("POST", "/api/v1/workloads", goodToken()).Code)
	})
	t.Run("the pod cannot be read", func(t *testing.T) {
		e := newPtEnv(t)
		e.install(ptPodObject())
		e.cs.PrependReactor("get", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("timeout")
		})
		require.Equal(t, http.StatusServiceUnavailable, ptDo("POST", "/api/v1/workloads", goodToken()).Code)
	})
	// failGet fails the read of one kind of record, the other reads go on.
	failGet := func(fail func(client.Object) bool) interceptor.Funcs {
		return interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
			obj client.Object, opts ...client.GetOption) error {
			if fail(obj) {
				return errors.New("etcd timeout")
			}
			return c.Get(ctx, key, obj, opts...)
		}}
	}
	isWorkspace := func(o client.Object) bool { _, ok := o.(*v1.Workspace); return ok }
	isWorkload := func(o client.Object) bool { _, ok := o.(*v1.Workload); return ok }
	t.Run("the workspace cannot be read", func(t *testing.T) {
		e := newPtEnv(t)
		e.funcs = failGet(isWorkspace)
		e.install(ptPodObject())
		require.Equal(t, http.StatusServiceUnavailable, ptDo("POST", "/api/v1/workloads", goodToken()).Code)
	})
	t.Run("the workload of the pod cannot be read", func(t *testing.T) {
		e := newPtEnv(t)
		e.funcs = failGet(isWorkload)
		e.install(ptPodObject())
		require.Equal(t, http.StatusServiceUnavailable, ptDo("POST", "/api/v1/workloads", goodToken()).Code)
	})
	t.Run("the target workload cannot be read", func(t *testing.T) {
		e := newPtEnv(t, ptChild("child-1", ptPod, ptOwner))
		calls := 0
		e.funcs = interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
			obj client.Object, opts ...client.GetOption) error {
			// The scale set is read first; the target of the request is the second read.
			if _, ok := obj.(*v1.Workload); ok {
				if calls++; calls == 2 {
					return errors.New("etcd timeout")
				}
			}
			return c.Get(ctx, key, obj, opts...)
		}}
		e.install(ptPodObject())
		require.Equal(t, http.StatusServiceUnavailable, ptDo("GET", "/api/v1/workloads/child-1", goodToken()).Code)
	})
	t.Run("the authentication is not initialised", func(t *testing.T) {
		podTokenState.Store(nil)
		require.Equal(t, http.StatusUnauthorized, ptDo("POST", "/api/v1/workloads", goodToken()).Code)
	})
}

// Only a token for the apiserver audience is looked at; any other credential goes on to the
// modes of the apiserver exactly as before.
func TestPodTokenLeavesOtherCredentialsAlone(t *testing.T) {
	e := newPtEnv(t)
	e.install(ptPodObject())
	for name, token := range map[string]string{
		"an API key":                  "ak-abcdef",
		"another audience":            ptToken(ptWorkspace, "https://kubernetes.default.svc"),
		"a token without a namespace": ptToken("", common.PodTokenAudience),
		"not a JWT":                   "a.b",
		"garbage payload":             "a.!!!.c",
	} {
		rec := ptDo("POST", "/api/v1/workloads", token)
		require.NotEqual(t, http.StatusOK, rec.Code, name)
	}
	require.Zero(t, e.reviews.Load(), "no review for a credential that is not a pod token")
}

// In the external mode the chain of authenticators would not know a ServiceAccount token; the
// pod token is decided before it, in both modes.
func TestPodTokenWorksInTheExternalMode(t *testing.T) {
	e := newPtEnv(t)
	e.install(ptPodObject())
	commonconfig.SetValue("auth.mode", "external")
	t.Cleanup(func() { commonconfig.SetValue("auth.mode", "") })
	externalState.Store(&externalAuth{chain: extauth.NewChain()})
	t.Cleanup(func() { externalState.Store(nil) })

	require.Equal(t, http.StatusOK, ptDo("POST", "/api/v1/workloads", goodToken()).Code)
	// The chain itself is in force for everything else.
	require.Equal(t, http.StatusUnauthorized, ptDo("POST", "/api/v1/workloads", "opaque-token").Code)
}

func TestScopeCreate(t *testing.T) {
	id := &PodIdentity{Pod: ptPod, Workspace: ptWorkspace, Workload: ptScaleSetWorkload()}
	mk := func(kind, workspace string) *v1.Workload {
		return &v1.Workload{Spec: v1.WorkloadSpec{Workspace: workspace,
			GroupVersionKind: v1.GroupVersionKind{Kind: kind, Version: "v1"}}}
	}
	for _, kind := range []string{common.CICDEphemeralRunnerKind, common.UnifiedJobKind} {
		w := mk(kind, ptWorkspace)
		w.Spec.Env = map[string]string{common.ScaleRunnerID: "forged-pod"}
		require.NoError(t, id.ScopeCreate(w))
		require.Equal(t, ptPod, w.Spec.Env[common.ScaleRunnerID], "the pod is stamped by the apiserver, not by the caller")
		require.Equal(t, ptScaleSet, w.Spec.Env[common.ScaleRunnerSetID])
	}
	w := mk(common.UnifiedJobKind, ptWorkspace)
	require.NoError(t, id.ScopeCreate(w), "a workload without env gets the markers")
	require.Equal(t, ptPod, w.Spec.Env[common.ScaleRunnerID])

	for _, kind := range []string{"PyTorchJob", common.CICDScaleRunnerSetKind, common.PodKind, ""} {
		require.Error(t, id.ScopeCreate(mk(kind, ptWorkspace)), kind)
	}
	require.Error(t, id.ScopeCreate(mk(common.UnifiedJobKind, "ws-b")), "another workspace")
}
