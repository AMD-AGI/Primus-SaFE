/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model

const (
	ChannelEmail = "email"
)

const (
	TopicWorkload = "workload"
)

type Message struct {
	Email *EmailMessage
}

// GetChannels returns the list of channels for message delivery.
func (m Message) GetChannels() []string {
	channels := []string{}
	if m.Email != nil {
		channels = append(channels, ChannelEmail)
	}
	return channels
}

type EmailMessage struct {
	To      []string
	Title   string
	Content string
}
