package framework

import (
	"crypto/md5"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"

	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// SignatureExtractor extracts workload signatures
type SignatureExtractor struct {
	// List of sensitive environment variables (excluded from similarity calculation)
	sensitiveEnvKeys []string

	// List of dynamic labels (excluded from similarity calculation)
	dynamicLabelKeys []string
}

// Workload represents a simplified workload interface for signature extraction
type Workload struct {
	UID       string
	Image     string
	Command   []string
	Args      []string
	Env       map[string]string
	Labels    map[string]string
	Namespace string
}

// NewSignatureExtractor creates a new signature extractor
func NewSignatureExtractor() *SignatureExtractor {
	return &SignatureExtractor{
		sensitiveEnvKeys: []string{
			"PASSWORD", "TOKEN", "SECRET", "KEY", "CERT",
		},
		dynamicLabelKeys: []string{
			"pod-template-hash", "controller-revision-hash",
		},
	}
}

// ExtractSignature extracts the workload signature
func (e *SignatureExtractor) ExtractSignature(workload *Workload) *coreModel.WorkloadSignature {
	signature := &coreModel.WorkloadSignature{
		Image:     workload.Image,
		Command:   e.normalizeCommand(workload.Command),
		Args:      workload.Args,
		Env:       e.filterSensitiveEnv(workload.Env),
		Labels:    e.filterDynamicLabels(workload.Labels),
		Namespace: workload.Namespace,
	}

	// Calculate hashes
	signature.ImageHash = e.calculateImageHash(signature.Image)
	signature.CommandHash = e.calculateCommandHash(signature.Command)
	signature.EnvHash = e.calculateEnvHash(signature.Env)

	return signature
}

// normalizeCommand normalizes the command
func (e *SignatureExtractor) normalizeCommand(cmd []string) []string {
	if len(cmd) == 0 {
		return []string{}
	}

	normalized := make([]string, len(cmd))
	copy(normalized, cmd)

	// Remove path prefix, keep only command name
	if len(normalized) > 0 {
		normalized[0] = filepath.Base(normalized[0])

		// Normalize Python executables (python, python2, python3 -> python)
		if strings.HasPrefix(normalized[0], "python") {
			normalized[0] = "python"
		}
	}

	return normalized
}

// filterSensitiveEnv filters out sensitive environment variables
func (e *SignatureExtractor) filterSensitiveEnv(env map[string]string) map[string]string {
	filtered := make(map[string]string)

	for key, value := range env {
		isSensitive := false
		upperKey := strings.ToUpper(key)

		for _, sensitive := range e.sensitiveEnvKeys {
			if strings.Contains(upperKey, sensitive) {
				isSensitive = true
				break
			}
		}

		if !isSensitive {
			filtered[key] = value
		}
	}

	return filtered
}

// filterDynamicLabels filters out dynamic labels
func (e *SignatureExtractor) filterDynamicLabels(labels map[string]string) map[string]string {
	filtered := make(map[string]string)

	for key, value := range labels {
		isDynamic := false

		for _, dynamic := range e.dynamicLabelKeys {
			if key == dynamic {
				isDynamic = true
				break
			}
		}

		if !isDynamic {
			filtered[key] = value
		}
	}

	return filtered
}

// calculateImageHash calculates the image hash
func (e *SignatureExtractor) calculateImageHash(image string) string {
	hash := md5.Sum([]byte(strings.ToLower(image)))
	return hex.EncodeToString(hash[:])
}

// calculateCommandHash calculates the command hash
func (e *SignatureExtractor) calculateCommandHash(cmd []string) string {
	// Sort before hashing to avoid order dependency
	sorted := make([]string, len(cmd))
	copy(sorted, cmd)
	sort.Strings(sorted)

	joined := strings.Join(sorted, "|")
	hash := md5.Sum([]byte(joined))
	return hex.EncodeToString(hash[:])
}

// calculateEnvHash calculates the environment variables hash (only key variables)
func (e *SignatureExtractor) calculateEnvHash(env map[string]string) string {
	// Extract key environment variables
	keyVars := []string{
		"FRAMEWORK", "TRAINING_FRAMEWORK",
		"DEEPSPEED_CONFIG", "MEGATRON_CONFIG", "PRIMUS_CONFIG",
		"WORLD_SIZE", "RANK", "LOCAL_RANK",
	}

	var pairs []string
	for _, key := range keyVars {
		if value, ok := env[key]; ok {
			pairs = append(pairs, key+"="+value)
		}
	}

	sort.Strings(pairs)
	joined := strings.Join(pairs, "|")
	hash := md5.Sum([]byte(joined))
	return hex.EncodeToString(hash[:])
}

// ExtractImageRepo extracts the image repository address (for database query optimization)
func ExtractImageRepo(image string) string {
	// registry.example.com/primus:v1.2.3 -> registry.example.com/primus
	parts := strings.Split(image, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return image
}
