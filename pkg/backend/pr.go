package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/charmbracelet/soft-serve/pkg/proto"
	"github.com/charmbracelet/soft-serve/pkg/utils"
)

// OpenPR opens a new pull request.
func (d *Backend) OpenPR(ctx context.Context, repo, source, target, title, body string) (models.PullRequest, error) {
	repo = utils.SanitizeRepo(repo)
	if source == target {
		return models.PullRequest{}, proto.ErrPRSameBranch
	}

	r, err := d.Repository(ctx, repo)
	if err != nil {
		return models.PullRequest{}, err
	}
	gr, err := r.Open()
	if err != nil {
		return models.PullRequest{}, err
	}
	if !gr.HasBranch(source) {
		return models.PullRequest{}, proto.ErrPRBranchMissing
	}
	if !gr.HasBranch(target) {
		return models.PullRequest{}, proto.ErrPRBranchMissing
	}

	creator := proto.UserFromContext(ctx)
	if creator == nil {
		return models.PullRequest{}, proto.ErrUnauthorized
	}

	var pr models.PullRequest
	err = d.db.TransactionContext(ctx, func(tx *db.Tx) error {
		var ierr error
		pr, ierr = d.store.CreatePR(ctx, tx, repo, creator.Username(), source, target, title, body)
		return ierr
	})
	if err != nil {
		return models.PullRequest{}, db.WrapError(err)
	}
	return pr, nil
}

// GetPR returns a PR by number.
func (d *Backend) GetPR(ctx context.Context, repo string, number int64) (models.PullRequest, error) {
	repo = utils.SanitizeRepo(repo)
	var pr models.PullRequest
	err := d.db.TransactionContext(ctx, func(tx *db.Tx) error {
		var ierr error
		pr, ierr = d.store.GetPRByNumber(ctx, tx, repo, number)
		return ierr
	})
	if err != nil {
		err = db.WrapError(err)
		if errors.Is(err, db.ErrRecordNotFound) {
			return models.PullRequest{}, proto.ErrPRNotFound
		}
		return models.PullRequest{}, err
	}
	return pr, nil
}

// ListPRs lists PRs for a repo, optionally filtered by status.
func (d *Backend) ListPRs(ctx context.Context, repo string, status *models.PRStatus) ([]models.PullRequest, error) {
	repo = utils.SanitizeRepo(repo)
	var rows []models.PullRequest
	err := d.db.TransactionContext(ctx, func(tx *db.Tx) error {
		var ierr error
		rows, ierr = d.store.ListPRsByRepo(ctx, tx, repo, status)
		return ierr
	})
	return rows, db.WrapError(err)
}

// ClosePR closes (abandons) an open PR. Caller-side auth lives in the SSH
// command layer (Task 13).
func (d *Backend) ClosePR(ctx context.Context, repo string, number int64) error {
	pr, err := d.GetPR(ctx, repo, number)
	if err != nil {
		return err
	}
	if pr.Status != models.PRStatusOpen {
		return proto.ErrPRNotOpen
	}
	return db.WrapError(d.db.TransactionContext(ctx, func(tx *db.Tx) error {
		return d.store.SetPRStatusClosed(ctx, tx, pr.ID)
	}))
}

// MergePR merges an open PR's source into target. Caller-side auth (i.e.,
// "does the merger have ReadWriteAccess on target?") lives in the SSH command
// layer (Task 14) so that direct backend invocation in tests is unencumbered.
func (d *Backend) MergePR(ctx context.Context, repo string, number int64) (models.PullRequest, error) {
	pr, err := d.GetPR(ctx, repo, number)
	if err != nil {
		return models.PullRequest{}, err
	}
	if pr.Status != models.PRStatusOpen {
		return models.PullRequest{}, proto.ErrPRNotOpen
	}

	merger := proto.UserFromContext(ctx)
	if merger == nil {
		return models.PullRequest{}, proto.ErrUnauthorized
	}

	msg := fmt.Sprintf("Merge pull request #%d: %s", pr.Number, pr.Title)
	if pr.Body != "" {
		msg += "\n\n" + pr.Body
	}

	res, err := d.performMerge(ctx, repo, pr.SourceBranch, pr.TargetBranch, merger.Username(), msg, "")
	if err != nil {
		return models.PullRequest{}, err
	}

	if err := db.WrapError(d.db.TransactionContext(ctx, func(tx *db.Tx) error {
		return d.store.SetPRStatusMerged(ctx, tx, pr.ID, res.MergeCommitSha)
	})); err != nil {
		return models.PullRequest{}, err
	}

	return d.GetPR(ctx, repo, pr.Number)
}
