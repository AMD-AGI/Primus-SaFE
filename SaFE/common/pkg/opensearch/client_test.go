/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"encoding/json"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestQueryIndex(t *testing.T) {
	endTime, err := time.Parse("2006-01-02T15:04:05", "2025-01-01T10:00:00")
	assert.NilError(t, err)

	client := NewClient(SearchClientConfig{DefaultIndex: "node-"}, nil)
	tests := []struct {
		name      string
		SinceTime time.Time
		UntilTime time.Time
		result    string
	}{
		{
			"across 2 months",
			endTime.Add(-time.Hour * 24 * 31),
			endTime,
			client.DefaultIndex + "*",
		},
		{
			"across 2 days",
			endTime.Add(-time.Hour * 24 * 2),
			endTime,
			client.DefaultIndex + "2024.12.30" + "," + client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
		{
			"within the same day",
			endTime.Add(-time.Hour * 2),
			endTime,
			client.DefaultIndex + "2025.01.01",
		},
		{
			"with the same time",
			endTime,
			endTime,
			client.DefaultIndex + "2025.01.01",
		},
		{
			"across 0 o'clock",
			endTime.Add(-time.Hour * 18),
			endTime.Add(-time.Hour * 10).Add(time.Minute),
			client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
		{
			"at 0 o'clock",
			endTime.Add(-time.Hour * 18),
			endTime.Add(-time.Hour * 10),
			client.DefaultIndex + "2024.12.31" + "," + client.DefaultIndex + "2025.01.01",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := client.generateIndexPattern(client.DefaultIndex, test.SinceTime, test.UntilTime)
			assert.NilError(t, err)
			assert.Equal(t, result, test.result)
		})
	}
}

func TestNormalizeLogResponseMessage(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectMsg   []string
		expectChange bool
	}{
		{
			name: "fills empty message from log field",
			input: `{"took":1,"hits":{"total":{"value":2},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","log":"hello from log"}},
				{"_id":"b","_source":{"@timestamp":"t2","message":"already set","log":"shadowed"}}
			]}}`,
			expectMsg:    []string{"hello from log", "already set"},
			expectChange: true,
		},
		{
			name: "no change when message present and log absent",
			input: `{"took":1,"hits":{"total":{"value":1},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","message":"only message"}}
			]}}`,
			expectMsg:    []string{"only message"},
			expectChange: false,
		},
		{
			name: "treats null message as missing",
			input: `{"took":1,"hits":{"total":{"value":1},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","message":null,"log":"from log"}}
			]}}`,
			expectMsg:    []string{"from log"},
			expectChange: true,
		},
		{
			name: "treats empty-string message as missing",
			input: `{"took":1,"hits":{"total":{"value":1},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","message":"","log":"from log"}}
			]}}`,
			expectMsg:    []string{"from log"},
			expectChange: true,
		},
		{
			name: "skips hit when both fields missing",
			input: `{"took":1,"hits":{"total":{"value":1},"hits":[
				{"_id":"a","_source":{"@timestamp":"t1","stream":"stdout"}}
			]}}`,
			expectMsg:    []string{""},
			expectChange: false,
		},
		{
			name: "tolerates empty body",
			input: ``,
			expectMsg:    nil,
			expectChange: false,
		},
		{
			name: "tolerates malformed body",
			input: `{"hits":`,
			expectMsg:    nil,
			expectChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := NormalizeLogResponseMessage([]byte(tt.input))
			if !tt.expectChange {
				assert.Equal(t, string(out), tt.input)
				return
			}
			var parsed struct {
				Hits struct {
					Hits []struct {
						Source struct {
							Message string `json:"message"`
						} `json:"_source"`
					} `json:"hits"`
				} `json:"hits"`
			}
			assert.NilError(t, json.Unmarshal(out, &parsed))
			assert.Equal(t, len(parsed.Hits.Hits), len(tt.expectMsg))
			for i, want := range tt.expectMsg {
				assert.Equal(t, parsed.Hits.Hits[i].Source.Message, want)
			}
		})
	}
}

func TestEffectiveMessageAndNormalize(t *testing.T) {
	resp := &OpenSearchLogResponse{}
	resp.Hits.Hits = make([]OpenSearchLogDoc, 3)
	resp.Hits.Hits[0].Source.Message = "primary"
	resp.Hits.Hits[0].Source.Log = "fallback ignored"
	resp.Hits.Hits[1].Source.Log = "fallback used"
	// hit [2] has neither field.

	assert.Equal(t, resp.Hits.Hits[0].EffectiveMessage(), "primary")
	assert.Equal(t, resp.Hits.Hits[1].EffectiveMessage(), "fallback used")
	assert.Equal(t, resp.Hits.Hits[2].EffectiveMessage(), "")

	resp.NormalizeMessages()
	assert.Equal(t, resp.Hits.Hits[0].Source.Message, "primary")
	assert.Equal(t, resp.Hits.Hits[1].Source.Message, "fallback used")
	assert.Equal(t, resp.Hits.Hits[2].Source.Message, "")
}
