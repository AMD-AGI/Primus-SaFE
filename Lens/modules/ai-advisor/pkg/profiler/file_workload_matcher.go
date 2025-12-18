package profiler

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// FileWorkloadMatch represents a match between a profiler file and a workload
type FileWorkloadMatch struct {
	WorkloadUID  string    `json:"workload_uid"`
	WorkloadName string    `json:"workload_name"`
	Namespace    string    `json:"namespace"`
	Confidence   string    `json:"confidence"` // "high", "medium", "low"
	MatchReason  string    `json:"match_reason"`
	CreatedAt    time.Time `json:"created_at"`
	EndAt        time.Time `json:"end_at,omitempty"`
}

// FileMatchResult represents the result of matching a profiler file to workloads
type FileMatchResult struct {
	FilePath       string              `json:"file_path"`
	FileName       string              `json:"file_name"`
	FileTimestamp  int64               `json:"file_timestamp_ns"` // Nanoseconds from filename
	FileTime       time.Time           `json:"file_time"`         // Converted timestamp
	Matches        []FileWorkloadMatch `json:"matches"`           // All matching workloads
	PrimaryMatch   *FileWorkloadMatch  `json:"primary_match"`     // Best match (if unique)
	HasConflict    bool                `json:"has_conflict"`      // Multiple workloads matched
	ConflictReason string              `json:"conflict_reason,omitempty"`
}

// FileWorkloadMatcher matches profiler files to workloads based on timestamps
type FileWorkloadMatcher struct {
	workloadFacade database.WorkloadFacadeInterface
}

// NewFileWorkloadMatcher creates a new file-workload matcher
func NewFileWorkloadMatcher() *FileWorkloadMatcher {
	return &FileWorkloadMatcher{
		workloadFacade: database.GetFacade().GetWorkload(),
	}
}

// timestampRegex matches the nanosecond timestamp in profiler filenames
// Format: primus-megatron-exp[...]-rank[0].{timestamp_ns}.pt.trace.json.gz
var timestampRegex = regexp.MustCompile(`\.(\d{19,})\.pt\.trace\.json`)

// ExtractTimestampFromFilename extracts the nanosecond timestamp from a profiler filename
func ExtractTimestampFromFilename(filename string) (int64, error) {
	// Try regex pattern first
	matches := timestampRegex.FindStringSubmatch(filename)
	if len(matches) >= 2 {
		ts, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		return ts, nil
	}

	// Fallback: try to find any 19-digit number in the filename
	parts := strings.Split(filepath.Base(filename), ".")
	for _, part := range parts {
		if len(part) >= 19 {
			ts, err := strconv.ParseInt(part, 10, 64)
			if err == nil && ts > 1000000000000000000 { // Reasonable nanosecond timestamp
				return ts, nil
			}
		}
	}

	return 0, fmt.Errorf("no timestamp found in filename: %s", filename)
}

// TimestampToTime converts a nanosecond timestamp to time.Time
func TimestampToTime(timestampNs int64) time.Time {
	return time.Unix(0, timestampNs)
}

// MatchFileToWorkloads matches a profiler file to potential workloads
func (m *FileWorkloadMatcher) MatchFileToWorkloads(
	ctx context.Context,
	filePath string,
	namespace string,
) (*FileMatchResult, error) {
	result := &FileMatchResult{
		FilePath: filePath,
		FileName: filepath.Base(filePath),
		Matches:  make([]FileWorkloadMatch, 0),
	}

	// Extract timestamp from filename
	timestamp, err := ExtractTimestampFromFilename(filePath)
	if err != nil {
		log.Warnf("Failed to extract timestamp from filename %s: %v", filePath, err)
		return result, nil // Return empty result, not an error
	}

	result.FileTimestamp = timestamp
	result.FileTime = TimestampToTime(timestamp)

	log.Debugf("Extracted timestamp from %s: %d -> %s", filePath, timestamp, result.FileTime)

	// Query workloads that were active at the file creation time
	// Add a buffer of 10 minutes before and after to handle profiler timing variations
	bufferDuration := 10 * time.Minute
	startTime := result.FileTime.Add(-bufferDuration)
	endTime := result.FileTime.Add(bufferDuration)

	workloads, err := m.workloadFacade.ListActiveTopLevelWorkloads(ctx, startTime, endTime, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to query active workloads: %w", err)
	}

	log.Debugf("Found %d potentially active workloads in time range [%s, %s]",
		len(workloads), startTime, endTime)

	// Filter workloads that were actually running when the file was created
	for _, workload := range workloads {
		// Check if file time falls within workload's runtime
		workloadEnd := workload.EndAt
		if workloadEnd.IsZero() || workloadEnd.Year() < 2000 {
			// Workload still running, use current time as end
			workloadEnd = time.Now()
		}

		// File should be created during workload execution
		// Allow some buffer for profiler delay (profiler typically dumps after training step)
		if result.FileTime.After(workload.CreatedAt.Add(-time.Minute)) &&
			result.FileTime.Before(workloadEnd.Add(5*time.Minute)) {

			match := FileWorkloadMatch{
				WorkloadUID:  workload.UID,
				WorkloadName: workload.Name,
				Namespace:    workload.Namespace,
				CreatedAt:    workload.CreatedAt,
				EndAt:        workload.EndAt,
			}

			// Calculate confidence based on timing
			match.Confidence, match.MatchReason = m.calculateConfidence(result.FileTime, workload)
			result.Matches = append(result.Matches, match)
		}
	}

	// Determine primary match and conflict status
	if len(result.Matches) == 0 {
		log.Warnf("No matching workload found for file %s (file time: %s)", filePath, result.FileTime)
	} else if len(result.Matches) == 1 {
		result.PrimaryMatch = &result.Matches[0]
		log.Infof("Matched file %s to workload %s (confidence: %s)",
			filePath, result.PrimaryMatch.WorkloadName, result.PrimaryMatch.Confidence)
	} else {
		// Multiple matches - conflict detected
		result.HasConflict = true
		result.ConflictReason = fmt.Sprintf("Multiple workloads (%d) were active at file creation time",
			len(result.Matches))

		// Find the best match based on confidence
		var bestMatch *FileWorkloadMatch
		for i := range result.Matches {
			if bestMatch == nil || compareConfidence(result.Matches[i].Confidence, bestMatch.Confidence) > 0 {
				bestMatch = &result.Matches[i]
			}
		}
		result.PrimaryMatch = bestMatch

		// Downgrade confidence for all matches due to conflict
		for i := range result.Matches {
			if result.Matches[i].Confidence == "high" {
				result.Matches[i].Confidence = "medium"
				result.Matches[i].MatchReason += " (downgraded due to conflict)"
			} else if result.Matches[i].Confidence == "medium" {
				result.Matches[i].Confidence = "low"
				result.Matches[i].MatchReason += " (downgraded due to conflict)"
			}
		}

		log.Warnf("Conflict detected for file %s: %d workloads matched. Primary: %s (confidence: %s)",
			filePath, len(result.Matches), result.PrimaryMatch.WorkloadName, result.PrimaryMatch.Confidence)
	}

	return result, nil
}

// calculateConfidence calculates the confidence level of a match
func (m *FileWorkloadMatcher) calculateConfidence(fileTime time.Time, workload *model.GpuWorkload) (string, string) {
	workloadStart := workload.CreatedAt
	workloadEnd := workload.EndAt
	if workloadEnd.IsZero() || workloadEnd.Year() < 2000 {
		workloadEnd = time.Now()
	}

	// Calculate how far the file time is from workload boundaries
	fromStart := fileTime.Sub(workloadStart)
	toEnd := workloadEnd.Sub(fileTime)
	duration := workloadEnd.Sub(workloadStart)

	// High confidence: file created well within workload duration
	// (after first 5 minutes and before last 5 minutes)
	if fromStart > 5*time.Minute && toEnd > 5*time.Minute {
		return "high", fmt.Sprintf("File created %.1f minutes after workload start", fromStart.Minutes())
	}

	// Medium confidence: file created near boundaries but within workload
	if fromStart > 0 && toEnd > 0 {
		if fromStart < 5*time.Minute {
			return "medium", fmt.Sprintf("File created shortly (%.1f min) after workload start", fromStart.Minutes())
		}
		return "medium", fmt.Sprintf("File created near workload end (%.1f min remaining)", toEnd.Minutes())
	}

	// Low confidence: timing is outside normal bounds
	if fromStart < 0 {
		return "low", fmt.Sprintf("File created %.1f min before workload started (profiler delay?)", -fromStart.Minutes())
	}
	if toEnd < 0 {
		return "low", fmt.Sprintf("File created %.1f min after workload ended", -toEnd.Minutes())
	}

	// Default case
	ratio := float64(fromStart) / float64(duration)
	return "medium", fmt.Sprintf("File created at %.0f%% of workload duration", ratio*100)
}

// compareConfidence compares two confidence levels
// Returns: 1 if a > b, -1 if a < b, 0 if equal
func compareConfidence(a, b string) int {
	levels := map[string]int{"high": 3, "medium": 2, "low": 1}
	aLevel := levels[a]
	bLevel := levels[b]
	if aLevel > bLevel {
		return 1
	} else if aLevel < bLevel {
		return -1
	}
	return 0
}

// MatchFilesToWorkload matches profiler files to a specific workload based on time range
// This is the inverse operation - given a workload, find files that belong to it
func (m *FileWorkloadMatcher) MatchFilesToWorkload(
	ctx context.Context,
	workloadUID string,
	files []string,
) ([]FileMatchResult, error) {
	// Get workload info
	workload, err := m.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}
	if workload == nil {
		return nil, fmt.Errorf("workload not found: %s", workloadUID)
	}

	results := make([]FileMatchResult, 0, len(files))

	workloadEnd := workload.EndAt
	if workloadEnd.IsZero() || workloadEnd.Year() < 2000 {
		workloadEnd = time.Now()
	}

	for _, filePath := range files {
		result := FileMatchResult{
			FilePath: filePath,
			FileName: filepath.Base(filePath),
			Matches:  make([]FileWorkloadMatch, 0),
		}

		// Extract timestamp
		timestamp, err := ExtractTimestampFromFilename(filePath)
		if err != nil {
			log.Debugf("Skipping file %s: %v", filePath, err)
			continue
		}

		result.FileTimestamp = timestamp
		result.FileTime = TimestampToTime(timestamp)

		// Check if file was created during workload execution
		if result.FileTime.After(workload.CreatedAt.Add(-time.Minute)) &&
			result.FileTime.Before(workloadEnd.Add(5*time.Minute)) {

			match := FileWorkloadMatch{
				WorkloadUID:  workload.UID,
				WorkloadName: workload.Name,
				Namespace:    workload.Namespace,
				CreatedAt:    workload.CreatedAt,
				EndAt:        workload.EndAt,
			}
			match.Confidence, match.MatchReason = m.calculateConfidence(result.FileTime, workload)

			result.Matches = append(result.Matches, match)
			result.PrimaryMatch = &result.Matches[0]
		}

		results = append(results, result)
	}

	return results, nil
}

// GetAllMatchedWorkloadUIDs returns all unique workload UIDs from matches
func (r *FileMatchResult) GetAllMatchedWorkloadUIDs() []string {
	uids := make([]string, 0, len(r.Matches))
	for _, match := range r.Matches {
		uids = append(uids, match.WorkloadUID)
	}
	return uids
}

// GetConfidence returns the overall confidence level for this file match
func (r *FileMatchResult) GetConfidence() string {
	if r.HasConflict {
		return "low"
	}
	if r.PrimaryMatch != nil {
		return r.PrimaryMatch.Confidence
	}
	return "none"
}
