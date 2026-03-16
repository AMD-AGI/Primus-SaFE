// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompbmarshal"
	"github.com/klauspost/compress/snappy"
)

var (
	globalBatchWriter *BatchWriter
	batchWriterOnce   sync.Once
)

// BatchWriter sends batched TimeSeries to vminsert via Prometheus remote write
// protocol, bypassing the per-series Push() of vmalert's remotewrite.Client.
type BatchWriter struct {
	url    string
	client *http.Client
}

func getBatchWriter() *BatchWriter {
	batchWriterOnce.Do(func() {
		cfg := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.Config
		if cfg == nil || cfg.Prometheus == nil {
			log.Errorf("BatchWriter: storage config not available, falling back to per-series Push")
			return
		}
		url := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/insert/0/prometheus/api/v1/write",
			cfg.Prometheus.WriteService, cfg.Prometheus.Namespace, cfg.Prometheus.WritePort)
		globalBatchWriter = &BatchWriter{
			url: url,
			client: &http.Client{
				Transport: &http.Transport{
					MaxIdleConns:        20,
					MaxIdleConnsPerHost: 20,
					IdleConnTimeout:     90 * time.Second,
				},
				Timeout: 30 * time.Second,
			},
		}
		log.Infof("BatchWriter: initialized with URL %s", url)
	})
	return globalBatchWriter
}

func (w *BatchWriter) WriteBatch(tss []prompb.TimeSeries) error {
	if len(tss) == 0 {
		return nil
	}

	marshalTss := make([]prompbmarshal.TimeSeries, len(tss))
	for i, ts := range tss {
		labels := make([]prompbmarshal.Label, len(ts.Labels))
		for j, l := range ts.Labels {
			labels[j] = prompbmarshal.Label{Name: l.Name, Value: l.Value}
		}
		samples := make([]prompbmarshal.Sample, len(ts.Samples))
		for j, s := range ts.Samples {
			samples[j] = prompbmarshal.Sample{Timestamp: s.Timestamp, Value: s.Value}
		}
		marshalTss[i] = prompbmarshal.TimeSeries{Labels: labels, Samples: samples}
	}

	wr := prompbmarshal.WriteRequest{Timeseries: marshalTss}
	data := wr.MarshalProtobuf(nil)
	compressed := snappy.Encode(nil, data)

	req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("send batch: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("remote write returned status %d for %d series", resp.StatusCode, len(tss))
	}
	return nil
}
