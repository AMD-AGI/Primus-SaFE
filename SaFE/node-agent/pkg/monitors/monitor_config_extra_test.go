/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMonitorConfigIsEnable reports on/off toggle state.
func TestMonitorConfigIsEnable(t *testing.T) {
	on := &MonitorConfig{Toggle: "on"}
	off := &MonitorConfig{Toggle: "off"}
	assert.True(t, on.IsEnable())
	assert.False(t, off.IsEnable())
}

// TestMonitorConfigDisabled sets toggle to off.
func TestMonitorConfigDisabled(t *testing.T) {
	conf := &MonitorConfig{Toggle: "on"}
	conf.Disabled()
	assert.Equal(t, conf.Toggle, "off")
}

// TestMonitorConfigEnabled sets toggle to on.
func TestMonitorConfigEnabled(t *testing.T) {
	conf := &MonitorConfig{Toggle: "off"}
	conf.Enabled()
	assert.Equal(t, conf.Toggle, "on")
}

// TestMonitorConfigSetDefaults fills cronjob, timeout, and consecutive count.
func TestMonitorConfigSetDefaults(t *testing.T) {
	conf := &MonitorConfig{}
	conf.SetDefaults()
	assert.Equal(t, conf.Cronjob, "@every 30s")
	assert.Equal(t, conf.TimeoutSecond, DefaultTimeout)
	assert.Equal(t, conf.ConsecutiveCount, 1)
}

// TestMonitorConfigValidateInvalidId rejects malformed monitor ids.
func TestMonitorConfigValidateInvalidId(t *testing.T) {
	conf := &MonitorConfig{Id: "INVALID", Script: "a.sh", Cronjob: "@every 1s"}
	conf.SetDefaults()
	err := conf.Validate()
	assert.Error(t, err)
}

// TestMonitorConfigValidateEmptyScript rejects missing script name.
func TestMonitorConfigValidateEmptyScript(t *testing.T) {
	conf := &MonitorConfig{Id: "safe.01", Script: "", Cronjob: "@every 1s"}
	err := conf.Validate()
	assert.Error(t, err)
}

// TestMonitorConfigValidateEmptyId rejects blank monitor ids.
func TestMonitorConfigValidateEmptyId(t *testing.T) {
	conf := &MonitorConfig{Id: "", Script: "a.sh", Cronjob: "@every 1s"}
	err := conf.Validate()
	assert.Error(t, err)
}

// TestMonitorConfigValidateEmptyCronjob rejects blank cron expressions.
func TestMonitorConfigValidateEmptyCronjob(t *testing.T) {
	conf := &MonitorConfig{Id: "safe.01", Script: "a.sh", Cronjob: ""}
	err := conf.Validate()
	assert.Error(t, err)
}
