/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

// +kubebuilder:scaffold:imports
import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	ginkgoconfig "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
	ginkgoconfig.GinkgoConfig.ParallelNode = 1
	ginkgoconfig.GinkgoConfig.ParallelTotal = 1

	ctx, cancel = context.WithCancel(context.Background())
	mockEnv = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "..", "apis", "pkg", "crds")},
		ErrorIfCRDPathMissing:    true,
		ControlPlaneStartTimeout: timeout,
		ControlPlaneStopTimeout:  timeout,
	}
	restConfig, err := mockEnv.Start()
	Expect(err).Should(BeNil())
	Expect(restConfig).NotTo(BeNil())
	mockScheme := clientscheme.Scheme
	Expect(v1.AddToScheme(mockScheme)).To(Succeed())
	// +kubebuilder:scaffold:scheme
	mgr, err := ctrlruntime.NewManager(restConfig, ctrlruntime.Options{
		Scheme: mockScheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",
		Controller: config.Controller{
			MaxConcurrentReconciles: 1,
			SkipNameValidation:      ptr.To(true),
		},
	})
	Expect(err).Should(BeNil())
	mockClient = mgr.GetClient()

	defer GinkgoRecover()
	setTestOptions()
	Expect(SetupControllers(ctx, mgr)).To(Succeed())
	startManager(mgr)

}, 60)

func setTestOptions() {
	defaultWorkspaceOption = WorkspaceReconcilerOption{
		processWait: 10 * time.Millisecond,
		nodeWait:    100 * time.Millisecond,
	}
	defaultFaultOption = FaultReconcilerOption{
		maxRetryCount: 10,
		processWait:   100 * time.Millisecond,
	}
}

func startManager(mgr manager.Manager) {
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
}

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
