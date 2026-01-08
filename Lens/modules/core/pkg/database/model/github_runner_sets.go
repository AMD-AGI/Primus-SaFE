package model

import (
	"time"
)

const TableNameGithubRunnerSets = "github_runner_sets"

// GithubRunnerSets represents an AutoScalingRunnerSet discovered in the cluster
type GithubRunnerSets struct {
	ID                 int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	UID                string    `gorm:"column:uid;not null" json:"uid"`
	Name               string    `gorm:"column:name;not null" json:"name"`
	Namespace          string    `gorm:"column:namespace;not null" json:"namespace"`
	GithubConfigURL    string    `gorm:"column:github_config_url" json:"github_config_url"`
	GithubConfigSecret string    `gorm:"column:github_config_secret" json:"github_config_secret"`
	RunnerGroup        string    `gorm:"column:runner_group" json:"runner_group"`
	GithubOwner        string    `gorm:"column:github_owner" json:"github_owner"`
	GithubRepo         string    `gorm:"column:github_repo" json:"github_repo"`
	MinRunners         int       `gorm:"column:min_runners;not null;default:0" json:"min_runners"`
	MaxRunners         int       `gorm:"column:max_runners;not null;default:0" json:"max_runners"`
	Status             string    `gorm:"column:status;not null;default:active" json:"status"`
	CurrentRunners     int       `gorm:"column:current_runners;not null;default:0" json:"current_runners"`
	DesiredRunners     int       `gorm:"column:desired_runners;not null;default:0" json:"desired_runners"`
	LastSyncAt         time.Time `gorm:"column:last_sync_at" json:"last_sync_at"`
	CreatedAt          time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (*GithubRunnerSets) TableName() string {
	return TableNameGithubRunnerSets
}

// RunnerSetStatus constants
const (
	RunnerSetStatusActive   = "active"
	RunnerSetStatusInactive = "inactive"
	RunnerSetStatusDeleted  = "deleted"
)

