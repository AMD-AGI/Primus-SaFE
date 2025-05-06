/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package log

import (
	"flag"
	"strconv"

	"k8s.io/klog/v2"
)

func Init(logfilePath string, logFileSize int) error {
	klog.InitFlags(nil)
	flag.Set("log_file", logfilePath)
	flag.Set("alsologtostderr", "true") // Also log to stderr.
	flag.Set("logtostderr", "false")
	flag.Set("skip_log_headers", "true")
	if logFileSize != 0 {
		flag.Set("log_file_max_size", strconv.Itoa(logFileSize))
	}
	flag.Parse()
	return nil
}
