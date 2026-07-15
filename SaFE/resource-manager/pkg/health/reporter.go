/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package health implements the resource-manager self-health reporter: it
// periodically collects SaFE control-plane health (component readiness, database
// reachability, managed-cluster reachability) and pushes it to the data-plane
// Robust VictoriaMetrics via the shared common/pkg/health registry.
package health

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonhealth "github.com/AMD-AIG-AIMA/SAFE/common/pkg/health"
	k8sclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

const (
	componentName = "resource-manager"

	// clusterProbeTimeout bounds a single data-plane API reachability probe.
	clusterProbeTimeout = 5 * time.Second
	// dbProbeTimeout bounds a single database ping.
	dbProbeTimeout = 5 * time.Second
	// cycleTimeout bounds the k8s list calls within one collection cycle.
	cycleTimeout = 20 * time.Second
)

// Reporter collects and pushes SaFE self-health metrics. It is a
// controller-runtime Runnable and runs on the leader only, so the shared VM
// receives a single copy of each series.
type Reporter struct {
	cli        client.Client
	httpClient *http.Client
}

// NewReporter creates a self-health reporter bound to the manager's client.
func NewReporter(cli client.Client) *Reporter {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	// Cross-cluster pushes may target a VM ingress with an internal-CA cert the
	// pusher does not trust; allow opting out of TLS verification per config.
	if commonconfig.IsMetricsRemoteWriteInsecureSkipVerify() {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return &Reporter{
		cli:        cli,
		httpClient: httpClient,
	}
}

// NeedLeaderElection makes the reporter run only on the elected leader.
func (r *Reporter) NeedLeaderElection() bool { return true }

// Start runs the collect+push loop until the context is cancelled.
func (r *Reporter) Start(ctx context.Context) error {
	if !commonconfig.IsMetricsRemoteWriteEnabled() {
		klog.Info("[self-health] metrics.remote_write disabled, reporter not started")
		return nil
	}
	if commonconfig.GetMetricsRemoteWriteURL() == "" {
		klog.Warning("[self-health] metrics.remote_write enabled but url is empty, reporter not started")
		return nil
	}

	interval := time.Duration(commonconfig.GetMetricsRemoteWriteIntervalSeconds()) * time.Second
	klog.Infof("[self-health] reporter started: url=%s interval=%s", commonconfig.GetMetricsRemoteWriteURL(), interval)
	commonhealth.BuildInfo.WithLabelValues(componentName).Set(1)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.collectAndPush(ctx)
	for {
		select {
		case <-ctx.Done():
			klog.Info("[self-health] reporter stopped")
			return nil
		case <-ticker.C:
			r.collectAndPush(ctx)
		}
	}
}

func (r *Reporter) collectAndPush(ctx context.Context) {
	commonhealth.ResetScanned()

	cctx, cancel := context.WithTimeout(ctx, cycleTimeout)
	defer cancel()

	r.collectComponents(cctx)
	r.collectDatabase(cctx)
	r.collectClusters(cctx)

	cfg := commonhealth.PushConfig{
		URL:   commonconfig.GetMetricsRemoteWriteURL(),
		Job:   commonconfig.GetMetricsRemoteWriteJob(),
		Token: commonconfig.GetMetricsRemoteWriteToken(),
	}
	if clusterName := commonconfig.GetMetricsRemoteWriteClusterName(); clusterName != "" {
		cfg.Extra = map[string]string{"cluster": clusterName}
	}
	if err := commonhealth.Push(ctx, r.httpClient, cfg); err != nil {
		klog.Warningf("[self-health] push failed: %v", err)
	}
}

// collectComponents reports readiness of every Deployment and DaemonSet in the
// primus-safe namespace (the SaFE control-plane workloads).
func (r *Reporter) collectComponents(ctx context.Context) {
	var deployList appsv1.DeploymentList
	if err := r.cli.List(ctx, &deployList, client.InNamespace(common.PrimusSafeNamespace)); err != nil {
		klog.Warningf("[self-health] list deployments: %v", err)
	} else {
		for i := range deployList.Items {
			d := &deployList.Items[i]
			desired := int32(1)
			if d.Spec.Replicas != nil {
				desired = *d.Spec.Replicas
			}
			ready := d.Status.ReadyReplicas
			r.setComponent(d.Name, "Deployment", float64(desired), float64(ready), desired > 0 && ready >= desired)
		}
	}

	var dsList appsv1.DaemonSetList
	if err := r.cli.List(ctx, &dsList, client.InNamespace(common.PrimusSafeNamespace)); err != nil {
		klog.Warningf("[self-health] list daemonsets: %v", err)
	} else {
		for i := range dsList.Items {
			ds := &dsList.Items[i]
			desired := ds.Status.DesiredNumberScheduled
			ready := ds.Status.NumberReady
			r.setComponent(ds.Name, "DaemonSet", float64(desired), float64(ready), desired > 0 && ready >= desired)
		}
	}
}

func (r *Reporter) setComponent(name, kind string, desired, ready float64, up bool) {
	commonhealth.ComponentReplicasDesired.WithLabelValues(name, kind).Set(desired)
	commonhealth.ComponentReplicasReady.WithLabelValues(name, kind).Set(ready)
	commonhealth.SetBool(commonhealth.ComponentUp.WithLabelValues(name, kind), up)
}

// collectDatabase pings the database and reports the "database" subsystem gauge.
func (r *Reporter) collectDatabase(ctx context.Context) {
	if !commonconfig.IsDBEnable() {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, dbProbeTimeout)
	defer cancel()

	ok := false
	if db := dbclient.NewClient(); db != nil {
		if err := db.Ping(dbCtx); err != nil {
			klog.Warningf("[self-health] db ping failed: %v", err)
		} else {
			ok = true
		}
	}
	commonhealth.SetBool(commonhealth.SubsystemUp.WithLabelValues(commonhealth.SubsystemDatabase), ok)
}

// collectClusters reports, per managed Cluster CR, whether it is in Ready phase
// and whether its API server is actually reachable using SaFE's stored creds.
func (r *Reporter) collectClusters(ctx context.Context) {
	var clusterList v1.ClusterList
	if err := r.cli.List(ctx, &clusterList); err != nil {
		klog.Warningf("[self-health] list clusters: %v", err)
		return
	}
	for i := range clusterList.Items {
		c := &clusterList.Items[i]
		if !c.GetDeletionTimestamp().IsZero() {
			continue
		}
		commonhealth.SetBool(commonhealth.ClusterReady.WithLabelValues(c.Name), c.IsReady())
		commonhealth.SetBool(commonhealth.ClusterUp.WithLabelValues(c.Name), r.probeCluster(ctx, c))
	}
}

// probeCluster attempts a bounded ServerVersion call against a data-plane
// cluster using the credentials stored on the Cluster CR status.
func (r *Reporter) probeCluster(ctx context.Context, c *v1.Cluster) bool {
	cps := c.Status.ControlPlaneStatus
	if len(cps.Endpoints) == 0 || cps.CertData == "" || cps.KeyData == "" {
		return false
	}
	clientSet, _, err := k8sclient.NewClientSet(cps.Endpoints[0], cps.CertData, cps.KeyData, cps.CAData, true)
	if err != nil {
		klog.V(4).Infof("[self-health] cluster %s client build failed: %v", c.Name, err)
		return false
	}

	probeCtx, cancel := context.WithTimeout(ctx, clusterProbeTimeout)
	defer cancel()
	done := make(chan bool, 1)
	go func() {
		_, verr := clientSet.Discovery().ServerVersion()
		done <- verr == nil
	}()
	select {
	case <-probeCtx.Done():
		return false
	case ok := <-done:
		return ok
	}
}
