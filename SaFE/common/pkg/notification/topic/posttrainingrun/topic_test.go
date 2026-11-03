/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttrainingrun

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// payload returns the notification data as the manager stores it: a JSON round trip of what the controller sent.
func payload(t *testing.T, d TopicData) map[string]interface{} {
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	out := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestFilter(t *testing.T) {
	topic := &Topic{}
	require.True(t, topic.Filter(map[string]interface{}{"event": EventFailed}))
	require.False(t, topic.Filter(map[string]interface{}{"event": "running"}))
	require.False(t, topic.Filter(map[string]interface{}{}))
}

func TestBuildMessage(t *testing.T) {
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1",
		Annotations: map[string]string{v1.UserEmailAnnotation: "u1@amd.com"}}}
	d := TopicData{Event: EventFailed, Users: []*v1.User{user}, Run: RunInfo{
		RunID: "pt-qwen-abc12", DisplayName: "qwen-sft", BaseModel: "Qwen3-8B", TrainType: "sft", Status: "Failed",
		FailedStage: "train", Message: "<b>OOM</b>", Duration: "1h2m", MainMetric: "train/loss 0.42 at step 340",
		Scores: map[string]float64{"gsm8k": 0.71}}}
	msgs, err := (&Topic{}).BuildMessage(context.Background(), payload(t, d))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	email := msgs[0].Email
	require.Equal(t, []string{"u1@amd.com"}, email.To)
	require.Equal(t, "[Primus Safe] Post Training run failed ( qwen-sft )", email.Title)
	require.Contains(t, email.Content, "Failed at train")
	require.Contains(t, email.Content, "/post-training/detail?runId=pt-qwen-abc12")
	require.Contains(t, email.Content, "&lt;b&gt;OOM&lt;/b&gt;", "the message is escaped")
	require.Contains(t, email.Content, "gsm8k")

	d.Users = nil
	msgs, err = (&Topic{}).BuildMessage(context.Background(), payload(t, d))
	require.NoError(t, err)
	require.Empty(t, msgs, "no recipients, no email")
}
