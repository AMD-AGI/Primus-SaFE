// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package detection

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// DetectionStatus represents the status of framework detection
type DetectionStatus string

const (
	DetectionStatusUnknown   DetectionStatus = "unknown"
	DetectionStatusSuspected DetectionStatus = "suspected"
	DetectionStatusConfirmed DetectionStatus = "confirmed"
	DetectionStatusVerified  DetectionStatus = "verified"
	DetectionStatusConflict  DetectionStatus = "conflict"
)

// SourceWeight defines weight configuration for different evidence sources
type SourceWeight struct {
	Source      string
	Weight      float64
	Description string
}

// DefaultSourceWeights defines the default weight configuration for evidence sources
// Higher weight = more trustworthy source
var DefaultSourceWeights = map[string]float64{
	"wandb":            1.00, // Highest priority - explicit framework info from WandB
	"import_detection": 0.95, // Import detection from WandB metadata
	"user_override":    0.95, // User explicit override
	"process":          0.85, // Process cmdline analysis
	"env":              0.80, // Environment variables
	"log":              0.75, // Log pattern matching
	"active_detection": 0.70, // Active probing results
	"image":            0.60, // Container image name
	"label":            0.50, // Pod labels/annotations
	"default":          0.30, // Default/fallback
}

// Thresholds for detection status determination
const (
	ConfidenceThresholdVerified  = 0.80
	ConfidenceThresholdConfirmed = 0.60
	ConfidenceThresholdSuspected = 0.40

	// Multi-source bonus configuration
	MultiSourceBonusPerSource = 0.05
	MultiSourceBonusMax       = 0.15

	// Conflict detection thresholds
	ConflictConfidenceThreshold = 0.50 // If two frameworks both have > 50% confidence, it's a conflict
)

// FrameworkVote tracks voting information for a framework
type FrameworkVote struct {
	Framework         string
	TotalScore        float64
	VoteCount         int
	HighestConfidence float64
	Sources           []string
	WorkloadType      string
	FrameworkLayer    string
	WrapperFramework  string
	BaseFramework     string
}

// DetectionConflict represents a conflict between detection sources
type DetectionConflict struct {
	Framework1   string  `json:"framework1"`
	Confidence1  float64 `json:"confidence1"`
	Sources1     []string `json:"sources1"`
	Framework2   string  `json:"framework2"`
	Confidence2  float64 `json:"confidence2"`
	Sources2     []string `json:"sources2"`
	DetectedAt   time.Time `json:"detected_at"`
}

// AggregationResult holds the result of evidence aggregation
type AggregationResult struct {
	Framework              string
	Frameworks             []string
	WorkloadType           string
	Confidence             float64
	Status                 DetectionStatus
	FrameworkLayer         string
	WrapperFramework       string
	OrchestrationFramework string // L2: Orchestration framework
	RuntimeFramework       string // L3: Runtime framework (formerly BaseFramework)
	BaseFramework          string // Deprecated: use RuntimeFramework, kept for backward compatibility
	EvidenceCount          int
	Sources                []string
	Conflicts              []DetectionConflict
}

// MultiLayerFrameworkVotes holds votes separated by layer
type MultiLayerFrameworkVotes struct {
	WrapperVotes       map[string]*FrameworkVote // L1: primus, lightning
	OrchestrationVotes map[string]*FrameworkVote // L2: megatron, deepspeed
	RuntimeVotes       map[string]*FrameworkVote // L3: pytorch, tensorflow, jax
	InferenceVotes     map[string]*FrameworkVote // Inference: vllm, triton
}

// MultiLayerWinners holds winners for each layer
type MultiLayerWinners struct {
	Wrapper       *FrameworkVote // L1 winner
	Orchestration *FrameworkVote // L2 winner
	Runtime       *FrameworkVote // L3 winner
	Inference     *FrameworkVote // Inference winner
}

// GetPrimaryFramework returns the highest-layer detected framework
func (w *MultiLayerWinners) GetPrimaryFramework() *FrameworkVote {
	// Priority: Wrapper > Orchestration > Runtime > Inference
	if w.Wrapper != nil {
		return w.Wrapper
	}
	if w.Orchestration != nil {
		return w.Orchestration
	}
	if w.Runtime != nil {
		return w.Runtime
	}
	return w.Inference
}

// GetFrameworkStack returns all detected frameworks as a stack
func (w *MultiLayerWinners) GetFrameworkStack() []string {
	var stack []string
	if w.Wrapper != nil {
		stack = append(stack, w.Wrapper.Framework)
	}
	if w.Orchestration != nil {
		stack = append(stack, w.Orchestration.Framework)
	}
	if w.Runtime != nil {
		stack = append(stack, w.Runtime.Framework)
	}
	if w.Inference != nil {
		stack = append(stack, w.Inference.Framework)
	}
	return stack
}

// EvidenceAggregator aggregates evidence from multiple sources
type EvidenceAggregator struct {
	evidenceFacade  database.WorkloadDetectionEvidenceFacadeInterface
	detectionFacade database.WorkloadDetectionFacadeInterface
	sourceWeights   map[string]float64
	layerResolver   *FrameworkLayerResolver
}

// NewEvidenceAggregator creates a new EvidenceAggregator
func NewEvidenceAggregator() *EvidenceAggregator {
	return &EvidenceAggregator{
		evidenceFacade:  database.NewWorkloadDetectionEvidenceFacade(),
		detectionFacade: database.NewWorkloadDetectionFacade(),
		sourceWeights:   DefaultSourceWeights,
		layerResolver:   GetLayerResolver(),
	}
}

// NewEvidenceAggregatorWithFacades creates a new EvidenceAggregator with custom facades
func NewEvidenceAggregatorWithFacades(
	evidenceFacade database.WorkloadDetectionEvidenceFacadeInterface,
	detectionFacade database.WorkloadDetectionFacadeInterface,
) *EvidenceAggregator {
	return &EvidenceAggregator{
		evidenceFacade:  evidenceFacade,
		detectionFacade: detectionFacade,
		sourceWeights:   DefaultSourceWeights,
		layerResolver:   GetLayerResolver(),
	}
}

// NewEvidenceAggregatorWithLayerResolver creates a new EvidenceAggregator with custom layer resolver
func NewEvidenceAggregatorWithLayerResolver(
	evidenceFacade database.WorkloadDetectionEvidenceFacadeInterface,
	detectionFacade database.WorkloadDetectionFacadeInterface,
	layerResolver *FrameworkLayerResolver,
) *EvidenceAggregator {
	return &EvidenceAggregator{
		evidenceFacade:  evidenceFacade,
		detectionFacade: detectionFacade,
		sourceWeights:   DefaultSourceWeights,
		layerResolver:   layerResolver,
	}
}

// SetSourceWeights sets custom source weights
func (a *EvidenceAggregator) SetSourceWeights(weights map[string]float64) {
	a.sourceWeights = weights
}

// GetSourceWeight returns the weight for a given source
func (a *EvidenceAggregator) GetSourceWeight(source string) float64 {
	if weight, ok := a.sourceWeights[source]; ok {
		return weight
	}
	return a.sourceWeights["default"]
}

// AggregateEvidence aggregates all unprocessed evidence for a workload
// Uses multi-layer voting to properly handle wrapper/orchestration/runtime framework stacks
func (a *EvidenceAggregator) AggregateEvidence(ctx context.Context, workloadUID string) (*AggregationResult, error) {
	// 1. Query all unprocessed evidence
	evidences, err := a.evidenceFacade.ListUnprocessedEvidence(ctx, workloadUID)
	if err != nil {
		return nil, err
	}

	if len(evidences) == 0 {
		// No new evidence, return current state
		return a.getCurrentState(ctx, workloadUID)
	}

	log.Debugf("Aggregating %d unprocessed evidence records for workload %s", len(evidences), workloadUID)

	// 2. Calculate multi-layer votes
	multiLayerVotes := a.calculateMultiLayerVotes(evidences)

	// 3. Collect sources
	sources := a.collectSources(evidences)

	// 4. Detect conflicts within each layer only
	conflicts := a.detectMultiLayerConflicts(multiLayerVotes)

	// 5. Select winners from each layer
	winners := a.selectMultiLayerWinners(multiLayerVotes)

	if winners.GetPrimaryFramework() == nil {
		return &AggregationResult{
			EvidenceCount: len(evidences),
			Sources:       sources,
			Status:        DetectionStatusUnknown,
			Conflicts:     conflicts,
		}, nil
	}

	// 6. Build result with multi-layer support
	result := a.buildMultiLayerResult(winners, evidences, sources, conflicts)

	// 7. Mark evidence as processed
	evidenceIDs := make([]int64, len(evidences))
	for i, ev := range evidences {
		evidenceIDs[i] = ev.ID
	}
	if err := a.evidenceFacade.MarkEvidenceProcessed(ctx, evidenceIDs); err != nil {
		log.Warnf("Failed to mark evidence as processed: %v", err)
	}

	// 8. Update detection state
	if err := a.updateDetectionState(ctx, workloadUID, result); err != nil {
		log.Warnf("Failed to update detection state: %v", err)
	}

	return result, nil
}

// AggregateAllEvidence aggregates ALL evidence for a workload (including processed)
// Uses multi-layer voting to properly handle wrapper/orchestration/runtime framework stacks
func (a *EvidenceAggregator) AggregateAllEvidence(ctx context.Context, workloadUID string) (*AggregationResult, error) {
	// Query all evidence
	evidences, err := a.evidenceFacade.ListEvidenceByWorkload(ctx, workloadUID)
	if err != nil {
		return nil, err
	}

	if len(evidences) == 0 {
		return &AggregationResult{
			Status: DetectionStatusUnknown,
		}, nil
	}

	// Calculate multi-layer votes
	multiLayerVotes := a.calculateMultiLayerVotes(evidences)
	sources := a.collectSources(evidences)
	conflicts := a.detectMultiLayerConflicts(multiLayerVotes)
	winners := a.selectMultiLayerWinners(multiLayerVotes)

	if winners.GetPrimaryFramework() == nil {
		return &AggregationResult{
			EvidenceCount: len(evidences),
			Sources:       sources,
			Status:        DetectionStatusUnknown,
			Conflicts:     conflicts,
		}, nil
	}

	return a.buildMultiLayerResult(winners, evidences, sources, conflicts), nil
}

// calculateVotes calculates votes for each framework from evidence (legacy method)
func (a *EvidenceAggregator) calculateVotes(evidences []*model.WorkloadDetectionEvidence) map[string]*FrameworkVote {
	frameworkVotes := make(map[string]*FrameworkVote)

	for _, ev := range evidences {
		if ev.Framework == "" {
			continue
		}

		weight := a.GetSourceWeight(ev.Source)
		score := ev.Confidence * weight

		if _, exists := frameworkVotes[ev.Framework]; !exists {
			frameworkVotes[ev.Framework] = &FrameworkVote{
				Framework:         ev.Framework,
				TotalScore:        0,
				VoteCount:         0,
				HighestConfidence: 0,
				Sources:           []string{},
			}
		}

		vote := frameworkVotes[ev.Framework]
		vote.TotalScore += score
		vote.VoteCount++
		vote.Sources = append(vote.Sources, ev.Source)

		if ev.Confidence > vote.HighestConfidence {
			vote.HighestConfidence = ev.Confidence
			vote.WrapperFramework = ev.WrapperFramework
			vote.BaseFramework = ev.BaseFramework
			vote.FrameworkLayer = ev.FrameworkLayer
			vote.WorkloadType = ev.WorkloadType
		}
	}

	return frameworkVotes
}

// calculateMultiLayerVotes calculates votes separated by framework layer
func (a *EvidenceAggregator) calculateMultiLayerVotes(evidences []*model.WorkloadDetectionEvidence) *MultiLayerFrameworkVotes {
	result := &MultiLayerFrameworkVotes{
		WrapperVotes:       make(map[string]*FrameworkVote),
		OrchestrationVotes: make(map[string]*FrameworkVote),
		RuntimeVotes:       make(map[string]*FrameworkVote),
		InferenceVotes:     make(map[string]*FrameworkVote),
	}

	for _, ev := range evidences {
		if ev.Framework == "" {
			continue
		}

		// Get layer from evidence first, fallback to config lookup
		layer := ev.FrameworkLayer
		if layer == "" && a.layerResolver != nil {
			layer = a.layerResolver.GetLayer(ev.Framework)
		}
		if layer == "" {
			layer = FrameworkLayerRuntime // Default to runtime
		}

		weight := a.GetSourceWeight(ev.Source)
		score := ev.Confidence * weight

		// Determine which vote map to use based on layer
		var voteMap map[string]*FrameworkVote
		switch layer {
		case FrameworkLayerWrapper:
			voteMap = result.WrapperVotes
		case FrameworkLayerOrchestration:
			voteMap = result.OrchestrationVotes
		case FrameworkLayerRuntime:
			voteMap = result.RuntimeVotes
		case FrameworkLayerInference:
			voteMap = result.InferenceVotes
		default:
			voteMap = result.RuntimeVotes // Default to runtime
		}

		if _, exists := voteMap[ev.Framework]; !exists {
			voteMap[ev.Framework] = &FrameworkVote{
				Framework:         ev.Framework,
				TotalScore:        0,
				VoteCount:         0,
				HighestConfidence: 0,
				Sources:           []string{},
				FrameworkLayer:    layer,
			}
		}

		vote := voteMap[ev.Framework]
		vote.TotalScore += score
		vote.VoteCount++
		vote.Sources = append(vote.Sources, ev.Source)

		if ev.Confidence > vote.HighestConfidence {
			vote.HighestConfidence = ev.Confidence
			vote.WrapperFramework = ev.WrapperFramework
			vote.BaseFramework = ev.BaseFramework
			vote.WorkloadType = ev.WorkloadType
		}

		// Also create votes for BaseFramework if present and different from main framework
		// This ensures multi-layer framework stacks (e.g., primus + megatron) are properly detected
		if ev.BaseFramework != "" && ev.BaseFramework != ev.Framework {
			baseLayer := ""
			if a.layerResolver != nil {
				baseLayer = a.layerResolver.GetLayer(ev.BaseFramework)
			}
			if baseLayer == "" {
				// Default: if main framework is wrapper, base is likely orchestration or runtime
				if layer == FrameworkLayerWrapper {
					baseLayer = FrameworkLayerOrchestration
				} else {
					baseLayer = FrameworkLayerRuntime
				}
			}

			var baseVoteMap map[string]*FrameworkVote
			switch baseLayer {
			case FrameworkLayerWrapper:
				baseVoteMap = result.WrapperVotes
			case FrameworkLayerOrchestration:
				baseVoteMap = result.OrchestrationVotes
			case FrameworkLayerRuntime:
				baseVoteMap = result.RuntimeVotes
			case FrameworkLayerInference:
				baseVoteMap = result.InferenceVotes
			default:
				baseVoteMap = result.OrchestrationVotes
			}

			// Use slightly lower confidence for derived base framework vote
			baseScore := ev.Confidence * weight * 0.9

			if _, exists := baseVoteMap[ev.BaseFramework]; !exists {
				baseVoteMap[ev.BaseFramework] = &FrameworkVote{
					Framework:         ev.BaseFramework,
					TotalScore:        0,
					VoteCount:         0,
					HighestConfidence: 0,
					Sources:           []string{},
					FrameworkLayer:    baseLayer,
				}
			}

			baseVote := baseVoteMap[ev.BaseFramework]
			baseVote.TotalScore += baseScore
			baseVote.VoteCount++
			baseVote.Sources = append(baseVote.Sources, ev.Source+"_derived")

			derivedConfidence := ev.Confidence * 0.9
			if derivedConfidence > baseVote.HighestConfidence {
				baseVote.HighestConfidence = derivedConfidence
				baseVote.WorkloadType = ev.WorkloadType
			}
		}
	}

	return result
}

// collectSources collects unique sources from evidence
func (a *EvidenceAggregator) collectSources(evidences []*model.WorkloadDetectionEvidence) []string {
	sourceMap := make(map[string]bool)
	for _, ev := range evidences {
		sourceMap[ev.Source] = true
	}

	sources := make([]string, 0, len(sourceMap))
	for source := range sourceMap {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

// detectConflicts detects conflicts between frameworks (legacy method)
func (a *EvidenceAggregator) detectConflicts(frameworkVotes map[string]*FrameworkVote) []DetectionConflict {
	var conflicts []DetectionConflict

	// Collect high-confidence frameworks
	var highConfidenceFrameworks []*FrameworkVote
	for _, vote := range frameworkVotes {
		if vote.HighestConfidence >= ConflictConfidenceThreshold {
			highConfidenceFrameworks = append(highConfidenceFrameworks, vote)
		}
	}

	// If more than one framework has high confidence, there's a conflict
	if len(highConfidenceFrameworks) > 1 {
		// Sort by confidence descending
		sort.Slice(highConfidenceFrameworks, func(i, j int) bool {
			return highConfidenceFrameworks[i].TotalScore > highConfidenceFrameworks[j].TotalScore
		})

		// Create conflict pairs (compare each with the winner)
		winner := highConfidenceFrameworks[0]
		for i := 1; i < len(highConfidenceFrameworks); i++ {
			other := highConfidenceFrameworks[i]
			conflicts = append(conflicts, DetectionConflict{
				Framework1:  winner.Framework,
				Confidence1: winner.HighestConfidence,
				Sources1:    winner.Sources,
				Framework2:  other.Framework,
				Confidence2: other.HighestConfidence,
				Sources2:    other.Sources,
				DetectedAt:  time.Now(),
			})
		}
	}

	return conflicts
}

// detectMultiLayerConflicts detects conflicts within each layer only
// Cross-layer combinations (e.g., primus + megatron + pytorch) are NOT conflicts
func (a *EvidenceAggregator) detectMultiLayerConflicts(votes *MultiLayerFrameworkVotes) []DetectionConflict {
	var conflicts []DetectionConflict

	// Check each layer for conflicts - conflicts only happen within the same layer
	wrapperConflicts := a.detectSingleLayerConflicts(votes.WrapperVotes)
	conflicts = append(conflicts, wrapperConflicts...)

	orchConflicts := a.detectSingleLayerConflicts(votes.OrchestrationVotes)
	conflicts = append(conflicts, orchConflicts...)

	runtimeConflicts := a.detectSingleLayerConflicts(votes.RuntimeVotes)
	conflicts = append(conflicts, runtimeConflicts...)

	inferenceConflicts := a.detectSingleLayerConflicts(votes.InferenceVotes)
	conflicts = append(conflicts, inferenceConflicts...)

	return conflicts
}

// detectSingleLayerConflicts detects conflicts within a single layer
func (a *EvidenceAggregator) detectSingleLayerConflicts(votes map[string]*FrameworkVote) []DetectionConflict {
	var conflicts []DetectionConflict

	var highConfidenceFrameworks []*FrameworkVote
	for _, vote := range votes {
		if vote.HighestConfidence >= ConflictConfidenceThreshold {
			highConfidenceFrameworks = append(highConfidenceFrameworks, vote)
		}
	}

	if len(highConfidenceFrameworks) > 1 {
		sort.Slice(highConfidenceFrameworks, func(i, j int) bool {
			return highConfidenceFrameworks[i].TotalScore > highConfidenceFrameworks[j].TotalScore
		})

		winner := highConfidenceFrameworks[0]
		for i := 1; i < len(highConfidenceFrameworks); i++ {
			other := highConfidenceFrameworks[i]
			conflicts = append(conflicts, DetectionConflict{
				Framework1:  winner.Framework,
				Confidence1: winner.HighestConfidence,
				Sources1:    winner.Sources,
				Framework2:  other.Framework,
				Confidence2: other.HighestConfidence,
				Sources2:    other.Sources,
				DetectedAt:  time.Now(),
			})
		}
	}

	return conflicts
}

// selectMultiLayerWinners selects winners for each layer
func (a *EvidenceAggregator) selectMultiLayerWinners(votes *MultiLayerFrameworkVotes) *MultiLayerWinners {
	return &MultiLayerWinners{
		Wrapper:       selectVoteWinner(votes.WrapperVotes),
		Orchestration: selectVoteWinner(votes.OrchestrationVotes),
		Runtime:       selectVoteWinner(votes.RuntimeVotes),
		Inference:     selectVoteWinner(votes.InferenceVotes),
	}
}

// selectVoteWinner selects the winning vote from a vote map
func selectVoteWinner(votes map[string]*FrameworkVote) *FrameworkVote {
	var winner *FrameworkVote
	for _, vote := range votes {
		if winner == nil || vote.TotalScore > winner.TotalScore {
			winner = vote
		}
	}
	return winner
}

// buildMultiLayerResult builds aggregation result with multi-layer support
func (a *EvidenceAggregator) buildMultiLayerResult(
	winners *MultiLayerWinners,
	evidences []*model.WorkloadDetectionEvidence,
	sources []string,
	conflicts []DetectionConflict,
) *AggregationResult {
	result := &AggregationResult{
		EvidenceCount: len(evidences),
		Sources:       sources,
		Conflicts:     conflicts,
	}

	// Set each layer's framework
	if winners.Wrapper != nil {
		result.WrapperFramework = winners.Wrapper.Framework
	}
	if winners.Orchestration != nil {
		result.OrchestrationFramework = winners.Orchestration.Framework
	}
	if winners.Runtime != nil {
		result.RuntimeFramework = winners.Runtime.Framework
		result.BaseFramework = winners.Runtime.Framework // Backward compatibility
	}

	// For backward compatibility: if base_framework is empty but orchestration exists,
	// use orchestration as base_framework (since orchestration is the "base" for wrapper frameworks)
	if result.BaseFramework == "" && winners.Orchestration != nil {
		result.BaseFramework = winners.Orchestration.Framework
	}

	// Primary framework: highest layer takes precedence
	primary := winners.GetPrimaryFramework()
	if primary != nil {
		result.Framework = primary.Framework
		result.FrameworkLayer = primary.FrameworkLayer
		result.WorkloadType = primary.WorkloadType
	}

	// Build frameworks list (stack from top to bottom)
	result.Frameworks = winners.GetFrameworkStack()

	// Calculate combined confidence
	// More layers detected = higher confidence
	layerCount := 0
	totalConfidence := 0.0
	if winners.Wrapper != nil {
		layerCount++
		totalConfidence += winners.Wrapper.HighestConfidence
	}
	if winners.Orchestration != nil {
		layerCount++
		totalConfidence += winners.Orchestration.HighestConfidence
	}
	if winners.Runtime != nil {
		layerCount++
		totalConfidence += winners.Runtime.HighestConfidence
	}
	if winners.Inference != nil {
		layerCount++
		totalConfidence += winners.Inference.HighestConfidence
	}

	if layerCount > 0 {
		// Base confidence from primary framework
		if primary != nil {
			result.Confidence = primary.HighestConfidence
		}
		// Multi-layer detection bonus
		if layerCount > 1 {
			bonus := float64(layerCount-1) * MultiSourceBonusPerSource
			result.Confidence = math.Min(1.0, result.Confidence+bonus)
		}
	}

	// Round to 3 decimal places
	result.Confidence = math.Round(result.Confidence*1000) / 1000

	// Determine status
	result.Status = a.determineStatus(result.Confidence, len(sources), conflicts)

	return result
}

// selectWinner selects the winning framework
func (a *EvidenceAggregator) selectWinner(frameworkVotes map[string]*FrameworkVote) *FrameworkVote {
	var winner *FrameworkVote

	for _, vote := range frameworkVotes {
		if winner == nil || vote.TotalScore > winner.TotalScore {
			winner = vote
		}
	}

	return winner
}

// calculateConfidence calculates aggregated confidence with multi-source bonus
func (a *EvidenceAggregator) calculateConfidence(winner *FrameworkVote, sourceCount int) float64 {
	if winner == nil || winner.VoteCount == 0 {
		return 0.0
	}

	// Base confidence from highest single source
	baseConfidence := winner.HighestConfidence

	// Multi-source bonus: more sources agreeing increases confidence
	uniqueSourceCount := len(a.uniqueSources(winner.Sources))
	sourceBonus := math.Min(MultiSourceBonusMax, float64(uniqueSourceCount-1)*MultiSourceBonusPerSource)

	// Calculate final confidence (capped at 1.0)
	finalConfidence := math.Min(1.0, baseConfidence+sourceBonus)

	return math.Round(finalConfidence*1000) / 1000 // Round to 3 decimal places
}

// uniqueSources returns unique sources from a list
func (a *EvidenceAggregator) uniqueSources(sources []string) []string {
	sourceMap := make(map[string]bool)
	for _, s := range sources {
		sourceMap[s] = true
	}

	unique := make([]string, 0, len(sourceMap))
	for s := range sourceMap {
		unique = append(unique, s)
	}
	return unique
}

// determineStatus determines detection status based on confidence and conflicts
func (a *EvidenceAggregator) determineStatus(confidence float64, sourceCount int, conflicts []DetectionConflict) DetectionStatus {
	// If there are unresolved conflicts
	if len(conflicts) > 0 {
		return DetectionStatusConflict
	}

	// Based on confidence thresholds
	switch {
	case confidence >= ConfidenceThresholdVerified:
		return DetectionStatusVerified
	case confidence >= ConfidenceThresholdConfirmed:
		return DetectionStatusConfirmed
	case confidence >= ConfidenceThresholdSuspected:
		return DetectionStatusSuspected
	default:
		return DetectionStatusUnknown
	}
}

// buildFrameworkList builds a list of all detected frameworks sorted by score
func (a *EvidenceAggregator) buildFrameworkList(frameworkVotes map[string]*FrameworkVote) []string {
	type frameworkScore struct {
		framework string
		score     float64
	}

	scores := make([]frameworkScore, 0, len(frameworkVotes))
	for fw, vote := range frameworkVotes {
		scores = append(scores, frameworkScore{framework: fw, score: vote.TotalScore})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	frameworks := make([]string, len(scores))
	for i, s := range scores {
		frameworks[i] = s.framework
	}

	return frameworks
}

// getCurrentState retrieves the current detection state for a workload
func (a *EvidenceAggregator) getCurrentState(ctx context.Context, workloadUID string) (*AggregationResult, error) {
	detection, err := a.detectionFacade.GetDetection(ctx, workloadUID)
	if err != nil {
		return nil, err
	}

	if detection == nil {
		return &AggregationResult{
			Status: DetectionStatusUnknown,
		}, nil
	}

	// Get sources from evidence table
	sources, err := a.evidenceFacade.GetDistinctSourcesByWorkload(ctx, workloadUID)
	if err != nil {
		sources = []string{}
	}

	return &AggregationResult{
		Framework:        detection.Framework,
		Frameworks:       a.parseFrameworks(detection.Frameworks),
		WorkloadType:     detection.WorkloadType,
		Confidence:       detection.Confidence,
		Status:           DetectionStatus(detection.Status),
		FrameworkLayer:   detection.FrameworkLayer,
		WrapperFramework: detection.WrapperFramework,
		BaseFramework:    detection.BaseFramework,
		EvidenceCount:    int(detection.EvidenceCount),
		Sources:          sources,
	}, nil
}

// parseFrameworks parses frameworks from ExtJSON
func (a *EvidenceAggregator) parseFrameworks(frameworks model.ExtJSON) []string {
	if len(frameworks) == 0 {
		return []string{}
	}

	var result []string
	if err := frameworks.UnmarshalTo(&result); err != nil {
		return []string{}
	}
	return result
}

// updateDetectionState updates the detection state in the database
func (a *EvidenceAggregator) updateDetectionState(ctx context.Context, workloadUID string, result *AggregationResult) error {
	// Check if detection record exists
	existing, err := a.detectionFacade.GetDetection(ctx, workloadUID)
	if err != nil {
		return err
	}

	// Convert frameworks to JSON
	var frameworksJSON model.ExtJSON
	if err := frameworksJSON.MarshalFrom(result.Frameworks); err != nil {
		log.Warnf("Failed to marshal frameworks: %v", err)
	}

	// Convert sources to JSON
	var sourcesJSON model.ExtJSON
	if err := sourcesJSON.MarshalFrom(result.Sources); err != nil {
		log.Warnf("Failed to marshal sources: %v", err)
	}

	// Convert conflicts to JSON
	var conflictsJSON model.ExtJSON
	if err := conflictsJSON.MarshalFrom(result.Conflicts); err != nil {
		log.Warnf("Failed to marshal conflicts: %v", err)
	}

	now := time.Now()

	if existing == nil {
		// Create new detection record
		detection := &model.WorkloadDetection{
			WorkloadUID:      workloadUID,
			Status:           string(result.Status),
			Framework:        result.Framework,
			Frameworks:       frameworksJSON,
			WorkloadType:     result.WorkloadType,
			Confidence:       result.Confidence,
			FrameworkLayer:   result.FrameworkLayer,
			WrapperFramework: result.WrapperFramework,
			BaseFramework:    result.BaseFramework,
			DetectionState:   "in_progress",
			EvidenceCount:    int32(result.EvidenceCount),
			EvidenceSources:  sourcesJSON,
			Conflicts:        conflictsJSON,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		if result.Status == DetectionStatusConfirmed || result.Status == DetectionStatusVerified {
			detection.DetectionState = "completed"
			detection.ConfirmedAt = now
		}

		return a.detectionFacade.CreateDetection(ctx, detection)
	}

	// Update existing detection record
	existing.Status = string(result.Status)
	existing.Framework = result.Framework
	existing.Frameworks = frameworksJSON
	existing.WorkloadType = result.WorkloadType
	existing.Confidence = result.Confidence
	existing.FrameworkLayer = result.FrameworkLayer
	existing.WrapperFramework = result.WrapperFramework
	existing.BaseFramework = result.BaseFramework
	existing.EvidenceCount = int32(result.EvidenceCount)
	existing.EvidenceSources = sourcesJSON
	existing.Conflicts = conflictsJSON
	existing.UpdatedAt = now

	if result.Status == DetectionStatusConfirmed || result.Status == DetectionStatusVerified {
		existing.DetectionState = "completed"
		if existing.ConfirmedAt.IsZero() {
			existing.ConfirmedAt = now
		}
	}

	return a.detectionFacade.UpdateDetection(ctx, existing)
}

// IsConfirmed returns true if the result status indicates confirmed detection
func (r *AggregationResult) IsConfirmed() bool {
	return r.Status == DetectionStatusConfirmed || r.Status == DetectionStatusVerified
}

// HasConflicts returns true if there are any detection conflicts
func (r *AggregationResult) HasConflicts() bool {
	return len(r.Conflicts) > 0
}

