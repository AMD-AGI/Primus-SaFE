/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/authn"
	gcrname "github.com/google/go-containerregistry/pkg/name"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonctrl "github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/ops_job/exportimage"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

func exportJob(name, workloadId, image string) *v1.OpsJob {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobExportImageType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterWorkload, Value: workloadId},
				{Name: v1.ParameterImage, Value: image},
			},
		},
		Status: v1.OpsJobStatus{Phase: v1.OpsJobRunning},
	}
	return job
}

func TestGetWorkloadIdAndSourceImageFromJob(t *testing.T) {
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{
		{Name: v1.ParameterWorkload, Value: "wl1"},
		{Name: v1.ParameterImage, Value: "img:1"},
	}}}
	assert.Equal(t, "wl1", getWorkloadIdFromJob(job))
	assert.Equal(t, "img:1", getSourceImageFromJob(job))
	assert.Equal(t, "", getWorkloadIdFromJob(&v1.OpsJob{}))
	assert.Equal(t, "", getSourceImageFromJob(&v1.OpsJob{}))
}

func TestExportImageObserveFilter(t *testing.T) {
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobExportImageType}}
	quit, err := r.observe(context.Background(), job)
	assert.NoError(t, err)
	assert.False(t, quit)
	assert.False(t, r.filter(context.Background(), job))
	assert.True(t, r.filter(context.Background(), &v1.OpsJob{Spec: v1.OpsJobSpec{Type: v1.OpsJobRebootType}}))
}

func TestExportImageHandlePending(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	_, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobRunning, job.Status.Phase)
}

func TestExportImageHandleNonPending(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning, StartedAt: &metav1.Time{Time: time.Now()}},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	res, err := r.handle(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), res.RequeueAfter.Nanoseconds())
}

func TestExportImageDoNoWorkloadId(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
}

func TestExportImageDoJobNotFound(t *testing.T) {
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	_, err := r.Do(context.Background(), "missing")
	assert.Error(t, err)
}

func TestExportImageDoMissingWorkloadId(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
		Status:     v1.OpsJobStatus{Phase: v1.OpsJobRunning},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
}

func TestExportImageDoWorkloadNotFound(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1"},
		Spec: v1.OpsJobSpec{
			Type: v1.OpsJobExportImageType,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterWorkload, Value: "wl1"},
				{Name: v1.ParameterImage, Value: "img:1"},
			},
		},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	_, err := r.Do(context.Background(), "j1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "j1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
}

func TestExportImageDoWorkloadBranches(t *testing.T) {
	ctx := context.Background()

	// workload missing -> failed
	t.Run("workload missing", func(t *testing.T) {
		job := exportJob("e1", "wl-missing", "img:1")
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
		_, err := r.Do(ctx, "e1")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})

	// workload with no pods -> failed
	t.Run("workload no pods", func(t *testing.T) {
		job := exportJob("e2", "wl2", "img:1")
		wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl2"}}
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
		_, err := r.Do(ctx, "e2")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e2"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})

	// workload pod without a name -> failed
	t.Run("pod without a name", func(t *testing.T) {
		job := exportJob("e3", "wl3", "img:1")
		wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl3"}}
		wl.Status.Pods = []v1.WorkloadPod{{AdminNodeName: "n1"}}
		r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl)}
		_, err := r.Do(ctx, "e3")
		assert.NoError(t, err)
		updated := &v1.OpsJob{}
		assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e3"}, updated))
		assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	})
}

func TestGenerateTargetImageName(t *testing.T) {
	now := time.Date(2026, 10, 8, 18, 4, 5, 0, time.UTC)
	for src, want := range map[string]string{
		"rocm/7.0-preview:tag":        "custom/rocm/7.0-preview:20261008180405-a1b2c3",
		"nginx":                       "custom/library/nginx:20261008180405-a1b2c3",
		"docker.io/library/nginx:1.0": "custom/library/nginx:20261008180405-a1b2c3",
		"reg.example.com:5000/proxy/Library/Python:3.12-slim":                    "custom/library/python:20261008180405-a1b2c3",
		"reg.example.com/proxy/library/python@sha256:" + strings.Repeat("a", 64): "custom/library/python:20261008180405-a1b2c3",
	} {
		got, err := generateTargetImageName("custom", src, now, "a1b2c3")
		assert.NoError(t, err, src)
		assert.Equal(t, want, got, src)
	}
	_, err := generateTargetImageName("custom", "Not A Reference", now, "a1b2c3")
	assert.Error(t, err)

	// Two exports of one image in the same second never share a tag.
	a, _ := generateTargetImageName("custom", "nginx", now, randomSuffix())
	b, _ := generateTargetImageName("custom", "nginx", now, randomSuffix())
	assert.NotEqual(t, a, b)
	assert.Regexp(t, `^[0-9a-f]{6}$`, randomSuffix())
}

func TestPickExportPod(t *testing.T) {
	assert.Equal(t, "", pickExportPod(nil))
	assert.Equal(t, "p1", pickExportPod([]v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodPending}}))
	assert.Equal(t, "p2", pickExportPod([]v1.WorkloadPod{
		{PodId: "p1", Phase: corev1.PodFailed}, {PodId: "p2", Phase: corev1.PodRunning}}))
}

func runningStatus(name string) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:    name,
		ImageID: "reg/x@sha256:" + strings.Repeat("a", 64),
		State:   corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Unix(100, 0)}},
	}
}

func TestExportContainer(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "sidecar"}, {Name: "main"}}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			runningStatus("sidecar"), runningStatus("main")}},
	}
	name, st, err := exportContainer(pod, "main")
	assert.NoError(t, err)
	assert.Equal(t, "main", name)
	assert.Equal(t, "main", st.Name)

	name, _, err = exportContainer(pod, "")
	assert.NoError(t, err)
	assert.Equal(t, "sidecar", name, "without a main container annotation, the first container")

	_, _, err = exportContainer(pod, "missing")
	assert.Error(t, err)

	pod.Status.ContainerStatuses[1].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}
	_, _, err = exportContainer(pod, "main")
	assert.Error(t, err, "a container that is not running cannot be read")
}

// exportFixture is a workload with one running pod, the registry settings, and an export
// function that records its request.
func exportFixture(t *testing.T, exportErr error) (*ExportImageJobReconciler, *exportimage.Request, func()) {
	t.Helper()
	job := exportJob("e1", "wl1", "harbor.local/proxy/library/python:3.12")
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name: "wl1",
		Labels: map[string]string{
			v1.WorkspaceIdLabel: "ws1", v1.ClusterIdLabel: "c1",
		},
		Annotations: map[string]string{v1.MainContainerAnnotation: "main"},
	}}
	wl.Status.Pods = []v1.WorkloadPod{{PodId: "p1", Phase: corev1.PodRunning}}
	authStr := base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	cred := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: common.ImageImportSecretName, Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{"config.json": []byte(`{"auths":{"harbor.local":{"auth":"` + authStr + `"}}}`)},
	}
	staging := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: common.SaveImageStagingSecretName, Namespace: common.PrimusSafeNamespace},
		Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{"harbor.local":{"auth":"` +
			base64.StdEncoding.EncodeToString([]byte("robot:staging")) + `"}}}`)},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ws1"},
		Spec:       corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{Name: "main"}}},
		Status:     corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{runningStatus("main")}},
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Status.NodeInfo.OperatingSystem, node.Status.NodeInfo.Architecture = "linux", "amd64"

	ctrl := gomock.NewController(t)
	db := mockclient.NewMockInterface(ctrl)
	db.EXPECT().ListWorkloadPods(gomock.Any(), "wl1", gomock.Any()).Return(nil, nil).AnyTimes()
	db.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(&model.RegistryInfo{URL: "harbor.local"}, nil).AnyTimes()

	cs := k8sfake.NewSimpleClientset(pod, node)
	patches := gomonkey.ApplyFunc(rmutils.GetK8sClientFactory,
		func(_ *commonutils.ObjectManager, _ string) (*commonclient.ClientFactory, error) {
			return commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs), nil
		})

	got := &exportimage.Request{}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job, wl, cred, staging), dbClient: db}
	r.export = func(_ context.Context, req exportimage.Request) (*exportimage.Result, error) {
		*got = req
		if exportErr != nil {
			return nil, exportErr
		}
		return &exportimage.Result{Digest: "sha256:" + strings.Repeat("d", 64)}, nil
	}
	return r, got, func() { patches.Reset(); ctrl.Finish() }
}

func TestExportImageDoSucceeds(t *testing.T) {
	r, req, cleanup := exportFixture(t, nil)
	defer cleanup()
	ctx := context.Background()
	_, err := r.Do(ctx, "e1")
	assert.NoError(t, err)

	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
	assert.Equal(t, v1.OpsJobSucceeded, updated.Status.Phase)
	outputs := map[string]string{}
	for _, p := range updated.Status.Outputs {
		outputs[p.Name] = p.Value
	}
	assert.Equal(t, "sha256:"+strings.Repeat("d", 64), outputs["digest"])
	assert.Regexp(t, `^harbor\.local/custom/library/python:[0-9]{14}-[0-9a-f]{6}$`, outputs["target"])
	assert.Equal(t, outputs["target"], req.Target.String())

	// The export reads the main container of the running pod, from the digest it runs.
	pe, ok := req.Exec.(*exportimage.PodExecer)
	assert.True(t, ok)
	assert.Equal(t, "ws1", pe.Namespace)
	assert.Equal(t, "p1", pe.Pod)
	assert.Equal(t, "main", pe.Container)
	assert.Equal(t, "reg/x@sha256:"+strings.Repeat("a", 64), req.ImageID)
	// Without settings for the cluster, the image is published in the default registry,
	// and each export gets a staging repository of its own there.
	assert.Equal(t, "harbor.local/save-staging/e1", req.Staging.String())

	// The container's token is minted with the staging-only account, everything else
	// with the platform's.
	auth := func(kc interface {
		Resolve(authn.Resource) (authn.Authenticator, error)
	}) string {
		reg, err := gcrname.NewRegistry("harbor.local")
		assert.NoError(t, err)
		a, err := kc.Resolve(reg)
		assert.NoError(t, err)
		cfg, err := a.Authorization()
		assert.NoError(t, err)
		return cfg.Auth
	}
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("robot:staging")), auth(req.StagingKeychain))
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("admin:secret")), auth(req.Keychain))
}

// Without a staging-only credential the export is refused: the platform's own credential
// would give the user's container the platform's power over the registry.
func TestExportImageRefusesWithoutAStagingCredential(t *testing.T) {
	r, _, cleanup := exportFixture(t, nil)
	defer cleanup()
	ctx := context.Background()
	s := &corev1.Secret{}
	assert.NoError(t, r.Get(ctx, types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: common.SaveImageStagingSecretName}, s))
	assert.NoError(t, r.Delete(ctx, s))
	_, err := r.Do(ctx, "e1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	assert.Contains(t, updated.Status.Conditions[0].Message, "never handed to a container")
}

// A cluster's settings choose the registry (where its containers upload and the image is
// published), the projects and the registry's CA.
func TestExportImageClusterDestination(t *testing.T) {
	r, req, cleanup := exportFixture(t, nil)
	defer cleanup()
	viper.Set("save_image.clusters", []map[string]any{{
		"cluster": "c1", "registry": "edge.example.com", "target_project": "saved",
		"staging_project": "stage", "ca_secret": "harbor/edge-ca",
	}})
	defer viper.Reset()
	testCA := selfSignedPEM(t)
	ca := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-ca", Namespace: "harbor"},
		Data:       map[string][]byte{"ca.crt": []byte(testCA)},
	}
	assert.NoError(t, r.Create(context.Background(), ca))

	_, err := r.Do(context.Background(), "e1")
	assert.NoError(t, err)
	assert.Regexp(t, `^edge\.example\.com/saved/library/python:[0-9]{14}-[0-9a-f]{6}$`, req.Target.String())
	assert.Equal(t, "edge.example.com/stage/e1", req.Staging.String())
	assert.Equal(t, testCA, string(req.CA), "the container checks the registry against its CA")
}

func selfSignedPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "edge"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	assert.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestExportImageClusterDestinationRefusesAMissingCA(t *testing.T) {
	r, _, cleanup := exportFixture(t, nil)
	defer cleanup()
	viper.Set("save_image.clusters", []map[string]any{{"cluster": "c1", "ca_secret": "harbor/missing"}})
	defer viper.Reset()
	ctx := context.Background()
	_, err := r.Do(ctx, "e1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	assert.Contains(t, updated.Status.Conditions[0].Message, "harbor/missing")
}

func TestExportImageDoFailsForANonRootContainer(t *testing.T) {
	r, _, cleanup := exportFixture(t, fmt.Errorf("%w (uid 1000): saving it would leave out the files it cannot read", exportimage.ErrNotRoot))
	defer cleanup()
	ctx := context.Background()
	_, err := r.Do(ctx, "e1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	assert.Empty(t, updated.Status.Outputs, "a failed export names no image")
	assert.Contains(t, updated.Status.Conditions[0].Message, "does not run as root")
}

func TestExportImageReconcileEntry(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: "j1", Finalizers: []string{v1.OpsJobFinalizer}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobExportImageType},
	}
	r := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t, job)}
	r.Controller = commonctrl.NewController[string](nil, 1)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}})
	assert.NoError(t, err)
}

func TestExportImageQueueControllerStarts(t *testing.T) {
	ei := &ExportImageJobReconciler{OpsJobBaseReconciler: newBaseWithObjs(t)}
	ei.Controller = commonctrl.NewController[string](ei, 0)
	ei.start(context.Background())
}

// builtinHarbor adds the built-in Harbor's external endpoint and private CA.
func builtinHarbor(t *testing.T, r *ExportImageJobReconciler, endpoint string) string {
	t.Helper()
	ca := selfSignedPEM(t)
	ctx := context.Background()
	assert.NoError(t, r.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: harborTLSSecretName, Namespace: harborNamespace},
		Data:       map[string][]byte{"ca.crt": []byte(ca)},
	}))
	assert.NoError(t, r.Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: harborCoreConfigMap, Namespace: harborNamespace},
		Data:       map[string]string{harborEndpointKey: endpoint},
	}))
	return ca
}

// The built-in Harbor's private CA goes to the container only when it uploads to the
// built-in Harbor; another registry is checked against the container's own roots.
func TestExportImageSendsTheBuiltinCAOnlyForTheBuiltinHarbor(t *testing.T) {
	t.Run("built-in Harbor", func(t *testing.T) {
		r, req, cleanup := exportFixture(t, nil)
		defer cleanup()
		ca := builtinHarbor(t, r, "https://harbor.local")
		_, err := r.Do(context.Background(), "e1")
		assert.NoError(t, err)
		assert.Equal(t, ca, string(req.CA))
		assert.Equal(t, "harbor.local", req.Registry)
	})
	t.Run("another registry, no CA configured", func(t *testing.T) {
		r, req, cleanup := exportFixture(t, nil)
		defer cleanup()
		builtinHarbor(t, r, "https://harbor.local")
		viper.Set("save_image.clusters", []map[string]any{{"cluster": "c1", "registry": "edge.example.com"}})
		defer viper.Reset()
		_, err := r.Do(context.Background(), "e1")
		assert.NoError(t, err)
		assert.Equal(t, "edge.example.com", req.Registry)
		assert.Empty(t, req.CA, "a publicly trusted registry needs no CA, and the built-in one does not sign it")
	})
}

// A registry named without a dot or a port reads as Docker Hub to the registry client;
// the export is refused before any credential is looked up.
func TestExportImageRefusesARegistryNameReadAsDockerHub(t *testing.T) {
	r, req, cleanup := exportFixture(t, nil)
	defer cleanup()
	viper.Set("save_image.clusters", []map[string]any{{"cluster": "c1", "registry": "myregistry"}})
	defer viper.Reset()
	ctx := context.Background()
	_, err := r.Do(ctx, "e1")
	assert.NoError(t, err)
	assert.Nil(t, req.Exec, "the export never started")
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	assert.Contains(t, updated.Status.Conditions[0].Message, "myregistry")
}

// ctxClient fails a call made with a context that is done, as a real API client does.
type ctxClient struct{ client.Client }

func (c ctxClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c ctxClient) Status() client.SubResourceWriter { return ctxStatus{c.Client.Status()} }

type ctxStatus struct{ client.SubResourceWriter }

func (s ctxStatus) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.SubResourceWriter.Update(ctx, obj, opts...)
}

// An export that runs out of time records why it failed: the job's context is over by
// then, and the outcome is written with one of its own.
func TestExportImageRecordsWhyItRanOutOfTime(t *testing.T) {
	r, _, cleanup := exportFixture(t, nil)
	defer cleanup()
	ctx := context.Background()
	job := &v1.OpsJob{}
	assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, job))
	job.CreationTimestamp = metav1.Now()
	job.Spec.TimeoutSecond = 1
	assert.NoError(t, r.Update(ctx, job))
	r.Client = ctxClient{r.Client}
	r.export = func(ctx context.Context, _ exportimage.Request) (*exportimage.Result, error) {
		<-ctx.Done()
		return nil, fmt.Errorf("uploading the layer at byte 4096: %w", ctx.Err())
	}
	_, err := r.Do(ctx, "e1")
	assert.NoError(t, err)
	updated := &v1.OpsJob{}
	assert.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
	assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
	assert.Contains(t, updated.Status.Conditions[0].Message, "uploading the layer at byte 4096")
}

// The base image is read for the platform of the pod's node, which is what the runtime
// pulled from a multi-platform image.
func TestExportImageReadsTheBaseForTheNodesPlatform(t *testing.T) {
	for _, tc := range []struct {
		name   string
		node   func(*corev1.Node)
		want   gcrv1.Platform
		errMsg string
	}{
		{name: "amd64", node: func(*corev1.Node) {}, want: gcrv1.Platform{OS: "linux", Architecture: "amd64"}},
		{name: "arm64", node: func(n *corev1.Node) { n.Status.NodeInfo.Architecture = "arm64" },
			want: gcrv1.Platform{OS: "linux", Architecture: "arm64"}},
		{name: "from the labels", node: func(n *corev1.Node) {
			n.Status.NodeInfo = corev1.NodeSystemInfo{}
			n.Labels = map[string]string{corev1.LabelOSStable: "linux", corev1.LabelArchStable: "arm64"}
		}, want: gcrv1.Platform{OS: "linux", Architecture: "arm64"}},
		{name: "unknown", node: func(n *corev1.Node) { n.Status.NodeInfo = corev1.NodeSystemInfo{} },
			errMsg: "reports no operating system or architecture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, req, cleanup := exportFixture(t, nil)
			defer cleanup()
			ctx := context.Background()
			cs, err := rmutils.GetK8sClientFactory(nil, "c1")
			require.NoError(t, err)
			n, err := cs.ClientSet().CoreV1().Nodes().Get(ctx, "n1", metav1.GetOptions{})
			require.NoError(t, err)
			tc.node(n)
			_, err = cs.ClientSet().CoreV1().Nodes().Update(ctx, n, metav1.UpdateOptions{})
			require.NoError(t, err)

			_, err = r.Do(ctx, "e1")
			require.NoError(t, err)
			updated := &v1.OpsJob{}
			require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "e1"}, updated))
			if tc.errMsg != "" {
				assert.Equal(t, v1.OpsJobFailed, updated.Status.Phase)
				assert.Contains(t, updated.Status.Conditions[0].Message, tc.errMsg)
				return
			}
			assert.Equal(t, v1.OpsJobSucceeded, updated.Status.Phase)
			assert.Equal(t, tc.want, req.Platform)
		})
	}
}
