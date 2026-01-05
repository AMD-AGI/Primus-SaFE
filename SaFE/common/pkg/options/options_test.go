/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package options

import (
	"os"
	"testing"

	"gotest.tools/assert"
)

func TestAddFlags(t *testing.T) {
	opts := &Options{}
	os.Args = []string{
		"test",
		"--config=./conf/config.yaml",
		"--log_file_size=10240",
		"--log_file_path=./log",
	}
	opts.InitFlags()

	t.Run("test parse arguments",
		func(t *testing.T) {
			assert.Equal(t, opts.Config, "./conf/config.yaml")
			assert.Equal(t, opts.LogFileSize, 10240)
			assert.Equal(t, opts.LogfilePath, "./log")
		},
	)
}
