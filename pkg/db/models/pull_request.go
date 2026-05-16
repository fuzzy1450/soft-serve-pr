package models

import (
	"database/sql"
	"time"
)

// PRStatus is the lifecycle status of a pull request.
type PRStatus int

const (
	PRStatusOpen   PRStatus = 0
	PRStatusMerged PRStatus = 1
	PRStatusClosed PRStatus = 2
)

func (s PRStatus) String() string {
	switch s {
	case PRStatusOpen:
		return "open"
	case PRStatusMerged:
		return "merged"
	case PRStatusClosed:
		return "closed"
	}
	return "unknown"
}

// PullRequest represents a same-repo pull request.
type PullRequest struct {
	ID             int64          `db:"id"`
	RepoID         int64          `db:"repo_id"`
	Number         int64          `db:"number"`
	CreatorID      int64          `db:"creator_id"`
	SourceBranch   string         `db:"source_branch"`
	TargetBranch   string         `db:"target_branch"`
	Title          string         `db:"title"`
	Body           string         `db:"body"`
	Status         PRStatus       `db:"status"`
	MergeCommitSha sql.NullString `db:"merge_commit_sha"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
	MergedAt       sql.NullTime   `db:"merged_at"`
	ClosedAt       sql.NullTime   `db:"closed_at"`
}
