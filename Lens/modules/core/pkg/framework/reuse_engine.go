package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ReuseEngine is the metadata reuse engine
type ReuseEngine struct {
	db     database.AiWorkloadMetadataFacadeInterface
	config coreModel.ReuseConfig

	// Signature extractor
	signatureExtractor *SignatureExtractor

	// Similarity calculator
	similarityCalc *SimilarityCalculator

	// Caches
	signatureCache  *cache.Cache // Signature cache
	similarityCache *cache.Cache // Similarity cache

	// Metrics recorder
	metrics *MetricsRecorder
}

// NewReuseEngine creates a new reuse engine
func NewReuseEngine(
	db database.AiWorkloadMetadataFacadeInterface,
	config coreModel.ReuseConfig,
) *ReuseEngine {
	cacheTTL := time.Duration(config.CacheTTLMinutes) * time.Minute

	return &ReuseEngine{
		db:                 db,
		config:             config,
		signatureExtractor: NewSignatureExtractor(),
		similarityCalc:     NewSimilarityCalculator(),
		signatureCache:     cache.New(cacheTTL, cacheTTL*2),
		similarityCache:    cache.New(cacheTTL, cacheTTL*2),
		metrics:            NewMetricsRecorder(),
	}
}

// TryReuse attempts to reuse existing metadata
func (r *ReuseEngine) TryReuse(
	ctx context.Context,
	workload *Workload,
) (*coreModel.FrameworkDetection, error) {
	if !r.config.Enabled {
		logrus.Debug("Reuse engine is disabled")
		return nil, nil
	}

	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		logrus.Debugf("TryReuse took %v", duration)
		r.metrics.RecordDuration(duration)
	}()

	// Step 1: Extract WorkloadSignature
	signature := r.signatureExtractor.ExtractSignature(workload)
	logrus.Debugf("Extracted signature for workload %s: image=%s",
		workload.UID, signature.Image)

	// Step 2: Check signature cache
	cacheKey := r.buildCacheKey(signature)
	if cached, found := r.signatureCache.Get(cacheKey); found {
		logrus.Debugf("Found cached signature for key %s", cacheKey)
		r.metrics.RecordCacheHit("signature")
		if detection, ok := cached.(*coreModel.FrameworkDetection); ok {
			r.metrics.RecordAttempt("success")
			return detection, nil
		}
	}
	r.metrics.RecordCacheMiss("signature")

	// Step 3: Query candidate workloads
	candidates, err := r.findCandidates(ctx, signature)
	if err != nil {
		r.metrics.RecordAttempt("error")
		return nil, fmt.Errorf("failed to find candidates: %w", err)
	}

	r.metrics.RecordCandidateCount(len(candidates))

	if len(candidates) == 0 {
		logrus.Debug("No candidate workloads found")
		r.metrics.RecordAttempt("no_candidate")
		return nil, nil
	}

	logrus.Infof("Found %d candidate workloads", len(candidates))

	// Step 4: Calculate similarity scores
	similarities := r.calculateSimilarities(signature, candidates)

	// Step 5: Sort and select best candidate
	sort.Slice(similarities, func(i, j int) bool {
		return similarities[i].Score > similarities[j].Score
	})

	best := similarities[0]
	logrus.Infof("Best candidate: workload=%s, score=%.4f",
		best.WorkloadUID, best.Score)

	// Step 6: Check threshold
	r.metrics.RecordSimilarityScore(best.Score)
	if best.Score < r.config.MinSimilarityScore {
		logrus.Infof("Best similarity score %.4f below threshold %.4f, not reusing",
			best.Score, r.config.MinSimilarityScore)
		r.metrics.RecordAttempt("below_threshold")
		return nil, nil
	}

	// Step 7: Load and copy detection
	detection, err := r.loadAndCopyDetection(ctx, best.WorkloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to load detection: %w", err)
	}

	// Step 8: Apply confidence decay
	originalConfidence := detection.Confidence
	detection.Confidence *= r.config.ConfidenceDecayRate

	// Step 9: Mark reuse status
	detection.Status = coreModel.DetectionStatusReused

	// Step 10: Add reuse information
	detection.ReuseInfo = &coreModel.ReuseInfo{
		ReusedFrom:         best.WorkloadUID,
		ReusedAt:           time.Now(),
		SimilarityScore:    best.Score,
		OriginalConfidence: originalConfidence,
	}

	// Clear original Sources and Conflicts, ready to collect new ones
	detection.Sources = []coreModel.DetectionSource{{
		Source:     "reuse",
		Frameworks: detection.Frameworks,
		Type:       detection.Type,
		Confidence: detection.Confidence,
		DetectedAt: time.Now(),
		Evidence: map[string]interface{}{
			"method":              "workload_similarity",
			"reused_from":         best.WorkloadUID,
			"similarity_score":    best.Score,
			"original_confidence": originalConfidence,
			"similarity_details":  best.Details,
		},
	}}
	detection.Conflicts = []coreModel.DetectionConflict{}
	detection.UpdatedAt = time.Now()

	// Step 11: Cache result
	r.signatureCache.Set(cacheKey, detection, cache.DefaultExpiration)

	// Step 12: Record metrics
	r.metrics.RecordAttempt("success")
	// Use first framework from Frameworks array for metrics
	var primaryFramework string
	if len(detection.Frameworks) > 0 {
		primaryFramework = detection.Frameworks[0]
	}
	r.metrics.RecordSuccess(primaryFramework, workload.Namespace, best.Score, detection.Confidence)

	logrus.Infof("Successfully reused metadata from workload %s (score=%.4f, confidence=%.2f)",
		best.WorkloadUID, best.Score, detection.Confidence)

	return detection, nil
}

// findCandidates finds candidate workloads
func (r *ReuseEngine) findCandidates(
	ctx context.Context,
	signature *coreModel.WorkloadSignature,
) ([]*coreModel.CandidateWorkload, error) {
	// Extract image prefix for filtering
	imagePrefix := ExtractImageRepo(signature.Image)

	// Calculate time window
	timeWindow := time.Now().AddDate(0, 0, -r.config.TimeWindowDays)

	// Query candidates from database
	results, err := r.db.FindCandidateWorkloads(
		ctx,
		imagePrefix,
		timeWindow,
		r.config.MinConfidence,
		r.config.MaxCandidates,
	)
	if err != nil {
		return nil, err
	}

	// Convert database model to CandidateWorkload
	var candidates []*coreModel.CandidateWorkload
	for _, result := range results {
		candidate := &coreModel.CandidateWorkload{
			WorkloadUID: result.WorkloadUID,
			CreatedAt:   result.CreatedAt,
		}

		// Parse framework_detection from metadata
		if detectionData, ok := result.Metadata["framework_detection"]; ok {
			detectionJSON, err := json.Marshal(detectionData)
			if err != nil {
				logrus.Warnf("Failed to marshal detection data: %v", err)
				continue
			}

			var detection coreModel.FrameworkDetection
			if err := json.Unmarshal(detectionJSON, &detection); err != nil {
				logrus.Warnf("Failed to unmarshal detection: %v", err)
				continue
			}
			candidate.Detection = &detection
			candidate.Confidence = detection.Confidence
		}

		// Parse workload_signature from metadata
		if signatureData, ok := result.Metadata["workload_signature"]; ok {
			signatureJSON, err := json.Marshal(signatureData)
			if err != nil {
				logrus.Warnf("Failed to marshal signature data: %v", err)
				continue
			}

			var sig coreModel.WorkloadSignature
			if err := json.Unmarshal(signatureJSON, &sig); err != nil {
				logrus.Warnf("Failed to unmarshal signature: %v", err)
				continue
			}
			candidate.Signature = &sig
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// calculateSimilarities calculates similarities in batch
func (r *ReuseEngine) calculateSimilarities(
	signature *coreModel.WorkloadSignature,
	candidates []*coreModel.CandidateWorkload,
) []*coreModel.SimilarityResult {
	results := make([]*coreModel.SimilarityResult, 0, len(candidates))

	for _, candidate := range candidates {
		// Check cache
		cacheKey := r.buildSimilarityCacheKey(signature, candidate.Signature)
		if cached, found := r.similarityCache.Get(cacheKey); found {
			r.metrics.RecordCacheHit("similarity")
			if result, ok := cached.(*coreModel.SimilarityResult); ok {
				result.WorkloadUID = candidate.WorkloadUID
				results = append(results, result)
				continue
			}
		}
		r.metrics.RecordCacheMiss("similarity")

		// Calculate similarity
		result := r.similarityCalc.CalculateSimilarity(signature, candidate.Signature)
		result.WorkloadUID = candidate.WorkloadUID

		// Cache result
		r.similarityCache.Set(cacheKey, result, cache.DefaultExpiration)

		results = append(results, result)
	}

	return results
}

// loadAndCopyDetection loads and copies detection
func (r *ReuseEngine) loadAndCopyDetection(
	ctx context.Context,
	workloadUID string,
) (*coreModel.FrameworkDetection, error) {
	// Load metadata from database
	metadata, err := r.db.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		return nil, err
	}

	if metadata == nil {
		return nil, fmt.Errorf("metadata not found for workload %s", workloadUID)
	}

	// Parse framework_detection from metadata
	if detectionData, ok := metadata.Metadata["framework_detection"]; ok {
		detectionJSON, err := json.Marshal(detectionData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal detection data: %w", err)
		}

		var detection coreModel.FrameworkDetection
		if err := json.Unmarshal(detectionJSON, &detection); err != nil {
			return nil, fmt.Errorf("failed to unmarshal detection: %w", err)
		}

		// Deep copy - return a new instance
		return &coreModel.FrameworkDetection{
			Frameworks: detection.Frameworks,
			Type:       detection.Type,
			Confidence: detection.Confidence,
			Version:    detection.Version,
		}, nil
	}

	return nil, fmt.Errorf("no framework detection in metadata for workload %s", workloadUID)
}

// buildCacheKey builds signature cache key
func (r *ReuseEngine) buildCacheKey(signature *coreModel.WorkloadSignature) string {
	return fmt.Sprintf("sig:%s:%s:%s",
		signature.ImageHash,
		signature.CommandHash,
		signature.EnvHash)
}

// buildSimilarityCacheKey builds similarity cache key
func (r *ReuseEngine) buildSimilarityCacheKey(
	sig1, sig2 *coreModel.WorkloadSignature,
) string {
	return fmt.Sprintf("sim:%s:%s",
		r.buildCacheKey(sig1),
		r.buildCacheKey(sig2))
}
