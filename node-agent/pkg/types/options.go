/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"flag"
	"fmt"
)

type Options struct {
	NodeName      string
	ConfigMapPath string
	ScriptPath    string
	LogfilePath   string
	LogFileSize   int // unit: MB
}

func (opt *Options) Init() error {
	if opt == nil {
		return fmt.Errorf("the options is not initialized")
	}
	flag.StringVar(&opt.NodeName, "node_name", "", "The node name for daemon")
	flag.StringVar(&opt.ConfigMapPath, "configmap_path", "", "The configmap path of node.")
	flag.StringVar(&opt.ScriptPath, "script_path", "", "The script path of node. "+
		"If a path is specified, load from that path; otherwise, use the files within the package.")
	flag.StringVar(&opt.LogfilePath, "log_file_path", "", "Path to the log file")
	flag.IntVar(&opt.LogFileSize, "log_file_size", 0,
		"Defines the maximum size of the log file. Unit is megabytes. "+
			"The default is 0, which means that the size is unlimited.")
	flag.Parse()

	if opt.NodeName == "" {
		return fmt.Errorf("-node_name is not found")
	}
	if opt.ConfigMapPath == "" {
		return fmt.Errorf("-configmap_path is not found")
	}
	return nil
}
