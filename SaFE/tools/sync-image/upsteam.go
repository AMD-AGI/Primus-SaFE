/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/containers/image/v5/types"
)

const (
	importImageUpsteamPath = "http://%s/api/v1/images:import/%s/progress"
)

const (
	DefaultTimeout = 30 * time.Second
)

var (
	client = NewClient()
)

// NewClient creates a new OpenSearch client with the provided configuration.
func NewClient() *http.Client {
	return &http.Client{
		Timeout: DefaultTimeout,
	}
}

type UpstreamEvent struct {
	Data              map[string]types.ProgressProperties `json:"data"`
	SyncLayerCount    int                                 `json:"syncLayerCount"`
	ComplexLayerCount int                                 `json:"complexLayerCount"`
	SkipLayerCount    int                                 `json:"skipLayerCount"`
}

// applyProgress records a progress event and updates layer counters.
// It returns true when the event is terminal (skipped or done) and should be reported upstream.
func (e *UpstreamEvent) applyProgress(p types.ProgressProperties) bool {
	e.Data[p.Artifact.Digest.String()] = p
	switch p.Event {
	case types.ProgressEventSkipped:
		e.SkipLayerCount++
		e.SyncLayerCount++
		return true
	case types.ProgressEventDone:
		e.ComplexLayerCount++
		e.SyncLayerCount++
		return true
	}
	return false
}

func upstreamData(domain, imageName string, data *UpstreamEvent) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	encodedImageName := base64.URLEncoding.EncodeToString([]byte(imageName))
	url := fmt.Sprintf(importImageUpsteamPath, domain, encodedImageName)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s unexpected status code: %d", url, resp.StatusCode)
	}

	return nil
}
