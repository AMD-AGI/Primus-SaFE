/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/klog/v2"

	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbModel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

type EmailRelayConfig struct {
	Enable    bool   `json:"enable" yaml:"enable"`
	AuthToken string `json:"auth_token,omitempty" yaml:"auth_token"`
}

type EmailRelayChannel struct {
	cfg       *EmailRelayConfig
	dbClient  *dbClient.Client
	mu        sync.RWMutex
	listeners []chan *dbModel.EmailOutbox
}

func (e *EmailRelayChannel) Name() string {
	return model.ChannelEmailRelay
}

func (e *EmailRelayChannel) Init(cfg Config) error {
	if cfg.EmailRelay == nil {
		return fmt.Errorf("email relay config not provided")
	}
	e.cfg = cfg.EmailRelay
	e.dbClient = dbClient.NewClient()
	if e.dbClient == nil {
		return fmt.Errorf("database client not available, email relay requires database")
	}
	return nil
}

// Send writes the email message to the outbox table and notifies SSE listeners.
func (e *EmailRelayChannel) Send(ctx context.Context, message *model.Message) error {
	if e.cfg == nil {
		return fmt.Errorf("email relay channel not initialized")
	}
	if message == nil || message.Email == nil {
		return fmt.Errorf("message is nil or has no email content")
	}

	msg := message.Email
	if len(msg.To) == 0 {
		return fmt.Errorf("no recipients provided for email relay")
	}

	outbox := &dbModel.EmailOutbox{
		Source:      dbModel.EmailOutboxSourceSafe,
		Recipients:  dbModel.StringArray(msg.To),
		Subject:     msg.Title,
		HTMLContent: msg.Content,
		Status:      dbModel.EmailOutboxStatusPending,
	}

	if err := e.dbClient.CreateEmailOutbox(ctx, outbox); err != nil {
		return fmt.Errorf("failed to write email to outbox: %w", err)
	}

	klog.Infof("Email queued in outbox (id=%d) for relay: subject=%q, to=%v", outbox.ID, msg.Title, msg.To)
	e.notifyListeners(outbox)
	return nil
}

// Subscribe returns a channel that receives new outbox entries in real-time.
func (e *EmailRelayChannel) Subscribe() chan *dbModel.EmailOutbox {
	ch := make(chan *dbModel.EmailOutbox, 64)
	e.mu.Lock()
	e.listeners = append(e.listeners, ch)
	e.mu.Unlock()
	return ch
}

// Unsubscribe removes a listener channel.
func (e *EmailRelayChannel) Unsubscribe(ch chan *dbModel.EmailOutbox) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, listener := range e.listeners {
		if listener == ch {
			e.listeners = append(e.listeners[:i], e.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

func (e *EmailRelayChannel) notifyListeners(outbox *dbModel.EmailOutbox) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, ch := range e.listeners {
		select {
		case ch <- outbox:
		default:
			klog.Warningf("Email relay listener channel full, dropping notification for outbox id=%d", outbox.ID)
		}
	}
}

var (
	relayInstance     *EmailRelayChannel
	relayInstanceOnce sync.Once
)

// GetEmailRelayInstance returns the singleton EmailRelayChannel if initialized.
func GetEmailRelayInstance() *EmailRelayChannel {
	return relayInstance
}

// SetEmailRelayInstance stores the relay channel singleton for SSE handler access.
func SetEmailRelayInstance(ch *EmailRelayChannel) {
	relayInstanceOnce.Do(func() {
		relayInstance = ch
	})
}
