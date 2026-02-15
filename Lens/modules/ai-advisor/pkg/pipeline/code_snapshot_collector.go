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
	"sort"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/snapshot"
)

const (
	// Maximum single file size to read from container (128KB)
	maxFileSize = 128 * 1024

	// Maximum total payload size for all collected files (2MB)
	maxTotalPayload = 2 * 1024 * 1024

	// Maximum number of files to collect
	maxCollectFiles = 200

	// Maximum number of config files to collect
	maxConfigFiles = 20

	// Maximum number of local module files to collect
	maxLocalModules = 100

	// Maximum depth for recursive directory listing
	maxDirTreeFiles = 5000
)

// sourceCodeExts lists file extensions considered source code
var sourceCodeExts = map[string]bool{
	".py":   true,
	".pyx":  true,
	".pxd":  true,
	".sh":   true,
	".bash": true,
}

// configFileExts lists file extensions considered configuration
var configFileExts = map[string]bool{
	".json": true,
	".yaml": true,
	".yml":  true,
	".toml": true,
	".cfg":  true,
	".ini":  true,
	".conf": true,
}

// skipDirNames lists directory names to skip during recursive scanning
var skipDirNames = map[string]bool{
	// Python / package managers
	"__pycache__":    true,
	".eggs":          true,
	".tox":           true,
	"site-packages":  true,
	"dist-packages":  true,
	".mypy_cache":    true,
	".pytest_cache":  true,
	".venv":          true,
	"venv":           true,
	"env":            true,
	".env":           true,
	"node_modules":   true,
	".npm":           true,
	// Version control
	".git":           true,
	".svn":           true,
	".hg":            true,
	// IDE
	".idea":          true,
	".vscode":        true,
	// Data / model artifacts
	"data":           true,
	"datasets":       true,
	"checkpoints":    true,
	"ckpt":           true,
	"output":         true,
	"outputs":        true,
	"results":        true,
	"runs":           true,
	"wandb":          true,
	"mlruns":         true,
	"tensorboard":    true,
	"tb_logs":        true,
	".cache":         true,
	"cache":          true,
	"logs":           true,
	"log":            true,
	// Build artifacts
	"build":          true,
	"dist":           true,
	"*.egg-info":     true,
}

// skipFilePatterns lists file patterns to skip
var skipFilePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\.pyc$`),
	regexp.MustCompile(`\.pyo$`),
	regexp.MustCompile(`\.so$`),
	regexp.MustCompile(`\.o$`),
	regexp.MustCompile(`\.a$`),
	regexp.MustCompile(`\.egg$`),
	regexp.MustCompile(`\.whl$`),
	regexp.MustCompile(`\.tar\.gz$`),
	regexp.MustCompile(`\.zip$`),
	regexp.MustCompile(`\.bin$`),
	regexp.MustCompile(`\.pt$`),
	regexp.MustCompile(`\.pth$`),
	regexp.MustCompile(`\.ckpt$`),
	regexp.MustCompile(`\.safetensors$`),
	regexp.MustCompile(`\.npy$`),
	regexp.MustCompile(`\.npz$`),
	regexp.MustCompile(`\.h5$`),
	regexp.MustCompile(`\.hdf5$`),
	regexp.MustCompile(`\.parquet$`),
	regexp.MustCompile(`\.arrow$`),
	regexp.MustCompile(`\.csv$`),
	regexp.MustCompile(`\.tsv$`),
	regexp.MustCompile(`\.pkl$`),
	regexp.MustCompile(`\.pickle$`),
	regexp.MustCompile(`\.db$`),
	regexp.MustCompile(`\.sqlite$`),
	regexp.MustCompile(`\.log$`),
}

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
// It scans the working directory recursively to collect all project source files,
// then stores the snapshot.  File contents go to an external Store (S3 or local FS)
// when one is configured; otherwise they fall back to inline JSONB in the database.
type CodeSnapshotCollector struct {
	podProber      *common.PodProber
	cmdParser      *CmdlineParser
	snapshotFacade database.WorkloadCodeSnapshotFacadeInterface
	// snapshotStore is the optional external file store (S3 / local).
	// When nil, file contents are stored inline in the database JSONB columns.
	snapshotStore snapshot.Store
}

// NewCodeSnapshotCollector creates a new collector.
// store may be nil, in which case file contents are stored inline in the DB.
func NewCodeSnapshotCollector(podProber *common.PodProber, store snapshot.Store) *CodeSnapshotCollector {
	return &CodeSnapshotCollector{
		podProber:      podProber,
		cmdParser:      NewCmdlineParser(),
		snapshotFacade: database.NewWorkloadCodeSnapshotFacade(),
		snapshotStore:  store,
	}
}

// Collect gathers code snapshot from a running container by scanning the project directory
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
		entryPath, isModule = c.cmdParser.ParseEntryPoint(pythonProc.Cmdline)
	}

	snapshot := &intent.CodeSnapshotEvidence{}
	var totalSize int

	// Resolve entry script absolute path
	var entryFullPath string
	if entryPath != "" {
		entryFullPath = c.resolvePath(entryPath, cwd, isModule)
	}

	// ---- Phase 1: Scan working directory tree ----
	dirTree, err := c.podProber.ListContainerDirectory(ctx, nodeName, pid, cwd, true, "")
	if err != nil {
		log.Warnf("CodeSnapshotCollector: failed to list working directory %s: %v", cwd, err)
		// Fallback to entry-script-only mode
		return c.collectEntryOnly(ctx, nodeName, pid, cwd, entryFullPath, cmdline, pythonProc.Cmdline, workloadUID)
	}

	// Build directory tree string for storage
	var treeBuilder strings.Builder
	sourceFiles := make([]string, 0, 64)
	configFilePaths := make([]string, 0, 16)

	for _, f := range dirTree.Files {
		if f == nil {
			continue
		}

		// Skip files beyond max tree size to prevent huge listings
		if treeBuilder.Len() > 256*1024 {
			break
		}

		relPath := f.Path
		treeBuilder.WriteString(fmt.Sprintf("%s\t%d\t%s\n", relPath, f.Size, f.Mode))

		// Skip directories and large files
		if f.IsDir {
			continue
		}

		// Check if any path segment is in skip list
		if c.shouldSkipPath(relPath) {
			continue
		}

		// Check if file matches skip patterns
		if c.shouldSkipFile(relPath) {
			continue
		}

		// Classify file
		ext := strings.ToLower(filepath.Ext(relPath))
		if sourceCodeExts[ext] {
			if f.Size <= int64(maxFileSize) {
				sourceFiles = append(sourceFiles, relPath)
			}
		} else if configFileExts[ext] {
			if f.Size <= int64(maxFileSize) {
				configFilePaths = append(configFilePaths, relPath)
			}
		}
	}

	workingDirTree := treeBuilder.String()

	// ---- Phase 2: Prioritize and read files ----

	// Sort source files: entry script first, then by path depth (shallower first)
	sort.Slice(sourceFiles, func(i, j int) bool {
		// Entry script always comes first
		if entryFullPath != "" {
			if sourceFiles[i] == entryFullPath {
				return true
			}
			if sourceFiles[j] == entryFullPath {
				return false
			}
		}
		// Then sort by depth (fewer slashes = higher priority)
		di := strings.Count(sourceFiles[i], "/")
		dj := strings.Count(sourceFiles[j], "/")
		if di != dj {
			return di < dj
		}
		return sourceFiles[i] < sourceFiles[j]
	})

	// Read source files
	for _, path := range sourceFiles {
		if totalSize >= maxTotalPayload {
			log.Infof("CodeSnapshotCollector: total payload limit reached (%d bytes), stopping", totalSize)
			break
		}
		if len(snapshot.LocalModules) >= maxLocalModules && (entryFullPath == "" || path != entryFullPath) {
			break
		}

		content, err := c.readFile(ctx, nodeName, pid, path)
		if err != nil {
			log.Debugf("CodeSnapshotCollector: failed to read %s: %v", path, err)
			continue
		}

		truncated := false
		if len(content) > maxFileSize {
			content = content[:maxFileSize]
			truncated = true
		}

		fc := &intent.FileContent{
			Path:      path,
			Content:   content,
			Size:      len(content),
			Hash:      hashContent(content),
			Truncated: truncated,
		}

		// Assign to appropriate field
		if entryFullPath != "" && path == entryFullPath {
			snapshot.EntryScript = fc
		} else {
			snapshot.LocalModules = append(snapshot.LocalModules, fc)
		}
		totalSize += len(content)
	}

	// If entry script was not found in the scan, try reading it directly
	if snapshot.EntryScript == nil && entryFullPath != "" {
		content, err := c.readFile(ctx, nodeName, pid, entryFullPath)
		if err != nil {
			log.Warnf("CodeSnapshotCollector: failed to read entry script %s: %v", entryFullPath, err)
		} else {
			snapshot.EntryScript = &intent.FileContent{
				Path:    entryFullPath,
				Content: content,
				Size:    len(content),
				Hash:    hashContent(content),
			}
			totalSize += len(content)
		}
	}

	// Read config files from cmdline args AND discovered config files
	cmdConfigPaths := c.cmdParser.ParseConfigPaths(cmdline)
	if len(cmdConfigPaths) == 0 {
		cmdConfigPaths = c.cmdParser.ParseConfigPaths(pythonProc.Cmdline)
	}
	// Merge cmdline config paths with discovered config files
	configSet := make(map[string]bool)
	var allConfigPaths []string
	// Cmdline-referenced configs first (higher priority)
	for _, cfgPath := range cmdConfigPaths {
		fullPath := c.resolvePath(cfgPath, cwd, false)
		if !configSet[fullPath] {
			configSet[fullPath] = true
			allConfigPaths = append(allConfigPaths, fullPath)
		}
	}
	// Then discovered configs
	for _, cfgPath := range configFilePaths {
		if !configSet[cfgPath] {
			configSet[cfgPath] = true
			allConfigPaths = append(allConfigPaths, cfgPath)
		}
	}

	for i, cfgPath := range allConfigPaths {
		if i >= maxConfigFiles || totalSize >= maxTotalPayload {
			break
		}
		content, err := c.readFile(ctx, nodeName, pid, cfgPath)
		if err != nil {
			log.Debugf("CodeSnapshotCollector: failed to read config %s: %v", cfgPath, err)
			continue
		}
		truncated := false
		if len(content) > maxFileSize {
			content = content[:maxFileSize]
			truncated = true
		}
		snapshot.ConfigFiles = append(snapshot.ConfigFiles, &intent.FileContent{
			Path:      cfgPath,
			Content:   content,
			Size:      len(content),
			Hash:      hashContent(content),
			Truncated: truncated,
		})
		totalSize += len(content)
	}

	// Read pip freeze output
	pipFreeze, err := c.readPipFreeze(ctx, nodeName, pid)
	if err == nil && pipFreeze != "" {
		snapshot.PipFreeze = pipFreeze
	}

	// Compute fingerprint
	snapshot.Fingerprint = c.computeFingerprint(snapshot)

	log.Infof("CodeSnapshotCollector: collected %d source files, %d config files, total %d bytes for workload %s",
		len(snapshot.LocalModules)+boolToInt(snapshot.EntryScript != nil),
		len(snapshot.ConfigFiles),
		totalSize,
		workloadUID)

	// Store in database
	if err := c.storeSnapshot(ctx, workloadUID, snapshot, workingDirTree, totalSize); err != nil {
		log.Warnf("CodeSnapshotCollector: failed to store snapshot: %v", err)
	}

	return snapshot, nil
}

// collectEntryOnly is the fallback when directory listing fails.
// It only reads the entry script and cmdline-referenced config files.
func (c *CodeSnapshotCollector) collectEntryOnly(
	ctx context.Context,
	nodeName string,
	pid int,
	cwd string,
	entryFullPath string,
	cmdline string,
	procCmdline string,
	workloadUID string,
) (*intent.CodeSnapshotEvidence, error) {
	snapshot := &intent.CodeSnapshotEvidence{}
	var totalSize int

	if entryFullPath != "" {
		content, err := c.readFile(ctx, nodeName, pid, entryFullPath)
		if err != nil {
			log.Warnf("CodeSnapshotCollector: failed to read entry script %s: %v", entryFullPath, err)
		} else {
			snapshot.EntryScript = &intent.FileContent{
				Path:    entryFullPath,
				Content: content,
				Size:    len(content),
				Hash:    hashContent(content),
			}
			totalSize += len(content)
		}
	}

	// Read config files referenced in cmdline
	configPaths := c.cmdParser.ParseConfigPaths(cmdline)
	if len(configPaths) == 0 {
		configPaths = c.cmdParser.ParseConfigPaths(procCmdline)
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
		totalSize += len(content)
	}

	// Read pip freeze output
	pipFreeze, err := c.readPipFreeze(ctx, nodeName, pid)
	if err == nil && pipFreeze != "" {
		snapshot.PipFreeze = pipFreeze
	}

	snapshot.Fingerprint = c.computeFingerprint(snapshot)

	if err := c.storeSnapshot(ctx, workloadUID, snapshot, "", totalSize); err != nil {
		log.Warnf("CodeSnapshotCollector: failed to store snapshot: %v", err)
	}

	return snapshot, nil
}

// shouldSkipPath checks if any path segment matches the skip list
func (c *CodeSnapshotCollector) shouldSkipPath(path string) bool {
	segments := strings.Split(path, "/")
	for _, seg := range segments {
		if skipDirNames[seg] {
			return true
		}
	}
	return false
}

// shouldSkipFile checks if a file matches skip patterns
func (c *CodeSnapshotCollector) shouldSkipFile(path string) bool {
	for _, re := range skipFilePatterns {
		if re.MatchString(path) {
			return true
		}
	}
	return false
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

	for _, mod := range snapshot.LocalModules {
		h.Write([]byte(mod.Path))
		h.Write([]byte(mod.Content))
	}

	for _, cfg := range snapshot.ConfigFiles {
		h.Write([]byte(cfg.Content))
	}

	if snapshot.PipFreeze != "" {
		h.Write([]byte(snapshot.PipFreeze))
	}

	return hex.EncodeToString(h.Sum(nil))[:16]
}

// storeSnapshot persists the code snapshot.
// When an external store (S3/local) is configured, file contents are uploaded there
// and only metadata (path, hash, size) is saved in the DB JSONB columns.
// When no external store is configured, the full content is inlined in the DB.
func (c *CodeSnapshotCollector) storeSnapshot(
	ctx context.Context,
	workloadUID string,
	snap *intent.CodeSnapshotEvidence,
	workingDirTree string,
	totalSize int,
) error {
	now := time.Now()
	record := &model.WorkloadCodeSnapshot{
		WorkloadUID:    workloadUID,
		Fingerprint:    snap.Fingerprint,
		PipFreeze:      snap.PipFreeze,
		WorkingDirTree: workingDirTree,
		TotalSize:      int32(totalSize),
		CapturedAt:     &now,
		CreatedAt:      now,
	}

	useExternalStore := c.snapshotStore != nil

	// --- Upload to external store if configured ---
	if useExternalStore {
		storageKey := snapshot.StorageKeyFor(workloadUID, snap.Fingerprint)
		storeType := string(c.snapshotStore.Type())
		record.StorageKey = &storageKey
		record.StorageType = &storeType

		var files []snapshot.FileEntry

		if snap.EntryScript != nil && snap.EntryScript.Content != "" {
			files = append(files, snapshot.FileEntry{
				RelPath: "entry/" + filepath.Base(snap.EntryScript.Path),
				Content: []byte(snap.EntryScript.Content),
			})
		}
		for i, mod := range snap.LocalModules {
			if mod.Content == "" {
				continue
			}
			relPath := fmt.Sprintf("modules/%04d_%s", i, filepath.Base(mod.Path))
			files = append(files, snapshot.FileEntry{
				RelPath: relPath,
				Content: []byte(mod.Content),
			})
		}
		for i, cfg := range snap.ConfigFiles {
			if cfg.Content == "" {
				continue
			}
			relPath := fmt.Sprintf("config/%04d_%s", i, filepath.Base(cfg.Path))
			files = append(files, snapshot.FileEntry{
				RelPath: relPath,
				Content: []byte(cfg.Content),
			})
		}
		if snap.PipFreeze != "" {
			files = append(files, snapshot.FileEntry{
				RelPath: "meta/pip_freeze.txt",
				Content: []byte(snap.PipFreeze),
			})
		}

		if len(files) > 0 {
			if err := c.snapshotStore.Save(ctx, storageKey, files); err != nil {
				log.Warnf("CodeSnapshotCollector: failed to save to external store, falling back to inline: %v", err)
				// Fall back to inline storage
				useExternalStore = false
				record.StorageKey = nil
				record.StorageType = nil
			} else {
				log.Infof("CodeSnapshotCollector: saved %d files to %s store key=%s",
					len(files), storeType, storageKey)
			}
		}
	}

	// --- Build DB record JSONB columns ---
	if snap.EntryScript != nil {
		entry := model.ExtType{
			"path": snap.EntryScript.Path,
			"hash": snap.EntryScript.Hash,
			"size": snap.EntryScript.Size,
		}
		// Include content only when NOT using external store
		if !useExternalStore {
			entry["content"] = snap.EntryScript.Content
		}
		record.EntryScript = entry
	}

	if len(snap.ConfigFiles) > 0 {
		configData := make([]map[string]interface{}, 0, len(snap.ConfigFiles))
		for _, cf := range snap.ConfigFiles {
			item := map[string]interface{}{
				"path":      cf.Path,
				"hash":      cf.Hash,
				"size":      cf.Size,
				"truncated": cf.Truncated,
			}
			if !useExternalStore {
				item["content"] = cf.Content
			}
			configData = append(configData, item)
		}
		configJSON, _ := json.Marshal(configData)
		record.ConfigFiles = model.ExtJSON(configJSON)
	}

	if len(snap.LocalModules) > 0 {
		modData := make([]map[string]interface{}, 0, len(snap.LocalModules))
		for _, mod := range snap.LocalModules {
			item := map[string]interface{}{
				"path":      mod.Path,
				"hash":      mod.Hash,
				"size":      mod.Size,
				"truncated": mod.Truncated,
			}
			if !useExternalStore {
				item["content"] = mod.Content
			}
			modData = append(modData, item)
		}
		modJSON, _ := json.Marshal(modData)
		record.LocalModules = model.ExtJSON(modJSON)
	}

	if snap.ImportGraph != nil {
		graphMap := make(model.ExtType, len(snap.ImportGraph))
		for k, v := range snap.ImportGraph {
			graphMap[k] = v
		}
		record.ImportGraph = graphMap
	}

	// Count total files
	fileCount := boolToInt(snap.EntryScript != nil) + len(snap.LocalModules) + len(snap.ConfigFiles)
	record.FileCount = int32(fileCount)

	return c.snapshotFacade.Create(ctx, record)
}

// toEvidence converts a DB model to CodeSnapshotEvidence.
// When a record has an external storage key, file contents are loaded from the store.
func (c *CodeSnapshotCollector) toEvidence(record *model.WorkloadCodeSnapshot) *intent.CodeSnapshotEvidence {
	evidence := &intent.CodeSnapshotEvidence{
		PipFreeze:   record.PipFreeze,
		Fingerprint: record.Fingerprint,
	}

	// If content lives in an external store, load it
	if record.StorageKey != nil && *record.StorageKey != "" && c.snapshotStore != nil {
		return c.loadEvidenceFromStore(record, evidence)
	}

	// Legacy path: read content from inline JSONB
	return c.loadEvidenceInline(record, evidence)
}

// loadEvidenceFromStore loads file contents from S3/local store and merges with DB metadata.
func (c *CodeSnapshotCollector) loadEvidenceFromStore(
	record *model.WorkloadCodeSnapshot,
	evidence *intent.CodeSnapshotEvidence,
) *intent.CodeSnapshotEvidence {
	ctx := context.Background()
	storageKey := *record.StorageKey

	files, err := c.snapshotStore.Load(ctx, storageKey)
	if err != nil {
		log.Warnf("CodeSnapshotCollector: failed to load from store key=%s, falling back to inline: %v", storageKey, err)
		return c.loadEvidenceInline(record, evidence)
	}

	// Index loaded files by relPath prefix
	fileIndex := make(map[string]string, len(files))
	for _, f := range files {
		fileIndex[f.RelPath] = string(f.Content)
	}

	// Populate entry script
	if record.EntryScript != nil {
		fc := &intent.FileContent{}
		if p, ok := record.EntryScript["path"].(string); ok {
			fc.Path = p
		}
		if h, ok := record.EntryScript["hash"].(string); ok {
			fc.Hash = h
		}
		if s, ok := record.EntryScript["size"].(float64); ok {
			fc.Size = int(s)
		}
		// Find content from store by "entry/" prefix
		for relPath, content := range fileIndex {
			if strings.HasPrefix(relPath, "entry/") {
				fc.Content = content
				break
			}
		}
		evidence.EntryScript = fc
	}

	// Populate local modules
	if len(record.LocalModules) > 0 {
		var modules []map[string]interface{}
		if err := json.Unmarshal(record.LocalModules, &modules); err == nil {
			for i, m := range modules {
				fc := &intent.FileContent{}
				if p, ok := m["path"].(string); ok {
					fc.Path = p
				}
				if h, ok := m["hash"].(string); ok {
					fc.Hash = h
				}
				if s, ok := m["size"].(float64); ok {
					fc.Size = int(s)
				}
				// Look up content by index-based key
				relKey := fmt.Sprintf("modules/%04d_%s", i, filepath.Base(fc.Path))
				if content, found := fileIndex[relKey]; found {
					fc.Content = content
				}
				evidence.LocalModules = append(evidence.LocalModules, fc)
			}
		}
	}

	// Populate config files
	if len(record.ConfigFiles) > 0 {
		var configs []map[string]interface{}
		if err := json.Unmarshal(record.ConfigFiles, &configs); err == nil {
			for i, m := range configs {
				fc := &intent.FileContent{}
				if p, ok := m["path"].(string); ok {
					fc.Path = p
				}
				if h, ok := m["hash"].(string); ok {
					fc.Hash = h
				}
				if s, ok := m["size"].(float64); ok {
					fc.Size = int(s)
				}
				relKey := fmt.Sprintf("config/%04d_%s", i, filepath.Base(fc.Path))
				if content, found := fileIndex[relKey]; found {
					fc.Content = content
				}
				evidence.ConfigFiles = append(evidence.ConfigFiles, fc)
			}
		}
	}

	// Load pip freeze from store if not in DB
	if evidence.PipFreeze == "" {
		if content, found := fileIndex["meta/pip_freeze.txt"]; found {
			evidence.PipFreeze = content
		}
	}

	return evidence
}

// loadEvidenceInline loads file contents from inline JSONB columns (legacy / fallback).
func (c *CodeSnapshotCollector) loadEvidenceInline(
	record *model.WorkloadCodeSnapshot,
	evidence *intent.CodeSnapshotEvidence,
) *intent.CodeSnapshotEvidence {
	if record.EntryScript != nil {
		evidence.EntryScript = &intent.FileContent{}
		if path, ok := record.EntryScript["path"].(string); ok {
			evidence.EntryScript.Path = path
		}
		if content, ok := record.EntryScript["content"].(string); ok {
			evidence.EntryScript.Content = content
		}
		if hash, ok := record.EntryScript["hash"].(string); ok {
			evidence.EntryScript.Hash = hash
		}
	}

	// Parse LocalModules from ExtJSON
	if len(record.LocalModules) > 0 {
		var modules []map[string]interface{}
		if err := json.Unmarshal(record.LocalModules, &modules); err == nil {
			for _, m := range modules {
				fc := &intent.FileContent{}
				if path, ok := m["path"].(string); ok {
					fc.Path = path
				}
				if content, ok := m["content"].(string); ok {
					fc.Content = content
				}
				if hash, ok := m["hash"].(string); ok {
					fc.Hash = hash
				}
				if size, ok := m["size"].(float64); ok {
					fc.Size = int(size)
				}
				evidence.LocalModules = append(evidence.LocalModules, fc)
			}
		}
	}

	// Parse ConfigFiles from ExtJSON
	if len(record.ConfigFiles) > 0 {
		var configs []map[string]interface{}
		if err := json.Unmarshal(record.ConfigFiles, &configs); err == nil {
			for _, m := range configs {
				fc := &intent.FileContent{}
				if path, ok := m["path"].(string); ok {
					fc.Path = path
				}
				if content, ok := m["content"].(string); ok {
					fc.Content = content
				}
				if hash, ok := m["hash"].(string); ok {
					fc.Hash = hash
				}
				evidence.ConfigFiles = append(evidence.ConfigFiles, fc)
			}
		}
	}

	return evidence
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16]
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
