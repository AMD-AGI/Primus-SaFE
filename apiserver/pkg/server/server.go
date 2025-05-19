/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apiserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/gin-gonic/gin"
	gerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/controllers"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/routers"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/log"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/options"
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
	opts        *options.Options
	server      *http.Server
	ctrlManager ctrlruntime.Manager
	ctx         context.Context
	isInited    bool
}

func NewServer() (*Server, error) {
	s := &Server{
		opts: &options.Options{},
	}
	if err := s.init(); err != nil {
		klog.ErrorS(err, "failed to init server")
		return nil, err
	}
	return s, nil
}

func (s *Server) init() error {
	gin.SetMode(gin.ReleaseMode)
	var err error
	if err = s.opts.InitFlags(); err != nil {
		return fmt.Errorf("failed to parse flags. %s", err.Error())
	}
	if err = s.initLogs(); err != nil {
		return fmt.Errorf("failed to init logs. %s", err.Error())
	}
	if err = s.initConfig(); err != nil {
		return fmt.Errorf("failed to init xcs config. %s", err.Error())
	}
	if s.ctrlManager, err = newCtrlManager(); err != nil {
		return fmt.Errorf("failed to new manager. %s", err.Error())
	}
	if err = controllers.SetupControllers(s.ctrlManager); err != nil {
		return fmt.Errorf("failed to setup controller. %s", err.Error())
	}
	s.isInited = true
	return nil
}

func (s *Server) Start() {
	if !s.isInited {
		klog.Errorf("please init api-server first")
		return
	}
	gin.EnableJsonDecoderDisallowUnknownFields()
	s.ctx = ctrlruntime.SetupSignalHandler()

	klog.Infof("starting api-server")
	go func() {
		if err := s.ctrlManager.Start(s.ctx); err != nil {
			klog.ErrorS(err, "failed to start controller manager")
			os.Exit(-1)
		}
	}()
	if !s.ctrlManager.GetCache().WaitForCacheSync(s.ctx) {
		klog.Errorf("failed to WaitForCacheSync")
		os.Exit(-1)
	}

	if err := s.startHttpServer(); err != nil {
		klog.ErrorS(err, "failed to start httpserver")
		os.Exit(-1)
	}

	<-s.ctx.Done()
	s.Stop()
}

func (s *Server) Stop() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		klog.Error(gerrors.Wrap(err, "api-server is stopped"))
	}
	klog.Info("api-server is stopped")
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

func (s *Server) startHttpServer() error {
	if commonconfig.GetServerPort() <= 0 {
		return fmt.Errorf("the apiserver port is not defined")
	}
	router, err := routers.InitRouters(s.ctx, s.ctrlManager)
	if err != nil {
		return err
	}
	address := fmt.Sprintf(":%d", commonconfig.GetServerPort())
	s.server = &http.Server{Addr: address, Handler: router}
	klog.Infof("api-server listen http://%s", s.server.Addr)
	if err = s.server.ListenAndServe(); err != nil {
		klog.ErrorS(err, "failed to ListenAndServe")
		return err
	}
	return nil
}

func newCtrlManager() (ctrlruntime.Manager, error) {
	healthProbeAddress := ""
	if commonconfig.IsHealthCheckEnabled() {
		localIp, err := netutil.GetLocalIp()
		if err != nil {
			return nil, err
		}
		if commonconfig.GetHealthCheckPort() <= 0 {
			return nil, fmt.Errorf("the healthcheck port is not defined")
		}
		healthProbeAddress = fmt.Sprintf("%s:%d", localIp, commonconfig.GetHealthCheckPort())
	}

	opts := manager.Options{
		Scheme:                 scheme,
		LeaderElection:         false,
		HealthProbeBindAddress: healthProbeAddress,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	}
	cfg, err := commonclient.GetRestConfigInCluster()
	if err != nil {
		return nil, err
	}
	mgr, err := manager.New(cfg, opts)
	if err != nil {
		return nil, err
	}
	if commonconfig.IsHealthCheckEnabled() {
		if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			return nil, fmt.Errorf("failed to set up health check: %v", err)
		}
		if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			return nil, fmt.Errorf("failed to set up ready check: %v", err)
		}
	}
	return mgr, nil
}
