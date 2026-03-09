// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	maxSmallFileSize = 512 * 1024 // 512 KB - only parse files smaller than this
)

// OCILayerResult holds the analysis output from scanning a single OCI layer.
type OCILayerResult struct {
	FileCount      int32                  `json:"file_count"`
	Packages       []OCIPackageInfo       `json:"packages"`
	FrameworkHints map[string]interface{} `json:"framework_hints"`
	NotablePaths   []string               `json:"notable_paths"`
}

// OCIPackageInfo describes an installed package discovered inside a layer.
type OCIPackageInfo struct {
	Manager string `json:"manager"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// OCILayerScanner streams through tar archives extracted from OCI layer blobs
// and identifies installed packages, notable paths, and framework hints.
type OCILayerScanner struct {
	pipMetadataRe *regexp.Regexp
	aptStatusRe   *regexp.Regexp
	condaMetaRe   *regexp.Regexp
}

// NewOCILayerScanner creates a new scanner with precompiled patterns.
func NewOCILayerScanner() *OCILayerScanner {
	return &OCILayerScanner{
		pipMetadataRe: regexp.MustCompile(`^.*/site-packages/([^/]+)\.dist-info/METADATA$`),
		aptStatusRe:   regexp.MustCompile(`^.*/var/lib/dpkg/status$`),
		condaMetaRe:   regexp.MustCompile(`^.*/conda-meta/([^/]+)\.json$`),
	}
}

// ScanLayer reads a compressed (gzip) or plain tar stream and extracts package
// information, notable paths, and file counts.
func (s *OCILayerScanner) ScanLayer(reader io.Reader) (*OCILayerResult, error) {
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		log.Debugf("OCILayerScanner: gzip decompress failed, trying plain tar: %v", err)
		// Fall back to plain tar if gzip fails. The caller must provide a
		// seekable reader or a fresh stream for the retry to work. In practice
		// the caller wraps the body in a buffered reader so we can't seek -
		// we return the error and let the caller retry with a plain reader.
		return nil, fmt.Errorf("gzip decompression failed: %w", err)
	}
	defer gzReader.Close()

	return s.scanTar(gzReader)
}

// ScanLayerPlain reads a plain (uncompressed) tar stream.
func (s *OCILayerScanner) ScanLayerPlain(reader io.Reader) (*OCILayerResult, error) {
	return s.scanTar(reader)
}

func (s *OCILayerScanner) scanTar(reader io.Reader) (*OCILayerResult, error) {
	result := &OCILayerResult{
		Packages:       make([]OCIPackageInfo, 0),
		FrameworkHints: make(map[string]interface{}),
		NotablePaths:   make([]string, 0),
	}

	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Debugf("OCILayerScanner: tar read error (partial scan): %v", err)
			break
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		result.FileCount++
		name := filepath.Clean(header.Name)

		// Check notable paths
		if isNotablePath(name) {
			result.NotablePaths = append(result.NotablePaths, name)
		}

		// Only parse small files for package metadata
		if header.Size > maxSmallFileSize {
			continue
		}

		// pip METADATA
		if s.pipMetadataRe.MatchString(name) {
			pkgs := s.parsePipMetadata(tr)
			result.Packages = append(result.Packages, pkgs...)
			continue
		}

		// conda-meta JSON
		if s.condaMetaRe.MatchString(name) {
			pkg := s.parseCondaMeta(name)
			if pkg != nil {
				result.Packages = append(result.Packages, *pkg)
			}
			continue
		}

		// dpkg status
		if s.aptStatusRe.MatchString(name) {
			pkgs := s.parseDpkgStatus(tr)
			result.Packages = append(result.Packages, pkgs...)
			continue
		}
	}

	result.FrameworkHints = s.deriveFrameworkHints(result.Packages)
	return result, nil
}

func (s *OCILayerScanner) parsePipMetadata(r io.Reader) []OCIPackageInfo {
	var pkgs []OCIPackageInfo
	scanner := bufio.NewScanner(r)
	var name, version string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Name: ") {
			name = strings.TrimPrefix(line, "Name: ")
		} else if strings.HasPrefix(line, "Version: ") {
			version = strings.TrimPrefix(line, "Version: ")
		}
		if name != "" && version != "" {
			pkgs = append(pkgs, OCIPackageInfo{Manager: "pip", Name: name, Version: version})
			name, version = "", ""
		}
	}
	return pkgs
}

func (s *OCILayerScanner) parseCondaMeta(path string) *OCIPackageInfo {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".json")

	// conda-meta filenames are typically: name-version-build.json
	parts := strings.SplitN(base, "-", 3)
	if len(parts) < 2 {
		return nil
	}
	return &OCIPackageInfo{
		Manager: "conda",
		Name:    parts[0],
		Version: parts[1],
	}
}

func (s *OCILayerScanner) parseDpkgStatus(r io.Reader) []OCIPackageInfo {
	var pkgs []OCIPackageInfo
	scanner := bufio.NewScanner(r)
	var name, version string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Package: ") {
			name = strings.TrimPrefix(line, "Package: ")
			version = ""
		} else if strings.HasPrefix(line, "Version: ") {
			version = strings.TrimPrefix(line, "Version: ")
		} else if line == "" && name != "" && version != "" {
			pkgs = append(pkgs, OCIPackageInfo{Manager: "apt", Name: name, Version: version})
			name, version = "", ""
		}
	}
	if name != "" && version != "" {
		pkgs = append(pkgs, OCIPackageInfo{Manager: "apt", Name: name, Version: version})
	}
	return pkgs
}

// deriveFrameworkHints maps known package names to framework categories.
func (s *OCILayerScanner) deriveFrameworkHints(packages []OCIPackageInfo) map[string]interface{} {
	hints := make(map[string]interface{})

	frameworkMap := map[string]string{
		"vllm":          "vllm",
		"sglang":        "sglang",
		"deepspeed":     "deepspeed",
		"megatron-core": "megatron",
		"torch":         "pytorch",
		"pytorch":       "pytorch",
		"tensorflow":    "tensorflow",
		"tensorflow-gpu": "tensorflow",
		"jax":           "jax",
		"jaxlib":        "jax",
		"transformers":  "huggingface",
		"accelerate":    "huggingface",
		"trl":           "trl",
		"peft":          "peft",
		"triton":        "triton",
		"tritonclient":  "triton",
		"lightning":     "lightning",
		"pytorch-lightning": "lightning",
		"colossalai":    "colossalai",
		"ray":           "ray",
	}

	servingFrameworks := map[string]bool{
		"vllm": true, "sglang": true, "triton": true,
	}
	trainingFrameworks := map[string]bool{
		"deepspeed": true, "megatron": true, "colossalai": true,
		"lightning": true, "trl": true, "peft": true,
	}
	runtimeFrameworks := map[string]bool{
		"pytorch": true, "tensorflow": true, "jax": true,
	}

	detected := make(map[string]bool)
	for _, pkg := range packages {
		normalizedName := strings.ToLower(pkg.Name)
		if fw, ok := frameworkMap[normalizedName]; ok {
			detected[fw] = true
		}
	}

	var serving, training, runtime []string
	for fw := range detected {
		if servingFrameworks[fw] {
			serving = append(serving, fw)
		}
		if trainingFrameworks[fw] {
			training = append(training, fw)
		}
		if runtimeFrameworks[fw] {
			runtime = append(runtime, fw)
		}
	}

	if len(serving) > 0 {
		hints["serving"] = serving
	}
	if len(training) > 0 {
		hints["training"] = training
	}
	if len(runtime) > 0 {
		hints["runtime"] = runtime
	}

	return hints
}

func isNotablePath(path string) bool {
	notablePatterns := []string{
		"requirements.txt",
		"setup.py",
		"setup.cfg",
		"pyproject.toml",
		"Dockerfile",
		"entrypoint.sh",
		"train.py",
		"serve.py",
		"inference.py",
		"deepspeed_config.json",
		"ds_config.json",
		"megatron",
		"accelerate_config.yaml",
	}
	base := filepath.Base(path)
	for _, pattern := range notablePatterns {
		if base == pattern || strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}
