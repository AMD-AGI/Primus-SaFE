/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification"
	commonsearch "github.com/AMD-AIG-AIMA/SAFE/common/pkg/opensearch"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/informer"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrlruntime "sigs.k8s.io/controller-runtime"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonklog "github.com/AMD-AIG-AIMA/SAFE/common/pkg/klog"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/options"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/exporter"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
)

// scheme is the runtime scheme used by the controller manager
var (
	scheme = runtime.NewScheme()
)

// init: initializes the runtime scheme with required API types
func init() {
	utilruntime.Must(clientscheme.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// Server: represents the main server process for the resource manager
// It coordinates controllers, exporters, and manages the overall lifecycle
type Server struct {
	opts        *options.Options
	ctrlManager *ControllerManager
	isInited    bool
}

// NewServer: creates and initializes a new Server instance
func NewServer() (*Server, error) {
	s := &Server{
		opts: &options.Options{},
	}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// init: performs server initialization including options parsing, logging, config loading, and controller setup
func (s *Server) init() error {
	var err error
	if err = s.opts.InitFlags(); err != nil {
		klog.ErrorS(err, "failed to parse options")
		return err
	}
	if err = s.initLogs(); err != nil {
		klog.ErrorS(err, "failed to initialize logs")
		return err
	}
	if err = s.initConfig(); err != nil {
		klog.ErrorS(err, "failed to initialize config")
		return err
	}
	if s.ctrlManager, err = NewControllerManager(scheme); err != nil {
		klog.ErrorS(err, "failed to initialize controller manager")
		return err
	}
	if err = resource.SetupControllers(s.ctrlManager.ctx, s.ctrlManager.ctrlManager); err != nil {
		klog.ErrorS(err, "failed to setup resource controllers")
		return err
	}
	if err = ops_job.SetupOpsJobs(s.ctrlManager.ctx, s.ctrlManager.ctrlManager); err != nil {
		klog.ErrorS(err, "failed to setup ops-job controllers")
		return err
	}
	if err = exporter.SetupExporters(s.ctrlManager.ctx, s.ctrlManager.ctrlManager); err != nil {
		klog.ErrorS(err, "failed to setup exporters")
		return err
	}
	if err = commonsearch.StartDiscover(s.ctrlManager.ctx); err != nil {
		klog.ErrorS(err, "failed to start opensearch discovery")
		return err
	}
	if err = s.ctrlManager.ctrlManager.Add(&NotificationRunner{ctrlManager: s.ctrlManager}); err != nil {
		klog.ErrorS(err, "failed to add notification runner to controller manager")
		return err
	}
	s.isInited = true
	return nil
}

// Start: begins the server operation by starting the controller manager and waiting for shutdown signal
func (s *Server) Start() {
	if !s.isInited {
		klog.Errorf("Please initialize the resource manager first")
		return
	}
	klog.Infof("starting resource manager")
	// start until SIGTERM or SIGINT signal is caught
	if err := s.ctrlManager.Start(); err != nil {
		klog.ErrorS(err, "failed to start resource manager")
		return
	}
	s.ctrlManager.Wait()
	klog.Infof("resource manager stopped")
	klog.Flush()
}

// initLogs: initializes logging configuration and sets up the controller runtime logger
func (s *Server) initLogs() error {
	if err := commonklog.Init(s.opts.LogfilePath, s.opts.LogFileSize); err != nil {
		return err
	}
	ctrlruntime.SetLogger(klogr.NewWithOptions())
	return nil
}

// initConfig: loads and validates the server configuration from the specified file path
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

type NotificationRunner struct {
	ctrlManager *ControllerManager
}

func (n *NotificationRunner) Start(ctx context.Context) error {
	klog.Infof("starting notification runner")
	var err error
	if commonconfig.IsNotificationEnable() {
		if err = notification.InitNotificationManager(n.ctrlManager.ctx, commonconfig.GetNotificationConfig()); err != nil {
			klog.ErrorS(err, "failed to initialize notification manager")
			return err
		}
		if err = informer.InitInformer(n.ctrlManager.ctx, n.ctrlManager.ctrlManager.GetConfig(), n.ctrlManager.ctrlManager.GetClient()); err != nil {
			klog.ErrorS(err, "failed to initialize informer")
			return err
		}
	}
	return err
}
