// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	opensearchgo "github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	defaultFlushSize     = 100
	defaultFlushInterval = 5 * time.Second
	indexDateFormat       = "2006.01.02"
)

var (
	writerOnce    sync.Once
	defaultWriter *SnapshotWriter

	writeTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "opensearch_snapshot_write_total",
		Help: "Total documents written to OpenSearch",
	})
	writeErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "opensearch_snapshot_write_errors_total",
		Help: "Total errors writing to OpenSearch",
	})
)

// BulkItem represents a single document to be written to OpenSearch.
type BulkItem struct {
	IndexPrefix string
	Doc         map[string]interface{}
}

// SnapshotWriter buffers documents and flushes them to OpenSearch via the Bulk API.
// It is safe for concurrent use. Documents are flushed when the buffer reaches
// flushSize or every flushInterval, whichever comes first.
// Write failures are logged and counted but never block callers.
type SnapshotWriter struct {
	client        *opensearchgo.Client
	buffer        []*BulkItem
	mu            sync.Mutex
	flushSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
}

// GetWriter returns the process-wide SnapshotWriter, creating it on first call.
// The OpenSearch client is obtained from the current cluster's StorageClientSet.
func GetWriter() *SnapshotWriter {
	writerOnce.Do(func() {
		osClient := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.OpenSearch
		defaultWriter = &SnapshotWriter{
			client:        osClient,
			buffer:        make([]*BulkItem, 0, defaultFlushSize),
			flushSize:     defaultFlushSize,
			flushInterval: defaultFlushInterval,
			stopCh:        make(chan struct{}),
		}
		go defaultWriter.run()
		log.Infof("OpenSearch SnapshotWriter initialized (flushSize=%d, flushInterval=%s)", defaultFlushSize, defaultFlushInterval)
	})
	return defaultWriter
}

// Append adds a document to the write buffer. The document will be indexed
// under "{indexPrefix}-{YYYY.MM.DD}" derived from the @timestamp field.
// This method never blocks.
func (w *SnapshotWriter) Append(indexPrefix string, doc map[string]interface{}) {
	w.mu.Lock()
	w.buffer = append(w.buffer, &BulkItem{IndexPrefix: indexPrefix, Doc: doc})
	shouldFlush := len(w.buffer) >= w.flushSize
	w.mu.Unlock()

	if shouldFlush {
		go w.flush()
	}
}

func (w *SnapshotWriter) run() {
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.flush()
		case <-w.stopCh:
			w.flush()
			return
		}
	}
}

func (w *SnapshotWriter) flush() {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return
	}
	items := w.buffer
	w.buffer = make([]*BulkItem, 0, w.flushSize)
	w.mu.Unlock()

	body, err := buildBulkBody(items)
	if err != nil {
		log.Errorf("Failed to build OpenSearch bulk body: %v", err)
		writeErrors.Add(float64(len(items)))
		return
	}

	req := opensearchapi.BulkRequest{
		Body: bytes.NewReader(body),
	}
	res, err := req.Do(context.Background(), w.client)
	if err != nil {
		log.Errorf("OpenSearch bulk request failed: %v", err)
		writeErrors.Add(float64(len(items)))
		return
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Errorf("OpenSearch bulk request returned error: %s", res.String())
		writeErrors.Add(float64(len(items)))
		return
	}

	writeTotal.Add(float64(len(items)))
	log.Infof("Flushed %d snapshot documents to OpenSearch", len(items))
}

// buildBulkBody constructs an NDJSON payload for the OpenSearch Bulk API.
func buildBulkBody(items []*BulkItem) ([]byte, error) {
	var buf bytes.Buffer
	for _, item := range items {
		ts, _ := item.Doc["@timestamp"].(string)
		indexName := resolveIndexName(item.IndexPrefix, ts)

		metaLine, err := json.Marshal(map[string]interface{}{
			"index": map[string]interface{}{"_index": indexName},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal meta: %w", err)
		}
		docLine, err := json.Marshal(item.Doc)
		if err != nil {
			return nil, fmt.Errorf("marshal doc: %w", err)
		}
		buf.Write(metaLine)
		buf.WriteByte('\n')
		buf.Write(docLine)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func resolveIndexName(prefix, timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t = time.Now()
	}
	return fmt.Sprintf("%s-%s", prefix, t.Format(indexDateFormat))
}
