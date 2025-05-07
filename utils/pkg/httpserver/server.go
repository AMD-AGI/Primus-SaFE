/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package httpserver

import (
	"context"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/errors"
)

type Mux interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// EnableMuxProfile register pprofile interface
func EnableMuxProfile(m Mux) {
	m.HandleFunc("/debug/pprof/", pprof.Index)
	m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.HandleFunc("/debug/pprof/trace", pprof.Symbol)
}

type HTTPServer interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
}

func Run(stopCh <-chan struct{}, gracefullyStopTimeout time.Duration, servers ...HTTPServer) error {
	for _, srv := range servers {
		if srv == nil {
			continue
		}
		go func(srv HTTPServer) {
			// service connections
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logrus.WithError(err).Warn("server is not successfully closed")
			}
		}(srv)
	}
	<-stopCh
	logrus.Infoln("Shutdown Server ...")

	var wg sync.WaitGroup
	errCh := make(chan error, len(servers))
	for _, srv := range servers {
		if srv == nil {
			continue
		}
		wg.Add(1)
		go func(srv HTTPServer) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), gracefullyStopTimeout)
			defer cancel()
			errCh <- srv.Shutdown(ctx)
		}(srv)
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for e := range errCh {
		if e != nil {
			errs = append(errs, e)
		}
	}
	return errors.NewAggregate(errs)
}
