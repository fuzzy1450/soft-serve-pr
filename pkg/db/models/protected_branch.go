package models

import "time"

// ProtectedBranch represents a branch pattern that requires explicit grants to update.
type ProtectedBranch struct {
	ID            int64     `db:"id"`
	RepoID        int64     `db:"repo_id"`
	BranchPattern string    `db:"branch_pattern"`
	CreatedAt     time.Time `db:"created_at"`
}
