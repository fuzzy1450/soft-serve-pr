package store

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
)

// BranchCollabStore manages per-branch collaborator grants.
type BranchCollabStore interface {
	AddBranchCollab(ctx context.Context, h db.Handler, username, repo, pattern string, level access.AccessLevel) error
	RemoveBranchCollab(ctx context.Context, h db.Handler, username, repo, pattern string) error
	ListBranchCollabsByRepo(ctx context.Context, h db.Handler, repo string) ([]models.BranchCollab, error)
	ListBranchCollabsForUserAndRepo(ctx context.Context, h db.Handler, username, repo string) ([]models.BranchCollab, error)
}
