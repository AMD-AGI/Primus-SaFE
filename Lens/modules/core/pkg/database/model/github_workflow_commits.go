package model

import (
	"time"
)

const TableNameGithubWorkflowCommits = "github_workflow_commits"

// GithubWorkflowCommits represents commit details fetched from GitHub API
type GithubWorkflowCommits struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RunID          int64     `gorm:"column:run_id;not null" json:"run_id"`
	SHA            string    `gorm:"column:sha;not null" json:"sha"`
	Message        string    `gorm:"column:message" json:"message"`
	AuthorName     string    `gorm:"column:author_name" json:"author_name"`
	AuthorEmail    string    `gorm:"column:author_email" json:"author_email"`
	AuthorDate     time.Time `gorm:"column:author_date" json:"author_date"`
	CommitterName  string    `gorm:"column:committer_name" json:"committer_name"`
	CommitterEmail string    `gorm:"column:committer_email" json:"committer_email"`
	CommitterDate  time.Time `gorm:"column:committer_date" json:"committer_date"`
	Additions      int       `gorm:"column:additions;not null;default:0" json:"additions"`
	Deletions      int       `gorm:"column:deletions;not null;default:0" json:"deletions"`
	FilesChanged   int       `gorm:"column:files_changed;not null;default:0" json:"files_changed"`
	ParentSHAs     ExtJSON   `gorm:"column:parent_shas;not null;default:[]" json:"parent_shas"`
	Files          ExtJSON   `gorm:"column:files" json:"files"`
	HTMLURL        string    `gorm:"column:html_url" json:"html_url"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

func (*GithubWorkflowCommits) TableName() string {
	return TableNameGithubWorkflowCommits
}
