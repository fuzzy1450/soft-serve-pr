package models

import (
	"time"

	"github.com/charmbracelet/soft-serve/pkg/access"
)

// BranchCollab represents a per-branch-pattern collaborator grant.
type BranchCollab struct {
	ID            int64              `db:"id"`
	UserID        int64              `db:"user_id"`
	RepoID        int64              `db:"repo_id"`
	BranchPattern string             `db:"branch_pattern"`
	AccessLevel   access.AccessLevel `db:"access_level"`
	CreatedAt     time.Time          `db:"created_at"`
	UpdatedAt     time.Time          `db:"updated_at"`
}
