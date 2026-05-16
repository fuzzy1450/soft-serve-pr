package store

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
)

// ProtectedBranchStore manages protected branch patterns per repo.
type ProtectedBranchStore interface {
	AddProtectedBranch(ctx context.Context, h db.Handler, repo, pattern string) error
	RemoveProtectedBranch(ctx context.Context, h db.Handler, repo, pattern string) error
	ListProtectedBranchesByRepo(ctx context.Context, h db.Handler, repo string) ([]models.ProtectedBranch, error)
}
