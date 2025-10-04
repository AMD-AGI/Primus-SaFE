/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"fmt"
	"path/filepath"

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
	opts       *options.Options
	jobManager *JobManager
	isInited   bool
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
		klog.ErrorS(err, "failed to initialize flags")
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
	if s.jobManager, err = NewJobManager(scheme); err != nil {
		klog.ErrorS(err, "failed to initialize job manager")
		return err
	}
	s.isInited = true
	return nil
}

func (s *Server) Start() {
	if !s.isInited {
		klog.Error("pls init job manager first!")
		return
	}
	klog.Infof("starting job manager")
	if err := s.jobManager.Start(); err != nil {
		klog.ErrorS(err, "failed to start job manager")
		return
	}
	s.jobManager.Wait()
	s.Stop()
}

func (s *Server) Stop() {
	klog.Info("job manager stopped")
	klog.Flush()
}

func (s *Server) initLogs() error {
	if err := commonklog.Init(s.opts.LogfilePath, s.opts.LogFileSize); err != nil {
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
