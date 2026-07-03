/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/options"
)

// TestServerStartNotInited verifies Start is a no-op when the server is not initialized.
func TestServerStartNotInited(t *testing.T) {
	s := &Server{isInited: false}
	// Should log and return without panicking (jobManager is nil).
	s.Start()
}

// TestServerStop verifies Stop flushes logs without error.
func TestServerStop(t *testing.T) {
	s := &Server{}
	s.Stop()
}

// TestNewServer patches the init dependencies so NewServer + init run end-to-end
// without real flags/config/manager.
func TestNewServer(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethod(reflect.TypeOf(&options.Options{}), "InitFlags",
		func(*options.Options) error { return nil })
	patches.ApplyPrivateMethod(reflect.TypeOf(&Server{}), "initLogs",
		func(*Server) error { return nil })
	patches.ApplyPrivateMethod(reflect.TypeOf(&Server{}), "initConfig",
		func(*Server) error { return nil })
	patches.ApplyFunc(NewJobManager, func(*runtime.Scheme) (*JobManager, error) {
		return &JobManager{}, nil
	})

	s, err := NewServer()
	assert.NilError(t, err)
	assert.Assert(t, s != nil)
	assert.Equal(t, s.isInited, true)
}

// TestServerStartInited patches the job manager lifecycle so Start runs its full path.
func TestServerStartInited(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethod(reflect.TypeOf(&JobManager{}), "Start",
		func(*JobManager) error { return nil })
	patches.ApplyMethod(reflect.TypeOf(&JobManager{}), "Wait",
		func(*JobManager) {})

	s := &Server{isInited: true, jobManager: &JobManager{}}
	s.Start()
}
