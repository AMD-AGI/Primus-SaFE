/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2a

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// Scanner periodically discovers K8s Services with A2A labels and syncs them to the registry.
type Scanner struct {
	k8sClient  client.Client
	dbClient   dbclient.Interface
	httpClient *http.Client
}

// NewScanner creates a new A2A service scanner.
func NewScanner(k8sClient client.Client, dbClient dbclient.Interface) *Scanner {
	return &Scanner{
		k8sClient:  k8sClient,
		dbClient:   dbClient,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Start begins the scanner loop. It blocks until the context is cancelled.
func (s *Scanner) Start(ctx context.Context) {
	interval := time.Duration(commonconfig.GetA2AScannerInterval()) * time.Second
	klog.InfoS("A2A scanner started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.scan(ctx)
	for {
		select {
		case <-ctx.Done():
			klog.InfoS("A2A scanner stopped")
			return
		case <-ticker.C:
			s.scan(ctx)
		}
	}
}

func (s *Scanner) scan(ctx context.Context) {
	namespaces := commonconfig.GetA2AScannerNamespaces()
	labelSelector := commonconfig.GetA2AScannerLabelSelector()
	labels := parseLabelSelector(labelSelector)

	klog.InfoS("A2A scanner scanning", "namespaces", namespaces, "labelSelector", labelSelector, "parsedLabels", labels)

	if len(namespaces) == 0 {
		var serviceList corev1.ServiceList
		if err := s.k8sClient.List(ctx, &serviceList, client.MatchingLabels(labels)); err != nil {
			klog.ErrorS(err, "failed to list services across all namespaces")
			return
		}
		klog.InfoS("A2A scanner found services", "namespace", "all", "count", len(serviceList.Items))
		for i := range serviceList.Items {
			s.syncService(ctx, &serviceList.Items[i])
		}
		return
	}

	for _, ns := range namespaces {
		var serviceList corev1.ServiceList
		if err := s.k8sClient.List(ctx, &serviceList, client.InNamespace(ns), client.MatchingLabels(labels)); err != nil {
			klog.ErrorS(err, "failed to list services", "namespace", ns)
			continue
		}
		klog.InfoS("A2A scanner found services", "namespace", ns, "count", len(serviceList.Items))
		for i := range serviceList.Items {
			s.syncService(ctx, &serviceList.Items[i])
		}
	}
}

func (s *Scanner) syncService(ctx context.Context, svc *corev1.Service) {
	annotations := svc.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}

	serviceName := annotations["a2a.primus.io/service-name"]
	if serviceName == "" {
		serviceName = svc.Name
	}

	port := annotations["a2a.primus.io/port"]
	if port == "" && len(svc.Spec.Ports) > 0 {
		port = fmt.Sprintf("%d", svc.Spec.Ports[0].Port)
	}

	pathPrefix := annotations["a2a.primus.io/path-prefix"]
	if pathPrefix == "" {
		pathPrefix = "/a2a"
	}

	agentCardPath := annotations["a2a.primus.io/agent-card-path"]
	if agentCardPath == "" {
		agentCardPath = "/.well-known/agent.json"
	}

	endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:%s", svc.Name, svc.Namespace, port)

	agentCard := s.fetchJSON(endpoint + pathPrefix + agentCardPath)
	health := "unknown"
	if s.checkHealth(endpoint + pathPrefix + "/health") {
		health = "healthy"
	} else {
		health = "unhealthy"
	}

	var skills string
	if card, ok := agentCard["skills"]; ok {
		if b, err := json.Marshal(card); err == nil {
			skills = string(b)
		}
	}
	cardJSON, _ := json.Marshal(agentCard)

	now := time.Now().UTC()
	reg := &dbclient.A2AServiceRegistry{
		ServiceName:     serviceName,
		DisplayName:     serviceName,
		Endpoint:        endpoint,
		A2APathPrefix:   pathPrefix,
		A2AAgentCard:    sql.NullString{String: string(cardJSON), Valid: len(cardJSON) > 2},
		A2ASkills:       sql.NullString{String: skills, Valid: skills != ""},
		A2AHealth:       health,
		A2ALastSeen:     pq.NullTime{Time: now, Valid: true},
		K8sNamespace:    sql.NullString{String: svc.Namespace, Valid: true},
		K8sService:      sql.NullString{String: svc.Name, Valid: true},
		DiscoverySource: "k8s-scanner",
		Status:          "active",
	}

	if err := s.dbClient.UpsertA2AService(ctx, reg); err != nil {
		klog.ErrorS(err, "failed to upsert a2a service", "serviceName", serviceName)
	} else {
		klog.V(4).InfoS("synced a2a service", "serviceName", serviceName, "health", health)
	}
}

func (s *Scanner) fetchJSON(url string) map[string]interface{} {
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return map[string]interface{}{}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return map[string]interface{}{}
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}

func (s *Scanner) checkHealth(url string) bool {
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func parseLabelSelector(selector string) map[string]string {
	labels := make(map[string]string)
	if selector == "" {
		return labels
	}
	parts := splitTrim(selector, ",")
	for _, part := range parts {
		kv := splitTrim(part, "=")
		if len(kv) == 2 {
			labels[kv[0]] = kv[1]
		}
	}
	return labels
}

func splitTrim(s, sep string) []string {
	var parts []string
	for _, p := range strings.Split(s, sep) {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
