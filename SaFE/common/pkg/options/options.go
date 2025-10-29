/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package options

import (
	"flag"
	"fmt"
)

type Options struct {
	Config         string
	KubeConfig     string
	LogfilePath    string
	LogFileSize    int
	SSHKeyFilePath string
}

// InitFlags initializes the command line flags for the application.
// It sets up the following flags:
//
//	-config: Path to the primus-safe config.yaml (required)
//	-kube_config: Path to the kubectl config
//	-log_file_size: Maximum size of the log file in megabytes (default: 0, unlimited)
//	-log_file_path: Path to the log file
//
// After parsing flags, it validates that the config path is provided.
// Returns an error if the options struct is nil or if the required -config flag is not provided.
func (opt *Options) InitFlags() error {
	if opt == nil {
		return fmt.Errorf("the options is not initialized")
	}
	flag.StringVar(&opt.Config, "config", "", "Path to the primus-safe config.yaml")
	flag.StringVar(&opt.KubeConfig, "kube_config", "", "Path to the kubectl config")
	flag.IntVar(&opt.LogFileSize, "log_file_size", 0,
		"Defines the maximum size of the log file. Unit is megabytes. "+
			"The default is 0, which means that the size is unlimited.")
	flag.StringVar(&opt.LogfilePath, "log_file_path", "", "Path to the log file")
	flag.Parse()
	if opt.Config == "" {
		return fmt.Errorf("-config is not found")
	}

	return nil
}
