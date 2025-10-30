/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"fmt"
	"regexp"
)

const (
	DefaultTimeout  = 300
	MonitorIdRule   = "^[a-z0-9][-a-z0-9\\.]{0,61}[a-z0-9]$"
	MonitorIdPrompt = "the name must consist of 1 to 63 lower case alphanumeric characters or '-' or '.'" +
		", start with an alphanumeric character, and end with an alphanumeric character"
)

var (
	MonitorIdRegexp = regexp.MustCompile(MonitorIdRule)
)

type MonitorConfig struct {
	// The unique ID. e.g. "001"
	Id string `json:"id"`
	// The name of the script to be executed
	Script string `json:"script"`
	// Execution interval, default "@every 30s"
	Cronjob string `json:"cronjob"`
	// Timeout duration in seconds. default 300
	TimeoutSecond int `json:"timeoutSecond,omitempty"`
	// If the value is greater than 0, the condition is only satisfied after n consecutive triggers.
	// It is only effective when the operation fails
	ConsecutiveCount int `json:"consecutiveCount,omitempty"`
	// Supported chip vendor. If empty, it means no restrictions. e.g. "amd"
	Chip string `json:"chip,omitempty"`
	// on/off. default "off"
	Toggle string `json:"toggle,omitempty"`
	// Script execution input parameters can include reserved words. They will be automatically replaced by the system with specific content.
	// The following words are currently supported:
	//   1. $Node: Node information, in json format, e.g. '{"nodeName": "testNode", "expectedGpuCount": 8, "observedGpuCount": 8}'
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
		return fmt.Errorf("the id of config is empty")
	}
	if len(conf.Script) == 0 {
		return fmt.Errorf("the script of config is empty")
	}
	if len(conf.Cronjob) == 0 {
		return fmt.Errorf("the cronjob of config is empty")
	}
	return nil
}
