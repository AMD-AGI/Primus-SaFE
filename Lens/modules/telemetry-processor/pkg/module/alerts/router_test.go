// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package alerts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchRoute(t *testing.T) {
	tests := []struct {
		name     string
		alert    *UnifiedAlert
		route    RouteConfig
		expected bool
	}{
		{
			name: "exact match single matcher",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"severity": "critical",
					"team":     "ml",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "severity", Value: "critical"},
				},
			},
			expected: true,
		},
		{
			name: "exact match multiple matchers",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"severity": "critical",
					"team":     "ml",
					"env":      "production",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "severity", Value: "critical"},
					{Name: "team", Value: "ml"},
				},
			},
			expected: true,
		},
		{
			name: "no match wrong value",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"severity": "warning",
					"team":     "ml",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "severity", Value: "critical"},
				},
			},
			expected: false,
		},
		{
			name: "no match missing label",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"team": "ml",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "severity", Value: "critical"},
				},
			},
			expected: false,
		},
		{
			name: "one matcher fails all fail",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"severity": "critical",
					"team":     "ml",
					"env":      "staging",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "severity", Value: "critical"},
					{Name: "team", Value: "ml"},
					{Name: "env", Value: "production"},
				},
			},
			expected: false,
		},
		{
			name: "empty matchers always match",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"severity": "critical",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{},
			},
			expected: true,
		},
		{
			name: "empty alert labels no match",
			alert: &UnifiedAlert{
				Labels: map[string]string{},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "severity", Value: "critical"},
				},
			},
			expected: false,
		},
		{
			name: "case sensitive matching",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"severity": "Critical",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "severity", Value: "critical"},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchRoute(tt.alert, tt.route)
			assert.Equal(t, tt.expected, result, "Match result should match expected")
		})
	}
}

func TestGetDefaultRoute(t *testing.T) {
	route := getDefaultRoute()

	assert.NotNil(t, route, "Default route should not be nil")
	assert.NotEmpty(t, route.Channels, "Default route should have channels")
	assert.Equal(t, 1, len(route.Channels), "Default route should have exactly one channel")

	defaultChannel := route.Channels[0]
	assert.Equal(t, ChannelWebhook, defaultChannel.Type, "Default channel should be webhook")
	assert.NotNil(t, defaultChannel.Config, "Default channel config should not be nil")
	assert.Contains(t, defaultChannel.Config, "url", "Webhook config should contain URL")
}

func TestMatchRouteWithComplexLabels(t *testing.T) {
	tests := []struct {
		name     string
		alert    *UnifiedAlert
		route    RouteConfig
		expected bool
	}{
		{
			name: "match with special characters in values",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"workload_id": "ml-training-job-v1.2.3",
					"namespace":   "team-ml",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "workload_id", Value: "ml-training-job-v1.2.3"},
				},
			},
			expected: true,
		},
		{
			name: "match with empty string value",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"optional_label": "",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "optional_label", Value: ""},
				},
			},
			expected: true,
		},
		{
			name: "no match with empty string vs missing",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"other_label": "value",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "optional_label", Value: ""},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchRoute(tt.alert, tt.route)
			assert.Equal(t, tt.expected, result, "Match result should match expected")
		})
	}
}

func TestMatchRouteWithRegexMatchers(t *testing.T) {
	// Note: Regex matching is TODO in the current implementation
	// These tests document expected behavior when implemented
	tests := []struct {
		name     string
		alert    *UnifiedAlert
		route    RouteConfig
		expected bool
		skip     bool
	}{
		{
			name: "regex matcher - currently skipped",
			alert: &UnifiedAlert{
				Labels: map[string]string{
					"workload_id": "training-job-123",
				},
			},
			route: RouteConfig{
				Matchers: []Matcher{
					{Name: "workload_id", Value: "training-job-.*", IsRegex: true},
				},
			},
			expected: true,
			skip:     true, // Skip because regex not implemented yet
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("Regex matching not yet implemented")
			}
			result := matchRoute(tt.alert, tt.route)
			assert.Equal(t, tt.expected, result, "Match result should match expected")
		})
	}
}

func TestGetRoutesForAlert(t *testing.T) {
	// Current implementation returns empty slice
	alert := &UnifiedAlert{
		Labels: map[string]string{
			"severity": "critical",
		},
	}

	routes := getRoutesForAlert(alert)

	// Currently returns empty, will be implemented later
	assert.NotNil(t, routes, "Routes should not be nil")
	assert.Empty(t, routes, "Routes should be empty (not yet implemented)")
}

func TestRouteConfigStructure(t *testing.T) {
	route := RouteConfig{
		Matchers: []Matcher{
			{Name: "severity", Value: "critical", IsRegex: false},
			{Name: "team", Value: "ml-.*", IsRegex: true},
		},
		Channels: []ChannelConfig{
			{
				Type: ChannelWebhook,
				Config: map[string]interface{}{
					"url": "http://webhook.example.com",
				},
			},
			{
				Type: ChannelEmail,
				Config: map[string]interface{}{
					"to": "team@example.com",
				},
			},
		},
		GroupBy:        []string{"alertname", "severity"},
		GroupWait:      "30s",
		GroupInterval:  "5m",
		RepeatInterval: "4h",
	}

	assert.Equal(t, 2, len(route.Matchers), "Should have 2 matchers")
	assert.Equal(t, 2, len(route.Channels), "Should have 2 channels")
	assert.Equal(t, 2, len(route.GroupBy), "Should have 2 group by fields")
	assert.NotEmpty(t, route.GroupWait, "GroupWait should not be empty")
	assert.NotEmpty(t, route.RepeatInterval, "RepeatInterval should not be empty")
}

func TestMatcherStructure(t *testing.T) {
	tests := []struct {
		name    string
		matcher Matcher
	}{
		{
			name: "simple exact matcher",
			matcher: Matcher{
				Name:    "severity",
				Value:   "critical",
				IsRegex: false,
			},
		},
		{
			name: "regex matcher",
			matcher: Matcher{
				Name:    "workload_id",
				Value:   "prod-.*",
				IsRegex: true,
			},
		},
		{
			name: "matcher with empty value",
			matcher: Matcher{
				Name:    "optional",
				Value:   "",
				IsRegex: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.matcher.Name, "Matcher name should not be empty")
		})
	}
}

func TestChannelConfigStructure(t *testing.T) {
	tests := []struct {
		name    string
		channel ChannelConfig
	}{
		{
			name: "webhook channel",
			channel: ChannelConfig{
				Type: ChannelWebhook,
				Config: map[string]interface{}{
					"url":    "http://example.com/webhook",
					"method": "POST",
				},
			},
		},
		{
			name: "email channel",
			channel: ChannelConfig{
				Type: ChannelEmail,
				Config: map[string]interface{}{
					"to":      "alerts@example.com",
					"from":    "noreply@example.com",
					"subject": "Alert Notification",
				},
			},
		},
		{
			name: "dingtalk channel",
			channel: ChannelConfig{
				Type: ChannelDingTalk,
				Config: map[string]interface{}{
					"webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=xxx",
				},
			},
		},
		{
			name: "slack channel",
			channel: ChannelConfig{
				Type: ChannelSlack,
				Config: map[string]interface{}{
					"webhook_url": "https://hooks.slack.com/services/xxx",
					"channel":     "#alerts",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.channel.Type, "Channel type should not be empty")
			assert.NotNil(t, tt.channel.Config, "Channel config should not be nil")
			assert.NotEmpty(t, tt.channel.Config, "Channel config should have entries")
		})
	}
}

