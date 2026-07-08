/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package enricher

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Scraper fetches per-GPU metrics from every AMD device-metrics-exporter
// endpoint and folds them into per-GPU records.
type Scraper struct {
	c          client.Client
	cfg        Config
	httpClient *http.Client
	// relevant is the set of AMD source metric names we actually consume.
	relevant map[string]struct{}
}

func NewScraper(c client.Client, cfg Config) *Scraper {
	relevant := map[string]struct{}{}
	for _, om := range outputMetrics {
		for _, s := range om.Sources {
			relevant[s] = struct{}{}
		}
		for _, s := range om.Numerator {
			relevant[s] = struct{}{}
		}
		for _, s := range om.Denominator {
			relevant[s] = struct{}{}
		}
	}
	return &Scraper{
		c:          c,
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
		relevant:   relevant,
	}
}

// gpuRecord aggregates all consumed metric values for one physical GPU on one
// node, along with the identifying labels needed to resolve/emit series.
type gpuRecord struct {
	// labels holds representative identity labels (hostname, gpu_id, pod,
	// namespace, container, serial_number).
	labels map[string]string
	// values maps AMD metric name -> value for this GPU.
	values map[string]float64
}

// identity labels copied from source samples onto emitted series / used for
// keying. Includes both the AMD exporter's names (hostname, serial_number) and
// robust gpu-exporter's names (node, address) so either source works.
var identityLabels = []string{"hostname", "node", "gpu_id", "pod", "namespace", "container", "serial_number", "address"}

// recNode returns the node identity from either exporter's label set
// (robust gpu-exporter uses "node", AMD device-metrics-exporter uses "hostname").
func recNode(labels map[string]string) string {
	if v := labels["node"]; v != "" {
		return v
	}
	return labels["hostname"]
}

// Targets returns the list of exporter endpoint base URLs to scrape, resolved
// from the exporter Service's Endpoints so every node's pod is covered.
func (s *Scraper) Targets(ctx context.Context) ([]string, error) {
	ep := &corev1.Endpoints{}
	key := types.NamespacedName{Namespace: s.cfg.ExporterNamespace, Name: s.cfg.ExporterServiceName}
	if err := s.c.Get(ctx, key, ep); err != nil {
		return nil, fmt.Errorf("get exporter endpoints %s: %w", key, err)
	}
	var targets []string
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			targets = append(targets, fmt.Sprintf("%s://%s:%d%s",
				s.cfg.ExporterScheme, addr.IP, s.cfg.ExporterPort, s.cfg.ExporterPath))
		}
	}
	return targets, nil
}

// ScrapeAll scrapes every target and returns the merged per-GPU records keyed
// by node+gpu identity.
func (s *Scraper) ScrapeAll(ctx context.Context, targets []string) map[string]*gpuRecord {
	records := map[string]*gpuRecord{}
	for _, t := range targets {
		if err := s.scrapeInto(ctx, t, records); err != nil {
			// One bad endpoint shouldn't drop the whole pass.
			continue
		}
	}
	return records
}

func (s *Scraper) scrapeInto(ctx context.Context, target string, records map[string]*gpuRecord) error {
	reqCtx, cancel := context.WithTimeout(ctx, s.cfg.HTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("exporter %s returned %d", target, resp.StatusCode)
	}

	return s.parseInto(resp.Body, records)
}

// parseInto reads Prometheus text-exposition lines and folds the metrics we
// care about into per-GPU records. We hand-parse instead of using
// prometheus/common's TextParser because recent versions require a per-parser
// name-validation scheme and panic on the zero value.
func (s *Scraper) parseInto(r io.Reader, records map[string]*gpuRecord) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		name, labels, value, ok := parseMetricLine(line)
		if !ok {
			continue
		}
		if _, want := s.relevant[name]; !want {
			continue
		}
		key := recordKey(labels)
		rec := records[key]
		if rec == nil {
			rec = &gpuRecord{labels: map[string]string{}, values: map[string]float64{}}
			records[key] = rec
		}
		// Merge identity labels, preferring non-empty values (pod/namespace
		// are empty on idle GPUs and populated once a workload owns them).
		for _, l := range identityLabels {
			if v := labels[l]; v != "" {
				rec.labels[l] = v
			}
		}
		rec.values[name] = value
	}
	return scanner.Err()
}

// parseMetricLine parses a single Prometheus text line:
//
//	metric_name{k="v",...} 12.3 [timestamp]
//	metric_name 12.3
func parseMetricLine(line string) (name string, labels map[string]string, value float64, ok bool) {
	labels = map[string]string{}
	brace := strings.IndexByte(line, '{')
	space := strings.IndexByte(line, ' ')

	var rest string
	if brace >= 0 && (space < 0 || brace < space) {
		close := strings.LastIndexByte(line, '}')
		if close < brace {
			return "", nil, 0, false
		}
		name = line[:brace]
		parseLabelSet(line[brace+1:close], labels)
		rest = strings.TrimSpace(line[close+1:])
	} else if space >= 0 {
		name = line[:space]
		rest = strings.TrimSpace(line[space+1:])
	} else {
		return "", nil, 0, false
	}

	// value is the first whitespace-delimited field of the remainder.
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", nil, 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", nil, 0, false // NaN/Inf/non-numeric -> skip
	}
	return name, labels, v, true
}

// parseLabelSet parses `k1="v1",k2="v2"` into the provided map, tolerating
// escaped quotes/backslashes in values.
func parseLabelSet(s string, out map[string]string) {
	i := 0
	n := len(s)
	for i < n {
		// key
		eq := strings.IndexByte(s[i:], '=')
		if eq < 0 {
			return
		}
		key := strings.TrimSpace(s[i : i+eq])
		i += eq + 1
		if i >= n || s[i] != '"' {
			return
		}
		i++ // opening quote
		var val bytes.Buffer
		for i < n {
			c := s[i]
			if c == '\\' && i+1 < n {
				next := s[i+1]
				switch next {
				case '"':
					val.WriteByte('"')
				case '\\':
					val.WriteByte('\\')
				case 'n':
					val.WriteByte('\n')
				default:
					val.WriteByte(next)
				}
				i += 2
				continue
			}
			if c == '"' {
				i++ // closing quote
				break
			}
			val.WriteByte(c)
			i++
		}
		if key != "" {
			out[key] = val.String()
		}
		// skip separator comma/spaces
		for i < n && (s[i] == ',' || s[i] == ' ') {
			i++
		}
	}
}

func recordKey(labels map[string]string) string {
	// serial_number (AMD) or address/PCIe-BDF (robust gpu-exporter) disambiguate
	// GPUs on a node.
	id := labels["serial_number"]
	if id == "" {
		id = labels["address"]
	}
	return recNode(labels) + "|" + labels["gpu_id"] + "|" + id
}
