// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitopics

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentifyComponentInput(t *testing.T) {
	input := IdentifyComponentInput{
		Workload: WorkloadInfo{
			UID:       "uid-123",
			Name:      "unknown-app",
			Namespace: "default",
			Kind:      "Deployment",
			Labels:    map[string]string{"app": "unknown"},
			Images:    []string{"custom-image:latest"},
		},
		Options: &IdentifyOptions{
			IncludeConfidence: true,
			IncludeRationale:  true,
		},
	}

	assert.Equal(t, "unknown-app", input.Workload.Name)
	assert.True(t, input.Options.IncludeConfidence)
	assert.True(t, input.Options.IncludeRationale)
}

func TestIdentifyOptions(t *testing.T) {
	opts := IdentifyOptions{
		IncludeConfidence: true,
		IncludeRationale:  false,
	}

	assert.True(t, opts.IncludeConfidence)
	assert.False(t, opts.IncludeRationale)
}

func TestIdentifyComponentOutput(t *testing.T) {
	output := IdentifyComponentOutput{
		ComponentType:  "web-server",
		Category:       "frontend",
		Confidence:     0.92,
		Rationale:      "Detected nginx image and port 80 exposure",
		AlternateTypes: []string{"reverse-proxy", "load-balancer"},
	}

	assert.Equal(t, "web-server", output.ComponentType)
	assert.Equal(t, "frontend", output.Category)
	assert.Equal(t, 0.92, output.Confidence)
	assert.Contains(t, output.Rationale, "nginx")
	assert.Len(t, output.AlternateTypes, 2)
}

func TestSuggestGroupingInput(t *testing.T) {
	input := SuggestGroupingInput{
		Workloads: []WorkloadInfo{
			{UID: "uid-1", Name: "app-1"},
			{UID: "uid-2", Name: "app-2"},
			{UID: "uid-3", Name: "app-3"},
		},
		ExistingGroups: []ComponentGroup{
			{GroupID: "group-1", Name: "Web Tier", Members: []string{"uid-4"}},
		},
		Options: &GroupingOptions{
			MaxSuggestions: 5,
			MinConfidence:  0.7,
		},
	}

	assert.Len(t, input.Workloads, 3)
	assert.Len(t, input.ExistingGroups, 1)
	assert.Equal(t, 5, input.Options.MaxSuggestions)
	assert.Equal(t, 0.7, input.Options.MinConfidence)
}

func TestGroupingOptions(t *testing.T) {
	opts := GroupingOptions{
		MaxSuggestions: 10,
		MinConfidence:  0.8,
	}

	assert.Equal(t, 10, opts.MaxSuggestions)
	assert.Equal(t, 0.8, opts.MinConfidence)
}

func TestSuggestGroupingOutput(t *testing.T) {
	output := SuggestGroupingOutput{
		Suggestions: []GroupingSuggestion{
			{
				SuggestionID:  "sug-1",
				GroupName:     "API Services",
				ComponentType: "api-server",
				Category:      "backend",
				Members:       []string{"uid-1", "uid-2"},
				Rationale:     "Similar naming pattern and shared labels",
				Confidence:    0.85,
			},
			{
				SuggestionID:  "sug-2",
				GroupName:     "Workers",
				ComponentType: "worker",
				Category:      "backend",
				Members:       []string{"uid-3"},
				Rationale:     "Job-like workload with no exposed ports",
				Confidence:    0.78,
			},
		},
	}

	assert.Len(t, output.Suggestions, 2)
	assert.Equal(t, "API Services", output.Suggestions[0].GroupName)
	assert.Equal(t, 0.85, output.Suggestions[0].Confidence)
}

func TestGroupingSuggestion(t *testing.T) {
	suggestion := GroupingSuggestion{
		SuggestionID:  "sug-abc",
		GroupName:     "Messaging Layer",
		ComponentType: "message-queue",
		Category:      "messaging",
		Members:       []string{"rabbitmq-1", "rabbitmq-2"},
		Rationale:     "Both workloads use RabbitMQ image and share amqp labels",
		Confidence:    0.93,
	}

	assert.Equal(t, "sug-abc", suggestion.SuggestionID)
	assert.Equal(t, "Messaging Layer", suggestion.GroupName)
	assert.Equal(t, "message-queue", suggestion.ComponentType)
	assert.Equal(t, "messaging", suggestion.Category)
	assert.Len(t, suggestion.Members, 2)
	assert.Contains(t, suggestion.Rationale, "RabbitMQ")
	assert.Equal(t, 0.93, suggestion.Confidence)
}

func TestIdentifyComponentInput_JSON(t *testing.T) {
	input := IdentifyComponentInput{
		Workload: WorkloadInfo{
			UID:    "uid-1",
			Name:   "test-app",
			Kind:   "Deployment",
			Labels: map[string]string{"app": "test"},
		},
		Options: &IdentifyOptions{
			IncludeConfidence: true,
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded IdentifyComponentInput
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "test-app", decoded.Workload.Name)
	assert.True(t, decoded.Options.IncludeConfidence)
}

func TestIdentifyComponentOutput_JSON(t *testing.T) {
	output := IdentifyComponentOutput{
		ComponentType:  "database",
		Category:       "storage",
		Confidence:     0.95,
		AlternateTypes: []string{"cache"},
	}

	data, err := json.Marshal(output)
	require.NoError(t, err)

	var decoded IdentifyComponentOutput
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "database", decoded.ComponentType)
	assert.Equal(t, 0.95, decoded.Confidence)
}

func TestSuggestGroupingInput_JSON(t *testing.T) {
	input := SuggestGroupingInput{
		Workloads: []WorkloadInfo{
			{UID: "uid-1", Name: "app-1"},
		},
		Options: &GroupingOptions{
			MaxSuggestions: 5,
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded SuggestGroupingInput
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Len(t, decoded.Workloads, 1)
	assert.Equal(t, 5, decoded.Options.MaxSuggestions)
}

func TestSuggestGroupingOutput_JSON(t *testing.T) {
	output := SuggestGroupingOutput{
		Suggestions: []GroupingSuggestion{
			{SuggestionID: "sug-1", GroupName: "Test Group"},
		},
	}

	data, err := json.Marshal(output)
	require.NoError(t, err)

	var decoded SuggestGroupingOutput
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Len(t, decoded.Suggestions, 1)
	assert.Equal(t, "Test Group", decoded.Suggestions[0].GroupName)
}

func TestIdentifyComponentOutput_EmptyAlternateTypes(t *testing.T) {
	output := IdentifyComponentOutput{
		ComponentType:  "unknown",
		Category:       "other",
		Confidence:     0.5,
		AlternateTypes: nil,
	}

	assert.Equal(t, "unknown", output.ComponentType)
	assert.Nil(t, output.AlternateTypes)
}

func TestGroupingSuggestion_SingleMember(t *testing.T) {
	suggestion := GroupingSuggestion{
		SuggestionID:  "sug-single",
		GroupName:     "Standalone Service",
		ComponentType: "service",
		Category:      "backend",
		Members:       []string{"single-workload"},
		Confidence:    0.6,
	}

	assert.Len(t, suggestion.Members, 1)
	assert.Equal(t, "single-workload", suggestion.Members[0])
}

