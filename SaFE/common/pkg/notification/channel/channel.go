/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

import (
	"context"
	"encoding/json"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

type Config struct {
	Email *EmailConfig `json:"email,omitempty" yaml:"email"`
}

type EmailConfig struct {
	SMTPHost string `json:"smtp_host" yaml:"smtp_host"`
	SMTPPort int    `json:"smtp_port" yaml:"smtp_port"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	From     string `json:"from" yaml:"from"`
	UseTLS   bool   `json:"use_tls" yaml:"use_tls"`
}

// ReadConfigFromFile reads notification configuration from a file.
func ReadConfigFromFile(data string) (*Config, error) {
	c := &Config{}
	err := json.Unmarshal([]byte(data), c)
	if err != nil {
		return nil, err
	}
	return c, nil
}

type Channel interface {
	Init(cfg Config) error
	Name() string
	Send(ctx context.Context, message *model.Message) error
}

// InitChannels initializes all notification channels from the configuration.
func InitChannels(ctx context.Context, conf *Config) (map[string]Channel, error) {
	channels := make(map[string]Channel)
	if conf.Email != nil {
		emailChannel := &EmailChannel{}
		if err := emailChannel.Init(*conf); err != nil {
			return nil, err
		}
		channels[emailChannel.Name()] = emailChannel
	}
	return channels, nil
}
