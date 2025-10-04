/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"testing"

	"github.com/alexflint/go-restructure"
	"gotest.tools/assert"
)

func TestParseSSHInfo(t *testing.T) {
	info := &UserInfo{}
	user := "root.primus-test-master-0.primus-safe-dev"
	ok, _ := restructure.Find(info, user)
	assert.Equal(t, true, ok)
	assert.Equal(t, "root", info.User)
	assert.Equal(t, "primus-test-master-0", info.Pod)
	assert.Equal(t, "primus-safe-dev", info.Namespace)
}
