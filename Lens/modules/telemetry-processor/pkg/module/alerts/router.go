// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package alerts

import (
	"context"
	"encoding/json"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// routeAndNotify routes an alert and sends notifications
func routeAndNotify(ctx context.Context, alert *UnifiedAlert) {
	log.GlobalLogger().WithContext(ctx).Infof("Routing alert: %s", alert.ID)
	
	// Get routing configuration for this alert
	routes := getRoutesForAlert(ctx, alert)
	
	if len(routes) == 0 {
		log.GlobalLogger().WithContext(ctx).Infof("No routes configured for alert: %s, using default notification", alert.ID)
		// Use default notification if no routes match
		routes = []RouteConfig{getDefaultRoute()}
	}
	
	// Send notifications for each route
	for _, route := range routes {
		for _, channel := range route.Channels {
			if err := sendNotification(ctx, alert, channel); err != nil {
				log.GlobalLogger().WithContext(ctx).Errorf("Failed to send notification via %s: %v", channel.Type, err)
			}
		}
	}
}

// AlertRoutingConfig represents the alert_routing field structure
type AlertRoutingConfig struct {
	Enabled  bool              `json:"enabled"`
	Channels []json.RawMessage `json:"channels"`
}

// getRoutesForAlert retrieves routing configurations that match the alert from database
func getRoutesForAlert(ctx context.Context, alert *UnifiedAlert) []RouteConfig {
	facade := database.GetFacade().GetMetricAlertRule()
	
	// Get cluster name from alert labels
	clusterName := alert.ClusterName
	if clusterName == "" {
		clusterName = alert.Labels["cluster"]
	}
	
	// Load all metric alert rules for the cluster
	rules, err := facade.ListMetricAlertRules(ctx, clusterName, nil, nil, nil)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to load metric alert rules: %v", err)
		return []RouteConfig{}
	}
	
	var routes []RouteConfig
	
	for _, rule := range rules {
		if rule.AlertRouting == nil {
			continue
		}
		
		// Parse alert_routing JSON
		routingBytes, err := json.Marshal(rule.AlertRouting)
		if err != nil {
			continue
		}
		
		var routing AlertRoutingConfig
		if err := json.Unmarshal(routingBytes, &routing); err != nil {
			continue
		}
		
		if !routing.Enabled {
			continue
		}
		
		// Check if this rule contains the alertname
		alertMatched := false
		groupsBytes, _ := json.Marshal(rule.Groups)
		if groupsBytes != nil && containsAlertName(string(groupsBytes), alert.AlertName) {
			alertMatched = true
		}
		
		if !alertMatched {
			continue
		}
		
		// Parse channels
		var channels []ChannelConfig
		for _, chRaw := range routing.Channels {
			var ch ChannelConfig
			if err := json.Unmarshal(chRaw, &ch); err != nil {
				continue
			}
			channels = append(channels, ch)
		}
		
		if len(channels) > 0 {
			routes = append(routes, RouteConfig{Channels: channels})
			log.GlobalLogger().WithContext(ctx).Infof("Found routing config for alert %s from rule %s", alert.AlertName, rule.Name)
		}
	}
	
	return routes
}

// containsAlertName checks if the groups JSON contains the alert name
func containsAlertName(groupsJSON string, alertName string) bool {
	return len(alertName) > 0 && len(groupsJSON) > 0 && 
		(contains(groupsJSON, `"alert":"`+alertName+`"`) || contains(groupsJSON, `"alert": "`+alertName+`"`))
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// getDefaultRoute returns the default routing configuration
func getDefaultRoute() RouteConfig {
	return RouteConfig{
		Channels: []ChannelConfig{
			{
				Type: ChannelWebhook,
				Config: map[string]interface{}{
					"url": "http://localhost:8080/webhook",
				},
			},
		},
	}
}

// matchRoute checks if an alert matches a route's matchers
func matchRoute(alert *UnifiedAlert, route RouteConfig) bool {
	// All matchers must match for the route to apply
	for _, matcher := range route.Matchers {
		labelValue, exists := alert.Labels[matcher.Name]
		if !exists {
			return false
		}
		
		if matcher.IsRegex {
			// TODO: Implement regex matching
			continue
		}
		
		if labelValue != matcher.Value {
			return false
		}
	}
	
	return true
}

