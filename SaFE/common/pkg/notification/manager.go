/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package notification

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/channel"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/topic"
)

var (
	singleton *Manager
)

// GetNotificationManager returns the singleton notification manager instance.
func GetNotificationManager() *Manager {
	return singleton
}

// InitNotificationManager initializes the notification manager with configuration.
func InitNotificationManager(ctx context.Context, configFile string) error {
	klog.Infof("Notification manager initializing with config file: %s", configFile)
	conf, err := channel.ReadConfigFromFile(configFile)
	if err != nil {
		return err
	}
	channels, err := channel.InitChannels(ctx, conf)
	if err != nil {
		return err
	}
	topics := topic.NewTopics()
	databaseClient := dbClient.NewClient()
	singleton = &Manager{
		channels: channels,
		topics:   topics,
		dbClient: databaseClient,
	}
	singleton.Start(ctx)
	return nil
}

type Manager struct {
	channels map[string]channel.Channel
	topics   map[string]topic.Topic
	dbClient *dbClient.Client
}

// SubmitNotification submits a notification to be processed and sent.
func (m *Manager) SubmitNotification(ctx context.Context, topic, uid string, data map[string]interface{}) error {
	if t, ok := m.topics[topic]; !ok {
		return nil
	} else {
		if !t.Filter(data) {
			return nil
		}
	}
	notification := &model.Notification{
		Data:      data,
		Topic:     topic,
		UID:       uid,
		CreatedAt: time.Now(),
	}
	return m.dbClient.SubmitNotification(ctx, notification)
}

// Start starts the server and begins processing requests.
func (m *Manager) Start(ctx context.Context) {
	go m.doListenNotifications(ctx)
}

func (m *Manager) doListenNotifications(ctx context.Context) {
	for {
		err := m.listenNotifications(ctx)
		if err != nil {
			klog.Errorf("failed to listen notifications: %v", err)
		}
		select {
		case <-ctx.Done():
			klog.Infof("notification manager stopping")
			return
		default:
		}
		time.Sleep(5 * time.Second)
	}
}

func (m *Manager) listenNotifications(ctx context.Context) error {
	unprocessed, err := m.dbClient.ListUnprocessedNotifications(ctx)
	if err != nil {
		return err
	}
	for _, notification := range unprocessed {
		if err := m.SubmitMessage(ctx, notification); err != nil {
			return err
		}
		notification.SentAt = time.Now()
		if err := m.dbClient.UpdateNotification(ctx, notification); err != nil {
			return err
		}
	}
	return nil
}

// SubmitMessage submits a pre-built notification message for delivery.
func (m *Manager) SubmitMessage(ctx context.Context, data *model.Notification) error {
	t, exists := m.topics[data.Topic]
	if !exists {
		return nil
	}
	messages, err := t.BuildMessage(ctx, data.Data)
	if err != nil {
		klog.Errorf("failed to build message for topic %s: %v", data.Topic, err)
		return err
	}
	for _, msg := range messages {
		channelNames := msg.GetChannels()
		for _, chName := range channelNames {
			ch, exists := m.channels[chName]
			if !exists {
				klog.Warningf("channel %s does not exist", chName)
				continue
			}
			if err := ch.Send(ctx, msg); err != nil {
				klog.Errorf("failed to send message to channel %s: %v", chName, err)
				return err
			}
		}
	}
	return nil
}
