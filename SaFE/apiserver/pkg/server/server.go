/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package apiserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/ptr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	safeconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/controllers"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonklog "github.com/AMD-AIG-AIMA/SAFE/common/pkg/klog"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/options"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/trace"
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
	httpServer  *http.Server
	sshServer   *SshServer
	ctrlManager ctrlruntime.Manager
	ctx         context.Context
	isInited    bool
}

// NewServer creates and returns a new Server instance.
func NewServer() (*Server, error) {
	s := &Server{
		opts: &options.Options{},
		ctx:  ctrlruntime.SetupSignalHandler(),
	}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// init performs the initial setup of the server including flag parsing,
// logging initialization, configuration loading, and controller manager setup.
// It also sets up the controllers and marks the server as initialized.
func (s *Server) init() error {
	gin.SetMode(gin.ReleaseMode)
	var err error
	if err = s.opts.InitFlags(); err != nil {
		klog.ErrorS(err, "failed to parse flags")
		return err
	}
	if err = s.initLogs(); err != nil {
		klog.ErrorS(err, "failed to init logs")
		return err
	}
	if err = s.initConfig(); err != nil {
		klog.ErrorS(err, "failed to init config")
		return err
	}
	if s.ctrlManager, err = newCtrlManager(); err != nil {
		klog.ErrorS(err, "failed to init controller manager")
		return err
	}
	if err = controllers.SetupControllers(s.ctx, s.ctrlManager); err != nil {
		klog.ErrorS(err, "failed to setup controllers")
		return err
	}
	if err = opensearch.StartDiscover(s.ctx); err != nil {
		klog.ErrorS(err, "failed to start opensearch discovery")
		return err
	}
	if safeconfig.IsTracingEnable() {
		if err = trace.InitTracer("primus-safe-apiserver"); err != nil {
			klog.Warningf("Failed to init tracer: %v", err)
		}
	} else {
		klog.Info("Tracing is disabled (tracing.enable: false)")
	}
	s.isInited = true
	return nil
}

// Start begins the server operation by starting the controller manager,
// HTTP server, and SSH server (if enabled) in separate goroutines.
// It waits for a signal to stop and then calls Stop to shut down services.
func (s *Server) Start() {
	if !s.isInited {
		klog.Errorf("please init api-server first")
		return
	}
	gin.EnableJsonDecoderDisallowUnknownFields()

	klog.Infof("starting api-server")
	go func() {
		if err := s.ctrlManager.Start(s.ctx); err != nil {
			klog.ErrorS(err, "failed to start controller manager")
			os.Exit(-1)
		}
	}()
	if !s.ctrlManager.GetCache().WaitForCacheSync(s.ctx) {
		klog.Errorf("failed to WaitForCacheSync for controller manager")
		os.Exit(-1)
	}

	go func() {
		if err := s.startHttpServer(); err != nil {
			klog.ErrorS(err, "failed to start http-server")
			os.Exit(-1)
		}
	}()

	go func() {
		if err := s.startSSHServer(); err != nil {
			klog.ErrorS(err, "failed to start ssh-server")
			os.Exit(-1)
		}
	}()

	<-s.ctx.Done()
	s.Stop()
}

// Stop gracefully shuts down the HTTP server and SSH server (if running).
// It cancels the context, shuts down services, and flushes logs before exiting.
func (s *Server) Stop() {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	klog.Info("shutting down http server...")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		klog.ErrorS(err, "failed to shutdown httpserver")
	}
	if s.sshServer != nil {
		if err := s.sshServer.Shutdown(); err != nil {
			klog.ErrorS(err, "failed to shutdown ssh-server")
		}
	}
	// Close tracer
	if err := trace.CloseTracer(); err != nil {
		klog.ErrorS(err, "failed to close tracer")
	}
	klog.Info("apiserver is stopped")
	klog.Flush()
}

// initLogs initializes the logging system with the specified log file path and size.
// It also sets up the controller-runtime logger to use klog.
func (s *Server) initLogs() error {
	if err := commonklog.Init(s.opts.LogfilePath, s.opts.LogFileSize); err != nil {
		return err
	}
	ctrlruntime.SetLogger(klogr.NewWithOptions())
	return nil
}

// initConfig loads the server configuration from the specified config file path.
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

// startHttpServer initializes and starts the HTTP server.
// It sets up the HTTP handlers, configures the server address based on the configured port,
// and starts listening for HTTP requests.
func (s *Server) startHttpServer() error {
	if commonconfig.GetServerPort() <= 0 {
		return fmt.Errorf("the apiserver port is not defined")
	}
	handler, err := handlers.InitHttpHandlers(s.ctx, s.ctrlManager)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf(":%d", commonconfig.GetServerPort())
	s.httpServer = &http.Server{Addr: addr, Handler: handler}
	klog.Infof("http-server listen port: %d", commonconfig.GetServerPort())
	if err = s.httpServer.ListenAndServe(); err != nil {
		klog.ErrorS(err, "failed to start http server")
		return err
	}
	return nil
}

// startSSHServer initializes and starts the SSH server if SSH functionality is enabled.
// It sets up the SSH handlers, configures the server address based on the configured port,
// and starts listening for SSH connections.
func (s *Server) startSSHServer() error {
	if !commonconfig.IsSSHEnable() {
		return nil
	}
	if commonconfig.GetSSHServerPort() <= 0 {
		return fmt.Errorf("the ssh port is not defined")
	}
	handler, err := handlers.InitSshHandlers(s.ctx, s.ctrlManager)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf(":%d", commonconfig.GetSSHServerPort())
	s.sshServer = NewSshServer(addr, handler)
	klog.Infof("ssh-server listen port: %d", commonconfig.GetSSHServerPort())
	if err = s.sshServer.Start(s.ctx); err != nil {
		klog.ErrorS(err, "failed to start ssh server")
		return err
	}
	return nil
}

// newCtrlManager creates and configures a new controller manager.
// It sets up the manager options including scheme, leader election, health probes,
// and then creates and returns the manager instance.
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
		Controller: config.Controller{
			SkipNameValidation: ptr.To(true),
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
