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
	Framework1  string    `json:"framework1"`
	Confidence1 float64   `json:"confidence1"`
	Sources1    []string  `json:"sources1"`
	Framework2  string    `json:"framework2"`
	Confidence2 float64   `json:"confidence2"`
	Sources2    []string  `json:"sources2"`
	DetectedAt  time.Time `json:"detected_at"`
}

// AggregationResult holds the result of evidence aggregation
type AggregationResult struct {
	Framework        string
	Frameworks       []string
	WorkloadType     string
	Confidence       float64
	Status           DetectionStatus
	FrameworkLayer   string
	WrapperFramework string
	BaseFramework    string
	EvidenceCount    int
	Sources          []string
	Conflicts        []DetectionConflict
}

// EvidenceAggregator aggregates evidence from multiple sources
type EvidenceAggregator struct {
	evidenceFacade  database.WorkloadDetectionEvidenceFacadeInterface
	detectionFacade database.WorkloadDetectionFacadeInterface
	sourceWeights   map[string]float64
}

// NewEvidenceAggregator creates a new EvidenceAggregator
func NewEvidenceAggregator() *EvidenceAggregator {
	return &EvidenceAggregator{
		evidenceFacade:  database.NewWorkloadDetectionEvidenceFacade(),
		detectionFacade: database.NewWorkloadDetectionFacade(),
		sourceWeights:   DefaultSourceWeights,
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

	// 2. Group evidence by framework and calculate votes
	frameworkVotes := a.calculateVotes(evidences)

	// 3. Collect sources
	sources := a.collectSources(evidences)

	// 4. Detect conflicts
	conflicts := a.detectConflicts(frameworkVotes)

	// 5. Select winning framework
	winner := a.selectWinner(frameworkVotes)

	if winner == nil {
		return &AggregationResult{
			EvidenceCount: len(evidences),
			Sources:       sources,
			Status:        DetectionStatusUnknown,
			Conflicts:     conflicts,
		}, nil
	}

	// 6. Calculate aggregated confidence
	aggregatedConfidence := a.calculateConfidence(winner, len(sources))

	// 7. Determine status based on confidence and conflicts
	status := a.determineStatus(aggregatedConfidence, len(sources), conflicts)

	// 8. Build result
	result := &AggregationResult{
		Framework:        winner.Framework,
		Frameworks:       a.buildFrameworkList(frameworkVotes),
		WorkloadType:     winner.WorkloadType,
		Confidence:       aggregatedConfidence,
		Status:           status,
		FrameworkLayer:   winner.FrameworkLayer,
		WrapperFramework: winner.WrapperFramework,
		BaseFramework:    winner.BaseFramework,
		EvidenceCount:    len(evidences),
		Sources:          sources,
		Conflicts:        conflicts,
	}

	// 9. Mark evidence as processed
	evidenceIDs := make([]int64, len(evidences))
	for i, ev := range evidences {
		evidenceIDs[i] = ev.ID
	}
	if err := a.evidenceFacade.MarkEvidenceProcessed(ctx, evidenceIDs); err != nil {
		log.Warnf("Failed to mark evidence as processed: %v", err)
	}

	// 10. Update detection state
	if err := a.updateDetectionState(ctx, workloadUID, result); err != nil {
		log.Warnf("Failed to update detection state: %v", err)
	}

	return result, nil
}

// AggregateAllEvidence aggregates ALL evidence for a workload (including processed)
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

	// Calculate votes
	frameworkVotes := a.calculateVotes(evidences)
	sources := a.collectSources(evidences)
	conflicts := a.detectConflicts(frameworkVotes)
	winner := a.selectWinner(frameworkVotes)

	if winner == nil {
		return &AggregationResult{
			EvidenceCount: len(evidences),
			Sources:       sources,
			Status:        DetectionStatusUnknown,
			Conflicts:     conflicts,
		}, nil
	}

	aggregatedConfidence := a.calculateConfidence(winner, len(sources))
	status := a.determineStatus(aggregatedConfidence, len(sources), conflicts)

	return &AggregationResult{
		Framework:        winner.Framework,
		Frameworks:       a.buildFrameworkList(frameworkVotes),
		WorkloadType:     winner.WorkloadType,
		Confidence:       aggregatedConfidence,
		Status:           status,
		FrameworkLayer:   winner.FrameworkLayer,
		WrapperFramework: winner.WrapperFramework,
		BaseFramework:    winner.BaseFramework,
		EvidenceCount:    len(evidences),
		Sources:          sources,
		Conflicts:        conflicts,
	}, nil
}

// calculateVotes calculates votes for each framework from evidence
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

// detectConflicts detects conflicts between frameworks
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
