// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// Maximum file size to read from container (128KB)
	maxFileSize = 128 * 1024

	// Maximum number of config files to collect
	maxConfigFiles = 10

	// Maximum number of local module files to collect
	maxLocalModules = 20

	// Common config file patterns
	configFileGlob = `*.json|*.yaml|*.yml|*.toml|*.cfg|*.ini`
)

// CmdlineParser extracts the entry script path from a Python command line.
// It handles various invocation patterns:
//   - python script.py args...
//   - python -m module args...
//   - torchrun script.py args...
//   - deepspeed script.py args...
//   - accelerate launch script.py args...
type CmdlineParser struct {
	moduleRe *regexp.Regexp
	scriptRe *regexp.Regexp
}

// NewCmdlineParser creates a parser
func NewCmdlineParser() *CmdlineParser {
	return &CmdlineParser{
		moduleRe: regexp.MustCompile(`(?:python[23]?|python3?\.\d+)\s+(?:-\w\s+)*-m\s+(\S+)`),
		scriptRe: regexp.MustCompile(`(?:python[23]?|python3?\.\d+|torchrun|deepspeed|accelerate\s+launch)\s+(?:[^\s]*\s+)*?(\S+\.py)\b`),
	}
}

// ParseEntryPoint extracts the entry script path from a command line
func (p *CmdlineParser) ParseEntryPoint(cmdline string) (scriptPath string, isModule bool) {
	cmdline = strings.TrimSpace(cmdline)
	if cmdline == "" {
		return "", false
	}

	// Check for -m module invocation first
	if matches := p.moduleRe.FindStringSubmatch(cmdline); len(matches) > 1 {
		modulePath := strings.ReplaceAll(matches[1], ".", "/")
		return modulePath, true
	}

	// Check for script path
	if matches := p.scriptRe.FindStringSubmatch(cmdline); len(matches) > 1 {
		return matches[1], false
	}

	// Fallback: look for any .py file in the command line
	tokens := strings.Fields(cmdline)
	for _, token := range tokens {
		if strings.HasSuffix(token, ".py") && !strings.HasPrefix(token, "-") {
			return token, false
		}
	}

	return "", false
}

// ParseConfigPaths extracts configuration file paths from command arguments
func (p *CmdlineParser) ParseConfigPaths(cmdline string) []string {
	var paths []string

	configArgPatterns := []string{
		`--config[_-]?(?:file|path)?\s+(\S+)`,
		`--ds[_-]config\s+(\S+)`,
		`--deepspeed[_-]config\s+(\S+)`,
		`--training[_-]args\s+(\S+)`,
		`--model[_-]config\s+(\S+)`,
	}

	for _, pattern := range configArgPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(cmdline, -1)
		for _, m := range matches {
			if len(m) > 1 {
				paths = append(paths, m[1])
			}
		}
	}

	return paths
}

// CodeSnapshotCollector collects code snapshots from running containers.
// It uses PodProber to read files via node-exporter, then stores the
// snapshot in the workload_code_snapshot table.
type CodeSnapshotCollector struct {
	podProber      *common.PodProber
	cmdParser      *CmdlineParser
	snapshotFacade database.WorkloadCodeSnapshotFacadeInterface
}

// NewCodeSnapshotCollector creates a new collector
func NewCodeSnapshotCollector(podProber *common.PodProber) *CodeSnapshotCollector {
	return &CodeSnapshotCollector{
		podProber:      podProber,
		cmdParser:      NewCmdlineParser(),
		snapshotFacade: database.NewWorkloadCodeSnapshotFacade(),
	}
}

// Collect gathers code snapshot from a running container
func (c *CodeSnapshotCollector) Collect(
	ctx context.Context,
	workloadUID string,
	cmdline string,
) (*intent.CodeSnapshotEvidence, error) {
	// Check if snapshot already exists and is fresh
	existing, err := c.snapshotFacade.GetByWorkloadUID(ctx, workloadUID)
	if err == nil && existing != nil && existing.Fingerprint != "" {
		log.Debugf("CodeSnapshotCollector: snapshot exists for workload %s (fingerprint=%s)", workloadUID, existing.Fingerprint)
		return c.toEvidence(existing), nil
	}

	// Select target pod
	pod, err := c.podProber.SelectTargetPod(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to select target pod: %w", err)
	}

	if !c.podProber.IsPodReady(ctx, pod) {
		return nil, fmt.Errorf("pod %s/%s is not ready", pod.Namespace, pod.Name)
	}

	// Get PID of the main process
	tree, err := c.podProber.GetProcessTree(ctx, pod, common.DefaultProcessTreeOptions())
	if err != nil {
		return nil, fmt.Errorf("failed to get process tree: %w", err)
	}

	pythonProc := c.podProber.FindPythonProcess(tree)
	if pythonProc == nil {
		return nil, fmt.Errorf("no python process found for workload %s", workloadUID)
	}

	pid := pythonProc.HostPID
	nodeName := pod.NodeName
	cwd := pythonProc.Cwd
	if cwd == "" {
		cwd = "/workspace"
	}

	// Parse entry point from cmdline
	entryPath, isModule := c.cmdParser.ParseEntryPoint(cmdline)
	if entryPath == "" {
		// Try from process cmdline
		entryPath, isModule = c.cmdParser.ParseEntryPoint(pythonProc.Cmdline)
	}

	snapshot := &intent.CodeSnapshotEvidence{}

	// Read entry script
	if entryPath != "" {
		fullPath := c.resolvePath(entryPath, cwd, isModule)
		content, err := c.readFile(ctx, nodeName, pid, fullPath)
		if err != nil {
			log.Warnf("CodeSnapshotCollector: failed to read entry script %s: %v", fullPath, err)
		} else {
			snapshot.EntryScript = &intent.FileContent{
				Path:    fullPath,
				Content: content,
				Size:    len(content),
				Hash:    hashContent(content),
			}
		}
	}

	// Read config files referenced in cmdline
	configPaths := c.cmdParser.ParseConfigPaths(cmdline)
	if len(configPaths) == 0 {
		configPaths = c.cmdParser.ParseConfigPaths(pythonProc.Cmdline)
	}

	for i, cfgPath := range configPaths {
		if i >= maxConfigFiles {
			break
		}
		fullPath := c.resolvePath(cfgPath, cwd, false)
		content, err := c.readFile(ctx, nodeName, pid, fullPath)
		if err != nil {
			log.Debugf("CodeSnapshotCollector: failed to read config %s: %v", fullPath, err)
			continue
		}
		snapshot.ConfigFiles = append(snapshot.ConfigFiles, &intent.FileContent{
			Path:    fullPath,
			Content: content,
			Size:    len(content),
			Hash:    hashContent(content),
		})
	}

	// Read pip freeze output
	pipFreeze, err := c.readPipFreeze(ctx, nodeName, pid)
	if err == nil && pipFreeze != "" {
		snapshot.PipFreeze = pipFreeze
	}

	// Compute fingerprint
	snapshot.Fingerprint = c.computeFingerprint(snapshot)

	// Store in database
	if err := c.storeSnapshot(ctx, workloadUID, snapshot); err != nil {
		log.Warnf("CodeSnapshotCollector: failed to store snapshot: %v", err)
	}

	return snapshot, nil
}

// resolvePath resolves a relative path against a working directory
func (c *CodeSnapshotCollector) resolvePath(path, cwd string, isModule bool) string {
	if isModule {
		// Module path: try common locations
		return filepath.Join(cwd, path+".py")
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

// readFile reads a file from the container via node-exporter
func (c *CodeSnapshotCollector) readFile(ctx context.Context, nodeName string, pid int, path string) (string, error) {
	content, err := c.podProber.ReadContainerFile(ctx, nodeName, pid, path)
	if err != nil {
		return "", err
	}

	// Truncate if too large
	if len(content) > maxFileSize {
		content = content[:maxFileSize]
	}

	return content, nil
}

// readPipFreeze reads pip freeze output from the container
func (c *CodeSnapshotCollector) readPipFreeze(ctx context.Context, nodeName string, pid int) (string, error) {
	// Try reading from the pip cache file first
	for _, path := range []string{
		"/tmp/pip-freeze.txt",
		"/root/.pip/pip-freeze.txt",
	} {
		content, err := c.podProber.ReadContainerFile(ctx, nodeName, pid, path)
		if err == nil && content != "" {
			return content, nil
		}
	}

	// pip freeze not available in file - would need exec which we don't have
	return "", fmt.Errorf("pip freeze not available")
}

// computeFingerprint generates a fingerprint for deduplication
func (c *CodeSnapshotCollector) computeFingerprint(snapshot *intent.CodeSnapshotEvidence) string {
	h := sha256.New()

	if snapshot.EntryScript != nil {
		h.Write([]byte(snapshot.EntryScript.Content))
	}

	for _, cfg := range snapshot.ConfigFiles {
		h.Write([]byte(cfg.Content))
	}

	if snapshot.PipFreeze != "" {
		h.Write([]byte(snapshot.PipFreeze))
	}

	return hex.EncodeToString(h.Sum(nil))[:16]
}

// storeSnapshot persists the code snapshot to the database
func (c *CodeSnapshotCollector) storeSnapshot(
	ctx context.Context,
	workloadUID string,
	snapshot *intent.CodeSnapshotEvidence,
) error {
	record := &model.WorkloadCodeSnapshot{
		WorkloadUID: workloadUID,
		Fingerprint: snapshot.Fingerprint,
		PipFreeze:   snapshot.PipFreeze,
		CreatedAt:   time.Now(),
	}

	if snapshot.EntryScript != nil {
		record.EntryScript = snapshot.EntryScript.Content
	}

	// Serialize config files and import graph as JSON
	if len(snapshot.ConfigFiles) > 0 {
		configData := make([]map[string]string, 0, len(snapshot.ConfigFiles))
		for _, cf := range snapshot.ConfigFiles {
			configData = append(configData, map[string]string{
				"path":    cf.Path,
				"content": cf.Content,
				"hash":    cf.Hash,
			})
		}
		// Store as JSON string (ConfigFiles is a text field in the DB)
		configJSON := marshalJSON(configData)
		record.ConfigFiles = configJSON
	}

	if snapshot.ImportGraph != nil {
		graphJSON := marshalJSON(snapshot.ImportGraph)
		record.ImportGraph = graphJSON
	}

	// Upsert: if fingerprint exists, skip; if workload_uid exists, update
	return c.snapshotFacade.Create(ctx, record)
}

// toEvidence converts a DB model to CodeSnapshotEvidence
func (c *CodeSnapshotCollector) toEvidence(record *model.WorkloadCodeSnapshot) *intent.CodeSnapshotEvidence {
	evidence := &intent.CodeSnapshotEvidence{
		PipFreeze:   record.PipFreeze,
		Fingerprint: record.Fingerprint,
	}

	if record.EntryScript != "" {
		evidence.EntryScript = &intent.FileContent{
			Content: record.EntryScript,
		}
	}

	return evidence
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16]
}

func marshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}
