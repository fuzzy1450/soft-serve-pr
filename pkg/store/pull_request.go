package store

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
)

// PullRequestStore manages pull requests.
type PullRequestStore interface {
	CreatePR(ctx context.Context, h db.Handler, repo, creator, source, target, title, body string) (models.PullRequest, error)
	GetPRByNumber(ctx context.Context, h db.Handler, repo string, number int64) (models.PullRequest, error)
	ListPRsByRepo(ctx context.Context, h db.Handler, repo string, status *models.PRStatus) ([]models.PullRequest, error)
	SetPRStatusMerged(ctx context.Context, h db.Handler, prID int64, mergeCommitSha string) error
	SetPRStatusClosed(ctx context.Context, h db.Handler, prID int64) error
}
