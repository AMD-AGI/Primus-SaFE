// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package storage_scan

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"k8s.io/client-go/kubernetes"
)

var (
	regMu   sync.RWMutex
	drivers = map[string]Driver{}
)

func init() {
	Register(&JuiceFSDriver{})
}

func Register(d Driver) {
	regMu.Lock()
	defer regMu.Unlock()
	n := d.Name()
	if _, ok := drivers[n]; ok {
		panic("duplicate driver: " + n)
	}
	drivers[n] = d
}

// ScanReport contains scan results for the entire cluster.
type ScanReport struct {
	Cluster      string          `json:"cluster"`
	Timestamp    time.Time       `json:"timestamp"`
	BackendItems []BackendReport `json:"backendItems"`
	Errors       []string        `json:"errors,omitempty"`
}

type ClusterTarget struct {
	Name       string
	ClientSets *clientsets.K8SClientSet
	Extra      map[string]string
}

// Scanner is responsible for orchestrating multiple clusters and drivers.
type Scanner struct {
	Targets []ClusterTarget
}

func (s *Scanner) Run(ctx context.Context) ([]ScanReport, error) {
	if len(s.Targets) == 0 {
		return nil, errors.New("no cluster targets")
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		reports []ScanReport
		errList []error
	)
	for _, t := range s.Targets {
		wg.Add(1)
		go func(t ClusterTarget) {
			defer wg.Done()
			rep, err := s.scanOne(ctx, t)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errList = append(errList, fmt.Errorf("cluster %s: %w", t.Name, err))
				return
			}
			reports = append(reports, rep)
		}(t)
	}
	wg.Wait()
	if len(errList) > 0 {
		// Simplified: return first error while still returning completed reports
		return reports, errList[0]
	}
	return reports, nil
}

func (s *Scanner) scanOne(ctx context.Context, t ClusterTarget) (ScanReport, error) {
	kube := kubernetes.NewForConfigOrDie(t.ClientSets.Config)

	dctx := DriverContext{Cluster: t.Name, Kube: kube, Extra: t.Extra}

	// Enumerate available drivers
	regMu.RLock()
	var ds []Driver
	for _, d := range drivers {
		ds = append(ds, d)
	}
	regMu.RUnlock()

	report := ScanReport{Cluster: t.Name, Timestamp: time.Now()}
	for _, d := range ds {
		cnt, derr := d.Detect(ctx, dctx)
		if derr != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("driver %s detect: %v", d.Name(), derr))
			continue
		}
		if cnt == 0 {
			continue
		}
		backs, lerr := d.ListBackends(ctx, dctx)
		if lerr != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("driver %s list: %v", d.Name(), lerr))
			continue
		}
		for _, b := range backs {
			br, cerr := d.Collect(ctx, dctx, b)
			if cerr != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("driver %s collect %s: %v", d.Name(), b, cerr))
				continue
			}
			br.Cluster = t.Name
			report.BackendItems = append(report.BackendItems, br)
		}
	}
	return report, nil
}
