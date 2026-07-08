/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Command observability-enricher is the SaFE-native metrics enricher. It scrapes
// the AMD device-metrics-exporter, attributes each GPU to the owning SaFE
// Workload, and remote-writes robust-compatible `workload_gpu_*{workload_uid}`
// series to VictoriaMetrics so per-workload Grafana dashboards work without the
// primus-robust data plane.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/observability/enricher/pkg/enricher"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	klog.LogToStderr(true)

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		klog.Fatalf("register client-go scheme: %v", err)
	}
	if err := v1.AddToScheme(scheme); err != nil {
		klog.Fatalf("register SaFE apis scheme: %v", err)
	}

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		klog.Fatalf("load kube config: %v", err)
	}
	c, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		klog.Fatalf("create k8s client: %v", err)
	}

	cfg := enricher.LoadConfig()
	klog.Infof("[enricher] starting: interval=%s exporter=%s/%s:%d vminsert=%s cluster=%s",
		cfg.Interval, cfg.ExporterNamespace, cfg.ExporterServiceName, cfg.ExporterPort,
		cfg.VMImportURL, cfg.ClusterName)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Minimal liveness/readiness endpoint for the Deployment probes.
	go serveHealth()

	e := enricher.New(c, cfg)
	e.Run(ctx)

	klog.Info("[enricher] shutting down")
	os.Exit(0)
}

func serveHealth() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	server := &http.Server{Addr: ":8080", Handler: mux}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		klog.Warningf("[enricher] health server: %v", err)
	}
}
