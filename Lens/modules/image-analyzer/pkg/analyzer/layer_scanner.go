// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package analyzer

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// maxSmallFileSize is the maximum file size to read fully for analysis
	maxSmallFileSize = 1 * 1024 * 1024 // 1MB
)

// LayerResult holds the analysis results for a single image layer
type LayerResult struct {
	FileCount      int32
	Packages       []PackageInfo
	FrameworkHints map[string]interface{}
	NotablePaths   []string
}

// PackageInfo describes a package found in a layer
type PackageInfo struct {
	Manager string `json:"manager"` // pip, apt, conda
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// LayerScanner performs streaming analysis of OCI layer tar archives.
// It uses tar.Reader to iterate entries without extracting to disk,
// only reading small metadata files while skipping large data files.
type LayerScanner struct {
	pipInstallRe   *regexp.Regexp
	aptInstallRe   *regexp.Regexp
	condaInstallRe *regexp.Regexp
}

// NewLayerScanner creates a new LayerScanner
func NewLayerScanner() *LayerScanner {
	return &LayerScanner{
		pipInstallRe:   regexp.MustCompile(`pip[3]?\s+install\s+(.+?)(?:\s*&&|\s*$|\\)`),
		aptInstallRe:   regexp.MustCompile(`apt(?:-get)?\s+install\s+(?:-y\s+)?(.+?)(?:\s*&&|\s*$|\\)`),
		condaInstallRe: regexp.MustCompile(`conda\s+install\s+(?:-y\s+)?(.+?)(?:\s*&&|\s*$|\\)`),
	}
}

// ScanLayer performs streaming analysis of a gzipped tar layer blob.
// It reads tar entry headers and only reads content of small metadata files.
func (s *LayerScanner) ScanLayer(reader io.Reader) (*LayerResult, error) {
	result := &LayerResult{
		FrameworkHints: make(map[string]interface{}),
	}

	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		// Not gzipped - try reading as plain tar
		log.Debug("LayerScanner: layer is not gzipped, attempting plain tar")
		return s.scanTar(reader, result)
	}
	defer gzReader.Close()

	return s.scanTar(gzReader, result)
}

func (s *LayerScanner) scanTar(reader io.Reader, result *LayerResult) (*LayerResult, error) {
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Corrupted tar - return what we have so far
			log.Debugf("LayerScanner: tar read error (returning partial results): %v", err)
			break
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		result.FileCount++

		name := filepath.ToSlash(header.Name)

		// Check for notable paths
		if s.isNotablePath(name) {
			result.NotablePaths = append(result.NotablePaths, name)
		}

		// Only read content for small metadata files
		if header.Size > 0 && header.Size <= maxSmallFileSize {
			if s.isPipMetadata(name) {
				pkgs := s.parsePipMetadata(tarReader, name)
				result.Packages = append(result.Packages, pkgs...)
			} else if s.isCondaMeta(name) {
				pkgs := s.parseCondaMeta(tarReader, name)
				result.Packages = append(result.Packages, pkgs...)
			} else if s.isAptStatus(name) {
				pkgs := s.parseAptStatus(tarReader)
				result.Packages = append(result.Packages, pkgs...)
			}
		}
	}

	// Derive framework hints from discovered packages
	s.deriveFrameworkHints(result)

	return result, nil
}

// isNotablePath checks if the file path is interesting for intent analysis
func (s *LayerScanner) isNotablePath(name string) bool {
	// Python package directories - too many to list individually
	if strings.Contains(name, "site-packages/") || strings.Contains(name, "dist-packages/") {
		return false
	}

	notable := []string{
		"entrypoint", "start.sh", "run.sh", "serve.py", "train.py",
		"requirements.txt", "setup.py", "setup.cfg", "pyproject.toml",
		"Dockerfile", "conda-meta/",
	}
	lowerName := strings.ToLower(filepath.Base(name))
	for _, pattern := range notable {
		if strings.Contains(lowerName, strings.ToLower(pattern)) {
			return true
		}
	}

	// Config files
	if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		dir := filepath.Dir(name)
		if strings.Contains(dir, "config") || strings.Contains(dir, "etc") {
			return true
		}
	}

	return false
}

// isPipMetadata checks if this is a pip METADATA file.
// Matches both site-packages (standard Python) and dist-packages (Debian/Ubuntu).
func (s *LayerScanner) isPipMetadata(name string) bool {
	return (strings.Contains(name, "site-packages/") || strings.Contains(name, "dist-packages/")) &&
		(strings.HasSuffix(name, "/METADATA") || strings.HasSuffix(name, "/PKG-INFO"))
}

// isCondaMeta checks if this is a conda metadata file
func (s *LayerScanner) isCondaMeta(name string) bool {
	return strings.Contains(name, "conda-meta/") && strings.HasSuffix(name, ".json")
}

// isAptStatus checks if this is the dpkg status file
func (s *LayerScanner) isAptStatus(name string) bool {
	return strings.HasSuffix(name, "var/lib/dpkg/status") ||
		strings.HasSuffix(name, "var/lib/dpkg/status.d/")
}

// parsePipMetadata extracts package name and version from a pip METADATA file
func (s *LayerScanner) parsePipMetadata(reader io.Reader, path string) []PackageInfo {
	scanner := bufio.NewScanner(reader)
	var name, version string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break // End of headers
		}
		if strings.HasPrefix(line, "Name: ") {
			name = strings.TrimPrefix(line, "Name: ")
		} else if strings.HasPrefix(line, "Version: ") {
			version = strings.TrimPrefix(line, "Version: ")
		}
	}

	if name != "" {
		return []PackageInfo{{Manager: "pip", Name: name, Version: version}}
	}
	return nil
}

// parseCondaMeta extracts package info from a conda metadata JSON file
func (s *LayerScanner) parseCondaMeta(reader io.Reader, path string) []PackageInfo {
	// Conda metadata filenames are like: package-version-build.json
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".json")

	// Split on the last two hyphens: name-version-build
	parts := strings.Split(base, "-")
	if len(parts) >= 2 {
		// Heuristic: last part is build hash, second-to-last is version
		name := strings.Join(parts[:len(parts)-2], "-")
		version := parts[len(parts)-2]
		if name != "" {
			return []PackageInfo{{Manager: "conda", Name: name, Version: version}}
		}
	}
	return nil
}

// parseAptStatus extracts package info from dpkg status
func (s *LayerScanner) parseAptStatus(reader io.Reader) []PackageInfo {
	var packages []PackageInfo
	scanner := bufio.NewScanner(reader)
	var name, version string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Package: ") {
			name = strings.TrimPrefix(line, "Package: ")
		} else if strings.HasPrefix(line, "Version: ") {
			version = strings.TrimPrefix(line, "Version: ")
		} else if line == "" && name != "" {
			packages = append(packages, PackageInfo{Manager: "apt", Name: name, Version: version})
			name = ""
			version = ""
		}
	}
	if name != "" {
		packages = append(packages, PackageInfo{Manager: "apt", Name: name, Version: version})
	}
	return packages
}

// deriveFrameworkHints analyzes discovered packages to generate framework signals
func (s *LayerScanner) deriveFrameworkHints(result *LayerResult) {
	packageNames := make(map[string]bool)
	for _, pkg := range result.Packages {
		packageNames[strings.ToLower(pkg.Name)] = true
	}

	// Serving frameworks
	servingFrameworks := map[string]string{
		"vllm":                      "vllm",
		"text-generation-inference": "tgi",
		"sglang":                    "sglang",
		"tritonserver":              "triton",
		"torchserve":                "torchserve",
	}
	for pkg, name := range servingFrameworks {
		if packageNames[pkg] {
			result.FrameworkHints["serving_framework"] = name
			break
		}
	}

	// Training frameworks
	trainingFrameworks := map[string]string{
		"deepspeed":     "deepspeed",
		"megatron-core": "megatron",
		"megatron-lm":   "megatron",
		"trl":           "trl",
		"peft":          "peft",
		"transformers":  "huggingface",
		"lightning":     "lightning",
	}
	for pkg, name := range trainingFrameworks {
		if packageNames[pkg] {
			result.FrameworkHints["training_framework"] = name
			break
		}
	}

	// Runtime framework
	if packageNames["torch"] || packageNames["pytorch"] {
		result.FrameworkHints["runtime"] = "pytorch"
	} else if packageNames["jax"] || packageNames["jaxlib"] {
		result.FrameworkHints["runtime"] = "jax"
	} else if packageNames["tensorflow"] || packageNames["tf-nightly"] {
		result.FrameworkHints["runtime"] = "tensorflow"
	}
}
