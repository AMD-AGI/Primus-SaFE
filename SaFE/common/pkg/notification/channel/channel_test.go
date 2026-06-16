/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	dbModel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

func TestReadConfigFromFile(t *testing.T) {
	cfg, err := ReadConfigFromFile(`{"email":{"smtp_host":"h","smtp_port":25,"from":"a@b"}}`)
	assert.NoError(t, err)
	assert.NotNil(t, cfg.Email)
	assert.Equal(t, "h", cfg.Email.SMTPHost)

	_, err = ReadConfigFromFile(`not-json`)
	assert.Error(t, err)
}

func TestEmailChannelInit(t *testing.T) {
	e := &EmailChannel{}
	assert.Equal(t, model.ChannelEmail, e.Name())
	assert.Error(t, e.Init(Config{}))
	assert.NoError(t, e.Init(Config{Email: &EmailConfig{From: "a@b"}}))
	// Send without init
	assert.Error(t, (&EmailChannel{}).Send(context.Background(), &model.Message{Email: &model.EmailMessage{}}))
}

func TestEmailRelayChannelPubSub(t *testing.T) {
	e := &EmailRelayChannel{}
	assert.Equal(t, model.ChannelEmailRelay, e.Name())
	// nil config -> error
	assert.Error(t, e.Init(Config{}))

	ch := e.Subscribe()
	e.Notify(&dbModel.EmailOutbox{ID: 1})
	got := <-ch
	assert.Equal(t, int32(1), got.ID)

	// full-buffer drop branch: fill beyond buffer (64) without draining
	for i := 0; i < 70; i++ {
		e.Notify(&dbModel.EmailOutbox{ID: int32(i)})
	}

	e.Unsubscribe(ch)
	// unsubscribe of unknown channel is a no-op
	e.Unsubscribe(make(chan *dbModel.EmailOutbox))
}

func TestEmailRelayInstance(t *testing.T) {
	ch := &EmailRelayChannel{}
	SetEmailRelayInstance(ch)
	assert.NotNil(t, GetEmailRelayInstance())
}

func TestInitChannels(t *testing.T) {
	// both nil -> empty map
	chs, err := InitChannels(context.Background(), &Config{})
	assert.NoError(t, err)
	assert.Empty(t, chs)

	// email branch
	chs, err = InitChannels(context.Background(), &Config{Email: &EmailConfig{From: "a@b"}})
	assert.NoError(t, err)
	assert.Contains(t, chs, model.ChannelEmail)
}
