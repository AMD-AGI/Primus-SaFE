/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"fmt"
	"regexp"
)

const (
	DefaultTimeout  = 300
	MonitorIdRule   = "^[a-z][-a-z0-9\\.]{0,61}[a-z0-9]$"
	MonitorIdPrompt = "the name must consist of 1 to 63 lower case alphanumeric characters or '-' or '.'" +
		", start with an alphabetic character, and end with an alphanumeric character"
)

var (
	MonitorIdRegexp = regexp.MustCompile(MonitorIdRule)
)

type MonitorConfig struct {
	// The unique ID identifier. e.g., "safe.001"
	Id string `json:"id"`
	// The name of the script to be executed
	Script string `json:"script"`
	// Execution interval, default "@every 30s"
	Cronjob string `json:"cronjob"`
	// Timeout duration in seconds. default 300
	TimeoutSecond int `json:"timeoutSecond,omitempty"`
	// It triggers when the condition is met N consecutive times. default 1
	// It is only effective when the operation fails
	ConsecutiveCount int `json:"consecutiveCount,omitempty"`
	// Supported chip vendor. If empty, it means no restrictions. e.g., "amd"
	Chip string `json:"chip,omitempty"`
	// on/off. default "off"
	Toggle string `json:"toggle,omitempty"`
	// The following reserved keywords will be automatically passed to the script by the system::
	//   1. $Node: Node information, in json format, e.g., '{"nodeIp": "10.0.0.1", "nodeName": "testNode", "gpuSpecCount": 8}'
	Arguments []string `json:"arguments,omitempty"`
}

func (conf *MonitorConfig) IsEnable() bool {
	return conf.Toggle == "on"
}

func (conf *MonitorConfig) Disabled() {
	conf.Toggle = "off"
}

func (conf *MonitorConfig) Enabled() {
	conf.Toggle = "on"
}

func (conf *MonitorConfig) SetDefaults() {
	if conf.Cronjob == "" {
		conf.Cronjob = "@every 30s"
	}
	if conf.TimeoutSecond <= 0 {
		conf.TimeoutSecond = DefaultTimeout
	}
	if conf.ConsecutiveCount <= 0 {
		conf.ConsecutiveCount = 1
	}
}

func (conf *MonitorConfig) Validate() error {
	if !MonitorIdRegexp.MatchString(conf.Id) {
		return fmt.Errorf(MonitorIdPrompt)
	}
	if len(conf.Id) == 0 {
		return fmt.Errorf("the id of config is not found")
	}
	if len(conf.Script) == 0 {
		return fmt.Errorf("the script of config is not found")
	}
	if len(conf.Cronjob) == 0 {
		return fmt.Errorf("the cronjob of config is not found")
	}
	return nil
}
