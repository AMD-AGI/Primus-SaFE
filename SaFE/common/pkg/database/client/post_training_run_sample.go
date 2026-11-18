/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm/clause"
)

const (
	TPostTrainingRunSample = "post_training_run_sample"

	postTrainingSampleBatchSize = 200
	// The widths of the dataset and status columns.
	postTrainingSampleDatasetLen = 128
	postTrainingSampleStatusLen  = 16
)

// PostTrainingRunSample is one RL sample of a run, truncated by the launcher: a response to a prompt of a rollout
// group, or of an evaluation set. A rerun of a rollout after a resume replaces its samples.
type PostTrainingRunSample struct {
	ID             int64           `gorm:"column:id;primaryKey;autoIncrement" json:"id" db:"id"`
	RunID          string          `gorm:"column:run_id" json:"runId" db:"run_id"`
	Kind           string          `gorm:"column:kind" json:"kind" db:"kind"`
	Step           int             `gorm:"column:step" json:"step" db:"step"`
	Dataset        string          `gorm:"column:dataset" json:"dataset" db:"dataset"`
	GroupIndex     int             `gorm:"column:group_index" json:"group" db:"group_index"`
	SampleIndex    int             `gorm:"column:sample_index" json:"index" db:"sample_index"`
	Prompt         string          `gorm:"column:prompt" json:"prompt" db:"prompt"`
	Response       string          `gorm:"column:response" json:"response" db:"response"`
	Label          sql.NullString  `gorm:"column:label" json:"label" db:"label"`
	Reward         sql.NullFloat64 `gorm:"column:reward" json:"reward" db:"reward"`
	RewardDetail   sql.NullString  `gorm:"column:reward_detail" json:"rewardDetail" db:"reward_detail"`
	Status         string          `gorm:"column:status" json:"status" db:"status"`
	ResponseTokens int             `gorm:"column:response_tokens" json:"tokens" db:"response_tokens"`
	PromptChars    int             `gorm:"column:prompt_chars" json:"promptChars" db:"prompt_chars"`
	ResponseChars  int             `gorm:"column:response_chars" json:"responseChars" db:"response_chars"`
	WallTime       time.Time       `gorm:"column:wall_time" json:"wallTime" db:"wall_time"`
}

// TableName sets the SQL table name for PostTrainingRunSample.
func (PostTrainingRunSample) TableName() string {
	return TPostTrainingRunSample
}

// PostTrainingRunSampleFilter selects the samples of one step for ListPostTrainingRunSamples.
type PostTrainingRunSampleFilter struct {
	RunID   string
	Kind    string
	Step    int
	Dataset string
}

// PostTrainingRunSampleStep is a step that has samples, with how many, per dataset.
type PostTrainingRunSampleStep struct {
	Step    int    `gorm:"column:step" json:"step"`
	Dataset string `gorm:"column:dataset" json:"dataset"`
	Count   int    `gorm:"column:count" json:"count"`
}

// UpsertPostTrainingRunSamples writes samples; a sample already stored under the same run, kind, step, dataset,
// group and index is replaced.
func (c *Client) UpsertPostTrainingRunSamples(ctx context.Context, samples []*PostTrainingRunSample) error {
	if len(samples) == 0 {
		return nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "run_id"}, {Name: "kind"}, {Name: "step"}, {Name: "dataset"},
			{Name: "group_index"}, {Name: "sample_index"}},
		DoUpdates: clause.AssignmentColumns([]string{"prompt", "response", "label", "reward", "reward_detail",
			"status", "response_tokens", "prompt_chars", "response_chars", "wall_time"}),
	}).CreateInBatches(cleanPostTrainingRunSamples(samples), postTrainingSampleBatchSize).Error
}

// postTrainingSampleKey is the unique key of a stored sample.
type postTrainingSampleKey struct {
	runID, kind, dataset     string
	step, group, sampleIndex int
}

// cleanPostTrainingRunSamples fits model output to its columns, as one refused sample fails the whole write: NULs,
// which text columns do not take, are dropped, the dataset and status are cut to their widths, and of the samples
// that then share a key the last is kept.
func cleanPostTrainingRunSamples(samples []*PostTrainingRunSample) []*PostTrainingRunSample {
	index := make(map[postTrainingSampleKey]int, len(samples))
	out := make([]*PostTrainingRunSample, 0, len(samples))
	for _, s := range samples {
		if s == nil {
			continue
		}
		s.Dataset = cutRunes(dropNUL(s.Dataset), postTrainingSampleDatasetLen)
		s.Status = cutRunes(dropNUL(s.Status), postTrainingSampleStatusLen)
		s.Prompt, s.Response = dropNUL(s.Prompt), dropNUL(s.Response)
		s.Label.String, s.RewardDetail.String = dropNUL(s.Label.String), dropNUL(s.RewardDetail.String)
		key := postTrainingSampleKey{runID: s.RunID, kind: s.Kind, dataset: s.Dataset, step: s.Step,
			group: s.GroupIndex, sampleIndex: s.SampleIndex}
		if i, ok := index[key]; ok {
			out[i] = s
			continue
		}
		index[key] = len(out)
		out = append(out, s)
	}
	return out
}

// dropNUL removes the NUL characters of s.
func dropNUL(s string) string {
	return strings.ReplaceAll(s, "\x00", "")
}

// cutRunes cuts s to at most n characters.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// CountPostTrainingRunSamples returns how many samples a run has stored.
func (c *Client) CountPostTrainingRunSamples(ctx context.Context, runID string) (int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return 0, err
	}
	var n int64
	err = db.WithContext(ctx).Model(&PostTrainingRunSample{}).Where("run_id = ?", runID).Count(&n).Error
	return n, err
}

// ListPostTrainingRunSamples returns the samples of one step of a run, by group and index.
func (c *Client) ListPostTrainingRunSamples(ctx context.Context,
	filter PostTrainingRunSampleFilter) ([]*PostTrainingRunSample, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var out []*PostTrainingRunSample
	err = db.WithContext(ctx).Where("run_id = ? AND kind = ? AND step = ? AND dataset = ?",
		filter.RunID, filter.Kind, filter.Step, filter.Dataset).
		Order("group_index, sample_index").Find(&out).Error
	return out, err
}

// ListPostTrainingRunSampleSteps returns the steps of a kind of sample with their counts per dataset, newest first.
func (c *Client) ListPostTrainingRunSampleSteps(ctx context.Context, runID, kind string) ([]PostTrainingRunSampleStep, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var out []PostTrainingRunSampleStep
	err = db.WithContext(ctx).Model(&PostTrainingRunSample{}).Select("step, dataset, COUNT(*) AS count").
		Where("run_id = ? AND kind = ?", runID, kind).Group("step, dataset").
		Order("step DESC, dataset").Scan(&out).Error
	return out, err
}

// DeletePostTrainingRunSamples removes a run's samples: all of them, or those after step when a resumed attempt
// reruns those rollouts.
func (c *Client) DeletePostTrainingRunSamples(ctx context.Context, runID string, afterStep *int) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	query := db.WithContext(ctx).Where("run_id = ?", runID)
	if afterStep != nil {
		query = query.Where("step > ?", *afterStep)
	}
	return query.Delete(&PostTrainingRunSample{}).Error
}
