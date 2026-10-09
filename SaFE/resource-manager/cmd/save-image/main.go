/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Command save-image runs inside a workload's container. The platform launcher runs
// "save-image record" before anything else, to record the files the container started
// with. The resource manager runs "save-image export" through pods/exec when the user
// saves the container as an image; it reads its request from standard input and prints
// its response on standard output.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/ops_job/exportimage/agent"
)

// maxRequest bounds what is read from standard input: a token and a CA bundle.
const maxRequest = 1 << 20

func main() {
	// The agent shares the container's memory limit with the user's processes.
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(256 << 20)
	}
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: %s record|export", os.Args[0]))
	}
	switch os.Args[1] {
	case "record":
		fail(record())
	case "export":
		fail(export())
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "save-image:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func mountinfo() (string, error) {
	b, err := os.ReadFile("/proc/self/mountinfo")
	return string(b), err
}

func record() error {
	start := time.Now()
	// Whatever happens below, no record from an earlier container is left behind.
	if err := os.Remove(agent.BaselinePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	mi, err := mountinfo()
	if err != nil {
		return err
	}
	n, err := agent.Record(agent.BaselinePath, "/", mi)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "save-image: recorded %d paths in %s\n", n, time.Since(start).Round(time.Millisecond))
	return nil
}

func export() error {
	var req agent.Request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, maxRequest)).Decode(&req); err != nil {
		return fmt.Errorf("reading the request: %w", err)
	}
	mi, err := mountinfo()
	if err != nil {
		return err
	}
	resp, err := agent.Export(context.Background(), req, agent.Env{
		Root:      "/",
		Baseline:  agent.BaselinePath,
		RunFile:   agent.LauncherRunFile,
		Mountinfo: mi,
		UID:       os.Getuid(),
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(resp)
}
