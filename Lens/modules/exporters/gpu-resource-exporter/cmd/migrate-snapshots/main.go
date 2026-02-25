// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// migrate-snapshots reads historical pod_snapshot and gpu_workload_snapshot rows
// from PostgreSQL and bulk-indexes them into OpenSearch. It is designed to be
// run once (or re-run idempotently) after the code switches to the upsert+OS
// write path, to back-fill timeline data.
//
// Usage:
//
//	go run ./cmd/migrate-snapshots \
//	  --pg-dsn "postgres://user:pass@host:5432/primus-lens?sslmode=require" \
//	  --os-endpoint "https://opensearch:9200" \
//	  --os-user admin --os-pass admin \
//	  --batch-size 500
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
)

const indexDateFormat = "2006.01.02"

func main() {
	pgDSN := flag.String("pg-dsn", "", "PostgreSQL connection string")
	osEndpoint := flag.String("os-endpoint", "", "OpenSearch endpoint (e.g. https://host:9200)")
	osUser := flag.String("os-user", "admin", "OpenSearch username")
	osPass := flag.String("os-pass", "admin", "OpenSearch password")
	batchSize := flag.Int("batch-size", 500, "rows per bulk request")
	skipPodSnapshot := flag.Bool("skip-pod-snapshot", false, "skip pod_snapshot migration")
	skipWorkloadSnapshot := flag.Bool("skip-workload-snapshot", false, "skip gpu_workload_snapshot migration")
	flag.Parse()

	if *pgDSN == "" || *osEndpoint == "" {
		log.Fatal("--pg-dsn and --os-endpoint are required")
	}

	db, err := sql.Open("postgres", *pgDSN)
	if err != nil {
		log.Fatalf("Failed to connect to PG: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("PG ping failed: %v", err)
	}

	osClient, err := opensearch.NewClient(opensearch.Config{
		Addresses: []string{*osEndpoint},
		Username:  *osUser,
		Password:  *osPass,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create OpenSearch client: %v", err)
	}

	ctx := context.Background()

	if !*skipPodSnapshot {
		migratePodSnapshots(ctx, db, osClient, *batchSize)
	}
	if !*skipWorkloadSnapshot {
		migrateWorkloadSnapshots(ctx, db, osClient, *batchSize)
	}

	log.Println("Migration complete.")
}

func migratePodSnapshots(ctx context.Context, db *sql.DB, osClient *opensearch.Client, batchSize int) {
	log.Println("=== Migrating pod_snapshot ===")
	var totalRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pod_snapshot").Scan(&totalRows); err != nil {
		log.Fatalf("Failed to count pod_snapshot: %v", err)
	}
	log.Printf("Total rows: %d", totalRows)

	lastID := 0
	migrated := 0
	for {
		rows, err := db.QueryContext(ctx,
			`SELECT id, pod_uid, pod_name, namespace, spec, metadata, status, resource_version, created_at
			 FROM pod_snapshot WHERE id > $1 ORDER BY id LIMIT $2`, lastID, batchSize)
		if err != nil {
			log.Fatalf("Query failed at id > %d: %v", lastID, err)
		}

		var items []bulkDoc
		for rows.Next() {
			var (
				id              int
				podUID, podName, namespace string
				spec, metadata, status     []byte
				resourceVersion            int
				createdAt                  time.Time
			)
			if err := rows.Scan(&id, &podUID, &podName, &namespace, &spec, &metadata, &status, &resourceVersion, &createdAt); err != nil {
				rows.Close()
				log.Fatalf("Scan failed: %v", err)
			}
			lastID = id
			items = append(items, bulkDoc{
				IndexPrefix: "pod-snapshot",
				Fields: map[string]interface{}{
					"pod_uid":          podUID,
					"pod_name":         podName,
					"namespace":        namespace,
					"spec":             json.RawMessage(spec),
					"metadata":         json.RawMessage(metadata),
					"status":           json.RawMessage(status),
					"resource_version": resourceVersion,
					"@timestamp":       createdAt.Format(time.RFC3339),
				},
			})
		}
		rows.Close()

		if len(items) == 0 {
			break
		}

		if err := bulkIndex(ctx, osClient, items); err != nil {
			log.Fatalf("Bulk index failed at id %d: %v", lastID, err)
		}
		migrated += len(items)
		log.Printf("  pod_snapshot: %d / %d migrated (last_id=%d)", migrated, totalRows, lastID)
	}
	log.Printf("pod_snapshot migration done: %d rows", migrated)
}

func migrateWorkloadSnapshots(ctx context.Context, db *sql.DB, osClient *opensearch.Client, batchSize int) {
	log.Println("=== Migrating gpu_workload_snapshot ===")
	var totalRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gpu_workload_snapshot").Scan(&totalRows); err != nil {
		log.Fatalf("Failed to count gpu_workload_snapshot: %v", err)
	}
	log.Printf("Total rows: %d", totalRows)

	lastID := 0
	migrated := 0
	for {
		rows, err := db.QueryContext(ctx,
			`SELECT id, uid, group_version, kind, name, namespace, metadata, detail, resource_version, created_at
			 FROM gpu_workload_snapshot WHERE id > $1 ORDER BY id LIMIT $2`, lastID, batchSize)
		if err != nil {
			log.Fatalf("Query failed at id > %d: %v", lastID, err)
		}

		var items []bulkDoc
		for rows.Next() {
			var (
				id                                          int
				uid, gv, kind, name, namespace              string
				metadataBytes, detailBytes                  []byte
				resourceVersion                             int
				createdAt                                   time.Time
			)
			if err := rows.Scan(&id, &uid, &gv, &kind, &name, &namespace, &metadataBytes, &detailBytes, &resourceVersion, &createdAt); err != nil {
				rows.Close()
				log.Fatalf("Scan failed: %v", err)
			}
			lastID = id

			doc := map[string]interface{}{
				"uid":              uid,
				"group_version":    gv,
				"kind":             kind,
				"name":             name,
				"namespace":        namespace,
				"resource_version": resourceVersion,
				"@timestamp":       createdAt.Format(time.RFC3339),
			}
			if len(metadataBytes) > 0 {
				doc["metadata"] = json.RawMessage(metadataBytes)
			}
			if len(detailBytes) > 0 {
				doc["detail"] = json.RawMessage(detailBytes)
			}
			items = append(items, bulkDoc{IndexPrefix: "workload-snapshot", Fields: doc})
		}
		rows.Close()

		if len(items) == 0 {
			break
		}

		if err := bulkIndex(ctx, osClient, items); err != nil {
			log.Fatalf("Bulk index failed at id %d: %v", lastID, err)
		}
		migrated += len(items)
		log.Printf("  gpu_workload_snapshot: %d / %d migrated (last_id=%d)", migrated, totalRows, lastID)
	}
	log.Printf("gpu_workload_snapshot migration done: %d rows", migrated)
}

type bulkDoc struct {
	IndexPrefix string
	Fields      map[string]interface{}
}

func bulkIndex(ctx context.Context, client *opensearch.Client, docs []bulkDoc) error {
	var buf bytes.Buffer
	for _, d := range docs {
		ts, _ := d.Fields["@timestamp"].(string)
		indexName := resolveIndex(d.IndexPrefix, ts)

		meta, _ := json.Marshal(map[string]interface{}{
			"index": map[string]interface{}{"_index": indexName},
		})
		body, _ := json.Marshal(d.Fields)
		buf.Write(meta)
		buf.WriteByte('\n')
		buf.Write(body)
		buf.WriteByte('\n')
	}

	req := opensearchapi.BulkRequest{Body: &buf}
	res, err := req.Do(ctx, client)
	if err != nil {
		return fmt.Errorf("bulk request: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("bulk response error: %s", res.String())
	}
	return nil
}

func resolveIndex(prefix, timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t = time.Now()
	}
	return fmt.Sprintf("%s-%s", prefix, t.Format(indexDateFormat))
}
