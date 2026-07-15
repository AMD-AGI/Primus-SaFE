/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package health exposes SaFE control-plane self-health as Prometheus metrics
// and pushes them into an external VictoriaMetrics/Prometheus (the data-plane
// Robust VM) through the plain-text import endpoint.
//
// The metrics live in a dedicated registry (not the global/controller-runtime
// registry) so the remote-write payload only carries low-cardinality SaFE
// component health instead of every Go/process metric.
package health

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/klog/v2"
)

const (
	// SubsystemDatabase is the subsystem label value for the DB health gauge.
	SubsystemDatabase = "database"

	importPath = "/api/v1/import/prometheus"
)

// Registry holds SaFE self-health series only.
var Registry = prometheus.NewRegistry()

var (
	// ComponentUp is 1 when a control-plane component is fully healthy
	// (desired > 0 and ready >= desired), else 0.
	ComponentUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_component_up",
		Help: "1 if the SaFE control-plane component is fully healthy (all desired replicas ready), else 0.",
	}, []string{"component", "kind"})

	// ComponentReplicasDesired reports the desired replica/scheduling count.
	ComponentReplicasDesired = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_component_replicas_desired",
		Help: "Desired replicas (Deployment) or desired scheduled pods (DaemonSet) of a SaFE component.",
	}, []string{"component", "kind"})

	// ComponentReplicasReady reports the ready replica/scheduling count.
	ComponentReplicasReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_component_replicas_ready",
		Help: "Ready replicas (Deployment) or ready pods (DaemonSet) of a SaFE component.",
	}, []string{"component", "kind"})

	// SubsystemUp is 1 when a shared subsystem (e.g. database) is reachable.
	SubsystemUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_subsystem_up",
		Help: "1 if the SaFE subsystem dependency is reachable, else 0.",
	}, []string{"subsystem"})

	// ClusterReady is 1 when a managed data-plane Cluster CR is in Ready phase.
	// The data-plane cluster is identified by target_cluster (the global
	// "cluster" label carries the reporting SaFE management-cluster name).
	ClusterReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_cluster_ready",
		Help: "1 if the managed data-plane cluster CR is in Ready phase, else 0.",
	}, []string{"target_cluster"})

	// ClusterUp is 1 when the managed data-plane cluster API is actually
	// reachable using SaFE's stored credentials. See ClusterReady for the
	// target_cluster/cluster label distinction.
	ClusterUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_cluster_up",
		Help: "1 if the managed data-plane cluster API server is reachable from SaFE, else 0.",
	}, []string{"target_cluster"})

	// BuildInfo is a constant 1 gauge carrying the reporting component name.
	BuildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_build_info",
		Help: "Constant 1 series identifying the SaFE component reporting self-health.",
	}, []string{"component"})

	// LastPushTimestamp records the unix time of the last successful push.
	LastPushTimestamp = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "safe_health_last_push_timestamp_seconds",
		Help: "Unix timestamp of the last successful self-health push to the remote VM.",
	})

	// PushTotal counts push attempts by result (success|error).
	PushTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_health_push_total",
		Help: "Total self-health push attempts to the remote VM, by result.",
	}, []string{"result"})
)

func init() {
	Registry.MustRegister(
		ComponentUp,
		ComponentReplicasDesired,
		ComponentReplicasReady,
		SubsystemUp,
		ClusterReady,
		ClusterUp,
		BuildInfo,
		LastPushTimestamp,
		PushTotal,
	)
}

// ResetScanned clears the vec metrics that are rebuilt from a full scan on every
// cycle, so series for deleted components/clusters do not linger as stale values.
func ResetScanned() {
	ComponentUp.Reset()
	ComponentReplicasDesired.Reset()
	ComponentReplicasReady.Reset()
	SubsystemUp.Reset()
	ClusterReady.Reset()
	ClusterUp.Reset()
}

// PushConfig configures a single push to the remote VM.
type PushConfig struct {
	URL   string            // base URL, e.g. http://host:8428
	Job   string            // job label applied to every series
	Token string            // optional Bearer token
	Extra map[string]string // optional extra labels applied to every series
}

// Push gathers the SaFE self-health registry and POSTs it to the remote VM's
// Prometheus text import endpoint. It records push result counters.
func Push(ctx context.Context, client *http.Client, cfg PushConfig) error {
	body, err := gatherText()
	if err != nil {
		PushTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("gather metrics: %w", err)
	}

	endpoint := strings.TrimRight(cfg.URL, "/") + importPath
	// VictoriaMetrics applies each extra_label=<k>=<v> query arg to all imported
	// series, which is how we attach the job (and any static) labels.
	labels := map[string]string{}
	if cfg.Job != "" {
		labels["job"] = cfg.Job
	}
	for k, v := range cfg.Extra {
		labels[k] = v
	}
	if q := encodeExtraLabels(labels); q != "" {
		endpoint += "?" + q
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		PushTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		PushTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("post metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		PushTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("remote VM returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	PushTotal.WithLabelValues("success").Inc()
	LastPushTimestamp.SetToCurrentTime()
	klog.V(4).Infof("[self-health] pushed %d bytes to %s", len(body), endpoint)
	return nil
}

// gatherText renders the registry as Prometheus text-exposition lines
// (name{labels} value). HELP/TYPE headers are omitted; the VM import endpoint
// accepts bare sample lines.
func gatherText() ([]byte, error) {
	mfs, err := Registry.Gather()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	for _, mf := range mfs {
		name := mf.GetName()
		for _, m := range mf.GetMetric() {
			val, ok := metricValue(m)
			if !ok {
				continue
			}
			buf.WriteString(name)
			writeLabels(&buf, m.GetLabel())
			buf.WriteByte(' ')
			buf.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes(), nil
}

func metricValue(m *dto.Metric) (float64, bool) {
	switch {
	case m.Gauge != nil:
		return m.Gauge.GetValue(), true
	case m.Counter != nil:
		return m.Counter.GetValue(), true
	case m.Untyped != nil:
		return m.Untyped.GetValue(), true
	default:
		return 0, false
	}
}

func writeLabels(buf *bytes.Buffer, labels []*dto.LabelPair) {
	if len(labels) == 0 {
		return
	}
	buf.WriteByte('{')
	for i, lp := range labels {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(lp.GetName())
		buf.WriteString(`="`)
		buf.WriteString(escapeLabelValue(lp.GetValue()))
		buf.WriteByte('"')
	}
	buf.WriteByte('}')
}

func escapeLabelValue(v string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	return replacer.Replace(v)
}

func encodeExtraLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		// extra_label=<name>=<value>; VM parses the first '=' as the separator.
		parts = append(parts, "extra_label="+k+"="+labels[k])
	}
	return strings.Join(parts, "&")
}

// SetBool sets a gauge to 1 for true and 0 for false.
func SetBool(g prometheus.Gauge, ok bool) {
	if ok {
		g.Set(1)
	} else {
		g.Set(0)
	}
}
