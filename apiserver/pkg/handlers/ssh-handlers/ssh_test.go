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

// ws://149.28.117.31:32736/api/v1/workloads/test-ssh-8kc7b/pods/test-ssh-8kc7b-master-0/webshell?namespace=safe-cluster-dev&rows=1800&cols=40&container=pytorch&cmd=bash
// ssh 6893ea02fd55c76ec4bc2ff8136f39f4.test-ssh-8kc7b-master-0.pytorch.bash.safe-cluster-dev@149.28.117.31 -p 31748
func TestParseSSHInfo(t *testing.T) {
	info := &UserInfo{}
	user := "root.primus-test-master-0.main.bash.primus-safe-dev"
	ok, _ := restructure.Find(info, user)
	assert.Equal(t, true, ok)
	assert.Equal(t, "root", info.User)
	assert.Equal(t, "primus-test-master-0", info.Pod)
	assert.Equal(t, "main", info.Container)
	assert.Equal(t, "bash", info.CMD)
	assert.Equal(t, "primus-safe-dev", info.Namespace)
}
