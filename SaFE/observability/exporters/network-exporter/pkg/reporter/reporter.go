// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/model"
)

// Config holds the reporter configuration.
type Config struct {
	Enabled  bool
	Endpoint string        // fault-manager HTTP endpoint
	Interval time.Duration // report interval (default 60s)
}

// Reporter sends aggregated flow data to fault-manager via HTTP POST.
type Reporter struct {
	config Config
	client *http.Client
}

// New creates a new Reporter. Returns nil if not enabled.
func New(cfg Config) *Reporter {
	if !cfg.Enabled || cfg.Endpoint == "" {
		return nil
	}
	if cfg.Interval == 0 {
		cfg.Interval = 60 * time.Second
	}
	return &Reporter{
		config: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Interval returns the configured report interval.
func (r *Reporter) Interval() time.Duration {
	return r.config.Interval
}

// Send posts the report payload to the fault-manager endpoint.
func (r *Reporter) Send(ctx context.Context, payload *model.ReportPayload) error {
	if len(payload.Flows) == 0 {
		slog.Debug("no flows to report, skipping")
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal report payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create report request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("send report to fault-manager: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fault-manager returned status %d", resp.StatusCode)
	}

	slog.Info("reported flows to fault-manager", "count", len(payload.Flows))
	return nil
}
