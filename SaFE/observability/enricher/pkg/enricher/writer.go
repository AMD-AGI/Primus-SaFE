/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package enricher

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Writer remote-writes enriched series to VictoriaMetrics using the Prometheus
// text exposition import endpoint (/api/v1/import/prometheus), which avoids the
// protobuf/snappy remote-write machinery while still landing native samples.
type Writer struct {
	url        string
	httpClient *http.Client
}

func NewWriter(url string, timeout time.Duration) *Writer {
	return &Writer{url: url, httpClient: &http.Client{Timeout: timeout}}
}

// series is one emitted sample: metric name + labels + value.
type series struct {
	name   string
	labels map[string]string
	value  float64
}

// Write formats the series as Prometheus text and POSTs them in one request.
func (w *Writer) Write(ctx context.Context, batch []series) error {
	if len(batch) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, s := range batch {
		buf.WriteString(s.name)
		writeLabels(&buf, s.labels)
		buf.WriteByte(' ')
		buf.WriteString(strconv.FormatFloat(s.value, 'g', -1, 64))
		buf.WriteByte('\n')
	}

	reqCtx, cancel := context.WithTimeout(ctx, w.httpClient.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, w.url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vm import request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("vm import returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// writeLabels renders a Prometheus label set in a stable (sorted) order.
func writeLabels(buf *bytes.Buffer, labels map[string]string) {
	if len(labels) == 0 {
		return
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(k)
		buf.WriteString(`="`)
		buf.WriteString(escapeLabelValue(labels[k]))
		buf.WriteByte('"')
	}
	buf.WriteByte('}')
}

// escapeLabelValue escapes backslash, double-quote and newline per the
// Prometheus text format.
func escapeLabelValue(v string) string {
	if !strings.ContainsAny(v, "\\\"\n") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}
