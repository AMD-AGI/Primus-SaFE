/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"fmt"
	"path/filepath"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/log"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/options"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpserver"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrlruntime "sigs.k8s.io/controller-runtime"
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
	ctrlManager *ControllerManager
	// httpServers specify HTTPServers to listen
	httpServers []httpserver.HTTPServer
	isInited    bool
}

func NewServer() (*Server, error) {
	s := &Server{
		opts: &options.Options{},
	}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) init() error {
	var err error
	if err = s.opts.InitFlags(); err != nil {
		return fmt.Errorf("failed to parse options. %s", err.Error())
	}
	if err = s.initLogs(); err != nil {
		return fmt.Errorf("failed to init logs. %s", err.Error())
	}
	if err = s.initConfig(); err != nil {
		return fmt.Errorf("failed to init config. %s", err.Error())
	}
	if s.ctrlManager, err = NewControllerManager(scheme); err != nil {
		return fmt.Errorf("failed to new controller manager. %s", err.Error())
	}
	s.isInited = true
	return nil
}

func (s *Server) Start() {
	if !s.isInited {
		klog.Errorf("Please initialize the resource manager first")
		return
	}
	klog.Infof("starting resource manager")
	stopCh := make(chan struct{})
	go func() {
		if err := httpserver.Run(stopCh, 5*time.Second, s.httpServers...); err != nil {
			klog.ErrorS(err, "http server shutdown failed")
		}
	}()

	// start until SIGTERM or SIGINT signal is caught
	if err := s.ctrlManager.Start(); err != nil {
		klog.ErrorS(err, "fail to start resource manager")
		return
	}
	s.ctrlManager.Wait()
	s.Stop()
	close(stopCh)
}

func (s *Server) Stop() {
	klog.Infof("resource manager stopped")
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
