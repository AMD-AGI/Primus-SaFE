/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

// +kubebuilder:scaffold:imports
import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	timeout = 10 * time.Second
)

var (
	mockClient client.Client
	mockEnv    *envtest.Environment
	ctx        context.Context
	cancel     context.CancelFunc
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllers Suite Test")
}

var _ = BeforeSuite(func() {
	By("setting up the test controllers environment")
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())
	mockEnv = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "..", "charts", "primus-safe", "crds")},
		ErrorIfCRDPathMissing:    true,
		ControlPlaneStartTimeout: timeout,
		ControlPlaneStopTimeout:  timeout,
		CRDInstallOptions:        envtest.CRDInstallOptions{},
	}
	cfg, err := mockEnv.Start()
	Expect(err).Should(BeNil())
	Expect(cfg).NotTo(BeNil())
	mockScheme := clientscheme.Scheme
	Expect(v1.AddToScheme(mockScheme)).To(Succeed())
	// +kubebuilder:scaffold:scheme
	mgr, err := ctrlruntime.NewManager(cfg, ctrlruntime.Options{
		Scheme: mockScheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",
	})
	Expect(err).Should(BeNil())
	mockClient = mgr.GetClient()

	err = addExporter(ctx, mgr, v1.SchemeGroupVersion.WithKind(v1.WorkloadKind), mockWorkloadHandler, mockWorkloadFilter)
	Expect(err).Should(BeNil())

	defer GinkgoRecover()
	go func() {
		if err := mgr.Start(ctx); err != nil {
			klog.ErrorS(err, "failed to start controller manager")
			os.Exit(-1)
		}
	}()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		klog.Errorf("failed to WaitForCacheSync")
		os.Exit(-1)
	}
})

var _ = AfterSuite(func() {
	By("tearing down the test controllers environment")
	cancel()
	if mockEnv != nil {
		Expect(mockEnv.Stop()).To(Succeed())
	}
})

func createObject(obj client.Object) {
	err := mockClient.Create(ctx, obj)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).Should(BeNil())
	}
	Eventually(func() bool {
		return mockClient.Get(ctx, client.ObjectKeyFromObject(obj), obj) == nil
	}, timeout, time.Millisecond*100).Should(BeTrue())
}

func deleteObject(obj client.Object) {
	err := mockClient.Delete(ctx, obj)
	if err != nil {
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}
}
