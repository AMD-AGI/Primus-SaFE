/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageChannels(t *testing.T) {
	empty := Message{}
	assert.Empty(t, empty.GetChannels())
	assert.Empty(t, empty.GetRelayChannels())

	m := Message{Email: &EmailMessage{To: []string{"a@b.com"}, Title: "t", Content: "c"}}
	assert.Equal(t, []string{ChannelEmail}, m.GetChannels())
	assert.Equal(t, []string{ChannelEmailRelay}, m.GetRelayChannels())
}
