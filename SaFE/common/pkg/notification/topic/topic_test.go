/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package topic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

func TestNewTopics(t *testing.T) {
	topics := NewTopics()
	assert.Contains(t, topics, model.TopicWorkload)
	assert.Equal(t, model.TopicWorkload, topics[model.TopicWorkload].Name())
}
