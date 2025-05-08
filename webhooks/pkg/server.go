/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/log"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientscheme.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

type Server struct {
	opts        *Options
	ctrlManager manager.Manager
	isInited    bool
}

func NewServer() (*Server, error) {
	s := &Server{
		opts: &Options{},
	}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) init() error {
	var err error
	if err = s.opts.InitFlags(); err != nil {
		return fmt.Errorf("failed to parse flags. %s", err.Error())
	}
	if err = s.initLogs(); err != nil {
		return fmt.Errorf("failed to init logs. %s", err.Error())
	}
	if err = s.initConfig(); err != nil {
		return fmt.Errorf("failed to init config. %s", err.Error())
	}
	if err = s.newCtrlManager(); err != nil {
		return fmt.Errorf("failed to new controller manager. %s", err.Error())
	}
	setUpWebhooks(s.ctrlManager, s.ctrlManager.GetWebhookServer())
	s.isInited = true
	return nil
}

func (s *Server) Start() {
	if !s.isInited {
		klog.Errorf("Please init webhooks server first")
		return
	}
	ctx := ctrlruntime.SetupSignalHandler()
	klog.Infof("start webhooks server")

	go func() {
		if err := s.ctrlManager.Start(ctx); err != nil {
			klog.ErrorS(err, "failed to start webhooks server")
			os.Exit(-1)
		}
	}()

	if !s.ctrlManager.GetCache().WaitForCacheSync(ctx) {
		klog.Errorf("failed to WaitForCacheSync")
		os.Exit(-1)
	}
	<-ctx.Done()
	s.shutdown()
}

func (s *Server) shutdown() {
	klog.Infof("webhooks server stopped")
	klog.Flush()
}

func (s *Server) initLogs() error {
	if err := log.Init(s.opts.LogfilePath, s.opts.LogFileSize); err != nil {
		return err
	}
	ctrlruntime.SetLogger(klogr.NewWithOptions())
	return nil
}

func (s *Server) initConfig() error {
	fullPath, err := filepath.Abs(s.opts.Config)
	if err != nil {
		return err
	}
	if err = commonconfig.LoadConfig(fullPath); err != nil {
		return fmt.Errorf("config path: %s, err: %v", fullPath, err)
	}
	return nil
}

func (s *Server) newCtrlManager() error {
	cfg, err := commonclient.GetRestConfigInCluster()
	if err != nil {
		return err
	}
	localIp, err := netutil.GetLocalIp()
	if err != nil {
		return err
	}
	if commonconfig.GetServerPort() <= 0 {
		return fmt.Errorf("the server port is not defined")
	}
	opts := manager.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		LeaderElection:             commonconfig.IsLeaderElectionEnable(),
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaderElectionNamespace:    commonconfig.GetLeaderElectionLock(),
		LeaderElectionID:           "primus-safe-webhooks",
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:     localIp,
			Port:     commonconfig.GetServerPort(),
			CertDir:  s.opts.CertDir,
			KeyName:  os.Getenv("KEY_NAME"),
			CertName: os.Getenv("CERT_NAME"),
		}),
	}
	s.ctrlManager, err = manager.New(cfg, opts)
	if err != nil {
		return err
	}
	return nil
}

func setUpWebhooks(mgr manager.Manager, server webhook.Server) {
	decoder := admission.NewDecoder(mgr.GetScheme())
	AddNodeWebhook(mgr, &server, decoder)
	AddNodeFlavorWebhook(mgr, &server, decoder)
}
