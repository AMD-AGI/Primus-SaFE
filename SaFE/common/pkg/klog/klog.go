/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package klog

import (
	"flag"
	"strconv"

	"k8s.io/klog/v2"
)

// Init initializes the klog logging system with the specified log file path and maximum log file size.
// It sets up logging to both file and stderr, skips log headers, and parses the flags.
func Init(logfilePath string, logFileSize int) error {
	klog.InitFlags(nil)
	flag.Set("log_file", logfilePath)
	flag.Set("alsologtostderr", "true")
	flag.Set("logtostderr", "false")
	flag.Set("skip_log_headers", "true")
	if logFileSize != 0 {
		flag.Set("log_file_max_size", strconv.Itoa(logFileSize))
	}
	flag.Parse()
	return nil
}
