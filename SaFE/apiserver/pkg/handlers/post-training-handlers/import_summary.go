/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

const (
	// importSchema and importParserVersions are the summary schema and parser versions this apiserver reads.
	importSchema = "slime-rocm-run/1"
	// importCluster is the cluster column of an imported run.
	importCluster = "spur"
	// importSparkPoints caps the sparkline of an imported run, as the controller's does.
	importSparkPoints = 60
)

var (
	importParserVersions = []string{"1"}
	importStatuses       = []string{"Succeeded", "Failed", "Stopped"}
	importValLoss        = regexp.MustCompile(`^eval/[^/]+/loss$`)
	importRunIDChars     = regexp.MustCompile(`[^a-z0-9-]+`)
)

// importSummary is the part of a slime-rocm-run/1 summary SaFE reads; the rest is kept as it came.
type importSummary struct {
	Schema          string                  `json:"schema"`
	ParserVersion   string                  `json:"parserVersion"`
	Format          string                  `json:"format"`
	Path            string                  `json:"path"`
	Name            string                  `json:"name"`
	SlurmJobID      string                  `json:"slurmJobId"`
	Verdict         string                  `json:"verdict"`
	Status          string                  `json:"status"`
	FailedStage     string                  `json:"failedStage"`
	Message         string                  `json:"message"`
	TrainType       string                  `json:"trainType"`
	Algorithm       string                  `json:"algorithm"`
	Method          string                  `json:"method"`
	StartTime       string                  `json:"startTime"`
	EndTime         string                  `json:"endTime"`
	Nodes           []json.RawMessage       `json:"nodes"`
	Image           string                  `json:"image"`
	GpusPerNode     int                     `json:"gpusPerNode"`
	BaseModel       string                  `json:"baseModel"`
	Dataset         string                  `json:"dataset"`
	SaveSet         bool                    `json:"saveSet"`
	Stages          json.RawMessage         `json:"stages"`
	Gates           []importGate            `json:"gates"`
	Markers         []json.RawMessage       `json:"markers"`
	MainLog         *importMainLog          `json:"mainLog"`
	Arguments       importArguments         `json:"arguments"`
	Resources       map[string]importHost   `json:"resources"`
	Checkpoints     importCheckpoints       `json:"checkpoints"`
	Metrics         map[string]importSeries `json:"metrics"`
	Warnings        []string                `json:"warnings"`
	StepsPerRollout int                     `json:"stepsPerRollout"`
}

// importGate is one gate on one node.
type importGate struct {
	Gate   string `json:"gate"`
	Node   string `json:"node"`
	Result string `json:"result"`
}

// importMainLog describes the main log the metric points came from.
type importMainLog struct {
	Name        string `json:"name"`
	Bytes       int64  `json:"bytes"`
	MetricLines int    `json:"metricLines"`
}

// importArguments are the kept arguments and how many the arguments section had.
type importArguments struct {
	Kept  map[string]json.RawMessage `json:"kept"`
	Total int                        `json:"total"`
}

// importHost holds the resource samples of one node.
type importHost struct {
	Samples [][]json.RawMessage `json:"samples"`
}

// importCheckpoints lists the checkpoints of the run directory.
type importCheckpoints struct {
	Iterations []json.RawMessage `json:"iterations"`
}

// importSeries is one metric series: its axis, its points as [step, value, epoch seconds or null], and how many points
// it had before the command downsampled it.
type importSeries struct {
	Axis        string       `json:"axis"`
	Count       int          `json:"count"`
	Downsampled bool         `json:"downsampled"`
	Points      [][]*float64 `json:"points"`
}

// importPreview is what the import dialog lists of a parsed directory.
type importPreview struct {
	Name          string         `json:"name"`
	Path          string         `json:"path"`
	Verdict       string         `json:"verdict"`
	FailedGate    *importGate    `json:"failedGate,omitempty"`
	Status        string         `json:"status"`
	TrainType     string         `json:"trainType"`
	SlurmJobID    string         `json:"slurmJobId,omitempty"`
	Nodes         int            `json:"nodes"`
	StartTime     string         `json:"startTime,omitempty"`
	MainLog       *importMainLog `json:"mainLog,omitempty"`
	Series        int            `json:"series"`
	Gates         int            `json:"gates"`
	Markers       int            `json:"markers"`
	Arguments     int            `json:"arguments"`
	KeptArguments int            `json:"keptArguments"`
	Samples       int            `json:"samples"`
	SampleNodes   int            `json:"sampleNodes"`
	Checkpoints   int            `json:"checkpoints"`
	SaveSet       bool           `json:"saveSet"`
	Warnings      []string       `json:"warnings,omitempty"`
}

// importInvalid is a summary that cannot be imported, with the field at fault.
type importInvalid struct {
	Field   string
	Message string
}

// Error returns the reason a summary is invalid.
func (e *importInvalid) Error() string {
	return e.Message
}

// parseImportSummary decodes and checks a summary against the schema, the framework formats and the registry's cap on
// metric points.
func parseImportSummary(data []byte, reg *registry.Registry) (*importSummary, error) {
	s := &importSummary{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, &importInvalid{Field: "", Message: "the summary is not valid JSON: " + err.Error()}
	}
	fail := func(field, format string, args ...interface{}) error {
		return &importInvalid{Field: field, Message: fmt.Sprintf(format, args...)}
	}
	switch {
	case s.Schema != importSchema:
		return nil, fail("schema", "unknown summary schema %q; this SaFE reads %s", s.Schema, importSchema)
	case !slices.Contains(importParserVersions, s.ParserVersion):
		return nil, fail("parserVersion", "unknown parser version %q", s.ParserVersion)
	case importFramework(reg, s.Format) == nil:
		return nil, fail("format", "no framework imports the %q format", s.Format)
	case !path.IsAbs(s.Path) || path.Clean(s.Path) != s.Path:
		return nil, fail("path", "the run directory must be a clean absolute path, not %q", s.Path)
	case s.Name == "":
		return nil, fail("name", "the run directory has no name")
	case !slices.Contains(importStatuses, s.Status):
		return nil, fail("status", "unknown status %q", s.Status)
	case s.TrainType != registry.TrainTypeSFT && s.TrainType != registry.TrainTypeRL:
		return nil, fail("trainType", "unknown train type %q", s.TrainType)
	case len(s.Nodes) == 0:
		return nil, fail("nodes", "the run directory has no node logs")
	}
	for _, t := range []struct{ field, value string }{{"startTime", s.StartTime}, {"endTime", s.EndTime}} {
		if _, ok := importTime(t.value); t.value != "" && !ok {
			return nil, fail(t.field, "%s is not an RFC 3339 time: %q", t.field, t.value)
		}
	}
	total := 0
	for name, series := range s.Metrics {
		if series.Axis != "train" && series.Axis != "rollout" {
			return nil, fail("metrics."+name, "unknown axis %q", series.Axis)
		}
		if series.Count < len(series.Points) {
			return nil, fail("metrics."+name, "count %d is below the %d points", series.Count, len(series.Points))
		}
		for _, p := range series.Points {
			if len(p) < 2 || p[0] == nil || p[1] == nil || *p[0] < 0 || *p[0] != math.Trunc(*p[0]) ||
				math.IsNaN(*p[1]) || math.IsInf(*p[1], 0) {
				return nil, fail("metrics."+name, "a point is not [step, value, time]")
			}
		}
		total += len(series.Points)
	}
	if fw := importFramework(reg, s.Format); fw.Metrics.MaxPointsPerRun > 0 && total > fw.Metrics.MaxPointsPerRun {
		return nil, fail("metrics", "%d metric points are more than the %d a run may hold", total,
			fw.Metrics.MaxPointsPerRun)
	}
	return s, nil
}

// importFramework returns the framework that imports a directory format, or nil.
func importFramework(reg *registry.Registry, format string) *registry.Framework {
	for i := range reg.Frameworks {
		if slices.Contains(reg.Frameworks[i].ImportFormats, format) {
			return &reg.Frameworks[i]
		}
	}
	return nil
}

// importTime parses an RFC 3339 time of a summary.
func importTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	return t.UTC(), err == nil
}

// importPathKey is the unique key of a directory in a workspace: the SHA-256 of the workspace and the path.
func importPathKey(workspace, dir string) string {
	sum := sha256.Sum256([]byte(workspace + "\n" + dir))
	return hex.EncodeToString(sum[:])
}

// importRunID is the run id of an imported directory: spur- and the directory name in lowercase letters, digits and
// dashes, with suffix after it when the plain id is taken.
func importRunID(name, suffix string) string {
	id := strings.Trim(importRunIDChars.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(id) > 48 {
		id = id[:48]
	}
	if suffix != "" {
		id += "-" + suffix
	}
	return "spur-" + id
}

// importPoints turns the series the framework records into metric points; a point without a time takes the run's
// start time.
func importPoints(s *importSummary, fw *registry.Framework, start time.Time) []*dbclient.PostTrainingRunMetric {
	var out []*dbclient.PostTrainingRunMetric
	for name, series := range s.Metrics {
		if !importIncluded(fw, name) {
			continue
		}
		for _, p := range series.Points {
			wall := start
			if len(p) > 2 && p[2] != nil {
				wall = time.Unix(int64(*p[2]), 0).UTC()
			}
			out = append(out, &dbclient.PostTrainingRunMetric{Name: name, Step: int(*p[0]), Value: *p[1], WallTime: wall})
		}
	}
	return out
}

// importIncluded tells whether the framework records a metric name.
func importIncluded(fw *registry.Framework, name string) bool {
	for _, pattern := range fw.Metrics.Include {
		if MatchName(pattern, name) {
			return true
		}
	}
	return false
}

// importMetricSummary is the metric summary of an imported run, in the controller's format: the point count, the
// train type's main metric with its last value and sparkline, the best validation loss, and the original point counts
// of the series the command downsampled.
type importMetricSummary struct {
	Points      int               `json:"points"`
	Main        *importMainMetric `json:"main,omitempty"`
	BestValLoss *importStepValue  `json:"bestValLoss,omitempty"`
	Downsampled map[string]int    `json:"downsampled,omitempty"`
}

// importMainMetric is the main metric of a run summary.
type importMainMetric struct {
	Name  string            `json:"name"`
	Step  int               `json:"step"`
	Value float64           `json:"value"`
	Spark []importStepValue `json:"spark,omitempty"`
}

// importStepValue is one (step, value) pair.
type importStepValue struct {
	Step  int     `json:"step"`
	Value float64 `json:"value"`
}

// importSummaryOf summarizes the recorded points of an imported run.
func importSummaryOf(s *importSummary, fw *registry.Framework, points []*dbclient.PostTrainingRunMetric) string {
	out := importMetricSummary{Points: len(points)}
	main := fw.Metrics.Summary[s.TrainType]
	var series []importStepValue
	for _, p := range points {
		if p.Name == main {
			series = append(series, importStepValue{Step: p.Step, Value: p.Value})
		}
		if importValLoss.MatchString(p.Name) && (out.BestValLoss == nil || p.Value < out.BestValLoss.Value) {
			out.BestValLoss = &importStepValue{Step: p.Step, Value: p.Value}
		}
	}
	if len(series) > 0 {
		slices.SortFunc(series, func(a, b importStepValue) int { return a.Step - b.Step })
		last := series[len(series)-1]
		out.Main = &importMainMetric{Name: main, Step: last.Step, Value: last.Value}
		stride := (len(series) + importSparkPoints - 1) / importSparkPoints
		for i := 0; i < len(series); i += stride {
			out.Main.Spark = append(out.Main.Spark, series[i])
		}
	}
	for name, series := range s.Metrics {
		if series.Downsampled && importIncluded(fw, name) {
			if out.Downsampled == nil {
				out.Downsampled = map[string]int{}
			}
			out.Downsampled[name] = series.Count
		}
	}
	data, _ := json.Marshal(out)
	return string(data)
}

// importRun builds the run row of an imported directory: written once in its final state, owned by the token's
// issuer, with no recipe so it never counts as a platform success.
func importRun(s *importSummary, fw *registry.Framework, token *dbclient.PostTrainingImportToken, runID string,
	points []*dbclient.PostTrainingRunMetric) *dbclient.PostTrainingRun {
	run := &dbclient.PostTrainingRun{
		RunID: runID, WorkloadID: runID, DisplayName: s.Name, Framework: fw.Name, Source: dbclient.PostTrainingSourceSpur,
		TrainType: s.TrainType, Method: s.Method, Algorithm: s.Algorithm, Workspace: token.Workspace,
		Cluster: importCluster, UserID: token.UserID, UserName: token.UserName, BaseModelName: s.BaseModel,
		DatasetName: s.Dataset, Image: s.Image, NodeCount: len(s.Nodes), GpuPerNode: s.GpusPerNode,
		OutputPath: s.Path, Status: s.Status, Stage: "done", FailedStage: s.FailedStage, Message: s.Message,
		StageHistory: string(s.Stages), Recipe: "{}", MetricSummary: importSummaryOf(s, fw, points),
	}
	if run.Method == "" {
		run.Method = registry.MethodFull
	}
	if t, ok := importTime(s.StartTime); ok {
		run.StartTime = pq.NullTime{Time: t, Valid: true}
	}
	if t, ok := importTime(s.EndTime); ok {
		run.EndTime = pq.NullTime{Time: t, Valid: true}
	}
	return run
}

// importRecord builds the import record of a summary. An imported record keeps the summary without its metric points
// and resource samples, which the run and the resources column hold; a staged one keeps all of it for its commit.
func importRecord(raw []byte, s *importSummary, token *dbclient.PostTrainingImportToken, importID, runID, via,
	state string) (*dbclient.PostTrainingImport, error) {
	parts := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	resources := parts["resources"]
	if state == dbclient.PostTrainingImportImported {
		delete(parts, "metrics")
		delete(parts, "resources")
	}
	summary, err := json.Marshal(parts)
	if err != nil {
		return nil, err
	}
	warnings, _ := json.Marshal(s.Warnings)
	imp := &dbclient.PostTrainingImport{
		ImportID: importID, RunID: runID, Workspace: token.Workspace, SourcePath: s.Path, Format: s.Format,
		SlurmJobID: s.SlurmJobID, Via: via, State: state, Verdict: s.Verdict, Summary: string(summary),
		Resources: string(resources), Warnings: string(warnings), ParserVersion: s.ParserVersion,
		UserID: token.UserID, UserName: token.UserName,
	}
	if state == dbclient.PostTrainingImportImported {
		imp.PathKey = sql.NullString{String: importPathKey(token.Workspace, s.Path), Valid: true}
	}
	return imp, nil
}

// previewOf lists what the import dialog shows of a summary.
func previewOf(s *importSummary) importPreview {
	p := importPreview{Name: s.Name, Path: s.Path, Verdict: s.Verdict, Status: s.Status, TrainType: s.TrainType,
		SlurmJobID: s.SlurmJobID, Nodes: len(s.Nodes), StartTime: s.StartTime, MainLog: s.MainLog,
		Series: len(s.Metrics), Gates: len(s.Gates), Markers: len(s.Markers), Arguments: s.Arguments.Total,
		KeptArguments: len(s.Arguments.Kept), SampleNodes: len(s.Resources),
		Checkpoints: len(s.Checkpoints.Iterations), SaveSet: s.SaveSet, Warnings: s.Warnings}
	for _, h := range s.Resources {
		p.Samples += len(h.Samples)
	}
	for i := range s.Gates {
		if g := s.Gates[i]; g.Result != "pass" && g.Result != "running" {
			p.FailedGate = &s.Gates[i]
			break
		}
	}
	return p
}
