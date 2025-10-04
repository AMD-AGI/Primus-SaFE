/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"flag"
	"fmt"
)

type Options struct {
	Config      string
	CertDir     string
	LogFileSize int // unit: MB
	LogfilePath string
}

func (opt *Options) InitFlags() error {
	if opt == nil {
		return fmt.Errorf("the options is not initialized")
	}
	flag.StringVar(&opt.Config, "config", "", "Path to the primus-safe config.yaml")
	flag.StringVar(&opt.CertDir, "cert_dir", "", "The cert dir for webhooks.")
	flag.IntVar(&opt.LogFileSize, "log_file_size", 0,
		"Defines the maximum size of the log file. Unit is megabytes. "+
			"The default is 0, which means that the size is unlimited.")
	flag.StringVar(&opt.LogfilePath, "log_file_path", "", "Path to the log file")
	flag.Parse()

	if opt.Config == "" {
		return fmt.Errorf("-config is not found")
	}
	if opt.CertDir == "" {
		return fmt.Errorf("-cert_dir is not found")
	}
	return nil
}
