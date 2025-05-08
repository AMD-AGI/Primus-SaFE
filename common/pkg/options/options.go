/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

func (opt *Options) InitFlags() error {
	if opt == nil {
		return fmt.Errorf("the options is not initialized")
	}
	flag.StringVar(&opt.Config, "config", "", "Path to the primus-ssafe config.yaml")
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
