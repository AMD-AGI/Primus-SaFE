/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

// A CI/CD runner pod (the scale-set pod that runs the runner proxy and the unified-job
// proxy) calls the apiserver to create, watch and stop the workload that does the work.
// It authenticates the way a Kubernetes pod does: with the projected ServiceAccount token
// that the kubelet mounts into it (audience common.PodTokenAudience, bound to the pod,
// renewed before it expires). The apiserver has the cluster that issued the token verify
// it (TokenReview), reads which pod it is bound to, and takes the owner of the CI/CD
// workload that pod belongs to as the user. The token reaches three routes only.

const (
	// UserTypePod is the user type recorded for a request made with a runner pod's token.
	UserTypePod = "pod"

	podNameExtra   = "authentication.kubernetes.io/pod-name"
	podUIDExtra    = "authentication.kubernetes.io/pod-uid"
	podIdentityKey = "podIdentity"
)

// PodTokenClusters returns the Kubernetes client of a data-plane cluster.
type PodTokenClusters func(clusterId string) (kubernetes.Interface, error)

type podTokenAuth struct {
	reader   client.Reader
	clusters PodTokenClusters
}

var podTokenState atomic.Pointer[podTokenAuth]

// InitPodTokenAuth enables the authentication of runner pods by their ServiceAccount token.
func InitPodTokenAuth(reader client.Reader, clusters PodTokenClusters) {
	podTokenState.Store(&podTokenAuth{reader: reader, clusters: clusters})
}

// PodIdentity says which runner pod a request came from and which CI/CD workload it serves.
type PodIdentity struct {
	// Pod is the name of the runner pod, Workspace its namespace.
	Pod       string
	Workspace string
	// Workload is the CI/CD scale set the pod belongs to; its owner is the user of the request.
	Workload *v1.Workload
}

// SetPodIdentity records which runner pod made the request.
func SetPodIdentity(c *gin.Context, id *PodIdentity) { c.Set(podIdentityKey, id) }

// PodIdentityFrom returns the identity of the runner pod that made the request, or nil when
// the request was not authenticated with a pod token.
func PodIdentityFrom(c *gin.Context) *PodIdentity {
	if v, ok := c.Get(podIdentityKey); ok {
		id, _ := v.(*PodIdentity)
		return id
	}
	return nil
}

// ScopeCreate holds a workload created with a pod token to what the runner proxies create:
// a runner or a unified job in the workspace of the scale set. The workload is marked with
// the pod that created it, which is what later lets the same pod (and no other) read and
// stop it.
func (p *PodIdentity) ScopeCreate(w *v1.Workload) error {
	if kind := w.SpecKind(); kind != common.CICDEphemeralRunnerKind && kind != common.UnifiedJobKind {
		return commonerrors.NewForbidden(fmt.Sprintf("a pod token cannot create a workload of kind %s", kind))
	}
	if w.Spec.Workspace != p.Workspace {
		return commonerrors.NewForbidden("a pod token creates workloads in the workspace of its own pod only")
	}
	if w.Spec.Env == nil {
		w.Spec.Env = map[string]string{}
	}
	w.Spec.Env[common.ScaleRunnerSetID] = p.Workload.Name
	w.Spec.Env[common.ScaleRunnerID] = p.Pod
	return nil
}

// podTokenRoute says whether a pod token may reach a route (create the workload, read it,
// stop it) and whether the route creates one.
func podTokenRoute(method, route string) (create bool, ok bool) {
	root := "/" + common.PrimusRouterCustomRootPath + "/workloads"
	switch {
	case method == "POST" && route == root:
		return true, true
	case method == "GET" && route == root+"/:"+common.Name:
		return false, true
	case method == "POST" && route == root+"/:"+common.Name+"/stop":
		return false, true
	}
	return false, false
}

// podTokenNamespace returns the namespace a ServiceAccount token claims, when the token is
// one for the apiserver audience. Nothing is verified here: the claim only says which
// cluster to ask, and the cluster's own answer decides.
func podTokenNamespace(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims struct {
		Aud        json.RawMessage `json:"aud"`
		Kubernetes struct {
			Namespace string `json:"namespace"`
		} `json:"kubernetes.io"`
	}
	if err = json.Unmarshal(payload, &claims); err != nil || claims.Kubernetes.Namespace == "" {
		return "", false
	}
	var audiences []string
	if err = json.Unmarshal(claims.Aud, &audiences); err != nil {
		var single string
		if err = json.Unmarshal(claims.Aud, &single); err != nil {
			return "", false
		}
		audiences = []string{single}
	}
	if !slices.Contains(audiences, common.PodTokenAudience) {
		return "", false
	}
	return claims.Kubernetes.Namespace, true
}

// parsePodToken authenticates a request that carries a runner pod's ServiceAccount token.
// handled is false when the credential is not one, and the caller goes on.
func parsePodToken(c *gin.Context) (handled bool, err error) {
	token := getBearerToken(c)
	namespace, ok := podTokenNamespace(token)
	if !ok {
		return false, nil
	}
	st := podTokenState.Load()
	if st == nil {
		// Not enabled (not the apiserver): the credential is judged as any other, and a
		// ServiceAccount token is none of the credentials the other modes accept.
		return false, nil
	}
	return true, st.authenticate(c, token, namespace)
}

func (st *podTokenAuth) authenticate(c *gin.Context, token, namespace string) error {
	create, allowed := podTokenRoute(c.Request.Method, c.FullPath())
	if !allowed {
		apimetrics.AuthFailuresTotal.WithLabelValues("pod_token_scope").Inc()
		return commonerrors.NewForbidden("this operation is not available with a pod token")
	}
	ctx := c.Request.Context()
	unauthorized := func(reason string) error {
		klog.Warningf("pod token refused: %s, namespace %s", reason, namespace)
		apimetrics.AuthFailuresTotal.WithLabelValues("pod_token_invalid").Inc()
		return commonerrors.NewUnauthorized(ErrInvalidToken)
	}
	unavailable := func(what string, err error) error {
		klog.ErrorS(err, "pod token cannot be verified", "what", what, "namespace", namespace)
		apimetrics.AuthFailuresTotal.WithLabelValues("unavailable").Inc()
		// Never a pass: the caller is told to retry, not that its credential is bad.
		return apierrors.NewServiceUnavailable("the pod token cannot be verified now")
	}

	workspace := &v1.Workspace{}
	if err := st.reader.Get(ctx, client.ObjectKey{Name: namespace}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			return unauthorized("unknown workspace")
		}
		return unavailable("workspace", err)
	}
	clientSet, err := st.clusters(workspace.Spec.Cluster)
	if err != nil {
		return unavailable("cluster client", err)
	}

	review, err := clientSet.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{Token: token, Audiences: []string{common.PodTokenAudience}},
	}, metav1.CreateOptions{})
	if err != nil {
		return unavailable("token review", err)
	}
	if !review.Status.Authenticated || review.Status.Error != "" ||
		!slices.Contains(review.Status.Audiences, common.PodTokenAudience) {
		return unauthorized("the cluster rejected the token")
	}
	// The token must be one a kubelet issued for a pod: bound to it, so that it dies with
	// it. An unbound ServiceAccount token never expires and is not accepted.
	if !strings.HasPrefix(review.Status.User.Username, "system:serviceaccount:"+namespace+":") {
		return unauthorized("the token is not a ServiceAccount token of the workspace")
	}
	podName := firstExtra(review.Status.User.Extra, podNameExtra)
	podUID := firstExtra(review.Status.User.Extra, podUIDExtra)
	if podName == "" || podUID == "" {
		return unauthorized("the token is not bound to a pod")
	}
	pod, err := clientSet.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return unauthorized("the pod is gone")
		}
		return unavailable("pod", err)
	}
	if string(pod.UID) != podUID {
		return unauthorized("the pod is not the one the token is bound to")
	}
	workloadId := podScaleSetId(pod)
	if workloadId == "" {
		return unauthorized("the pod belongs to no workload")
	}
	workload := &v1.Workload{}
	if err = st.reader.Get(ctx, client.ObjectKey{Name: workloadId}, workload); err != nil {
		if apierrors.IsNotFound(err) {
			return unauthorized("the workload of the pod is gone")
		}
		return unavailable("workload", err)
	}
	owner := v1.GetUserId(workload)
	if !commonworkload.IsCICDScalingRunnerSet(workload) || workload.Spec.Workspace != namespace || owner == "" {
		return unauthorized("the pod does not belong to a CI/CD runner set of the workspace")
	}

	if !create {
		// Read or stop: only what this pod created.
		target := &v1.Workload{}
		name := strings.TrimSpace(c.Param(common.Name))
		err = st.reader.Get(ctx, client.ObjectKey{Name: name}, target)
		switch {
		case err == nil:
			if target.Spec.Env[common.ScaleRunnerID] != podName || v1.GetUserId(target) != owner {
				apimetrics.AuthFailuresTotal.WithLabelValues("pod_token_scope").Inc()
				return commonerrors.NewForbidden("a pod token reaches the workloads its own pod created only")
			}
		case apierrors.IsNotFound(err):
			// Nothing to protect; the handler answers not found.
		default:
			return unavailable("target workload", err)
		}
	}

	SetPodIdentity(c, &PodIdentity{Pod: podName, Workspace: namespace, Workload: workload})
	c.Set(common.UserId, owner)
	c.Set(common.UserName, v1.GetUserName(workload))
	c.Set(common.UserType, UserTypePod)
	// The party acting for the user, so that the audit log shows which pod it was.
	c.Set(common.ActorClientId, "pod:"+namespace+"/"+podName)
	klog.Infof("pod token accepted: pod %s/%s acts for user %s", namespace, podName, owner)
	return nil
}

// podScaleSetId is the CI/CD scale set a runner pod belongs to: the id the dispatcher puts in the
// env of its containers, which is also what the runner proxy reads to name its workload.
func podScaleSetId(pod *corev1.Pod) string {
	for _, container := range pod.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == common.ScaleRunnerSetID && env.Value != "" {
				return env.Value
			}
		}
	}
	return ""
}

func firstExtra(extra map[string]authenticationv1.ExtraValue, key string) string {
	if values := extra[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}
