package alerts

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
)

// routeAndNotify routes an alert and sends notifications
func routeAndNotify(ctx context.Context, alert *UnifiedAlert) {
	log.GlobalLogger().WithContext(ctx).Infof("Routing alert: %s", alert.ID)
	
	// Get routing configuration for this alert
	routes := getRoutesForAlert(alert)
	
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

// getRoutesForAlert retrieves routing configurations that match the alert
func getRoutesForAlert(alert *UnifiedAlert) []RouteConfig {
	// TODO: Implement route matching logic
	// This would typically:
	// 1. Load routing rules from database or configuration
	// 2. Match alert labels against route matchers
	// 3. Return matching routes
	
	// For now, return empty to use default route
	return []RouteConfig{}
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

