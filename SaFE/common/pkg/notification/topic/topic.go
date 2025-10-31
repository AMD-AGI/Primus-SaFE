/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package topic

import (
	"context"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/topic/workload"
)

type Topic interface {
	Name() string
	BuildMessage(ctx context.Context, data map[string]interface{}) ([]*model.Message, error)
	Filter(data map[string]interface{}) bool
}

// NewTopics creates and returns all supported notification topics.
func NewTopics() map[string]Topic {
	topics := make(map[string]Topic)
	workloadTopic := &workload.Topic{}
	topics[workloadTopic.Name()] = workloadTopic

	return topics
}
