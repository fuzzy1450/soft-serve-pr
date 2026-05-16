package database

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/charmbracelet/soft-serve/pkg/store"
	"github.com/charmbracelet/soft-serve/pkg/utils"
)

type pullRequestStore struct{}

var _ store.PullRequestStore = (*pullRequestStore)(nil)

// CreatePR implements store.PullRequestStore.
func (*pullRequestStore) CreatePR(ctx context.Context, tx db.Handler, repo, creator, source, target, title, body string) (models.PullRequest, error) {
	creator = strings.ToLower(creator)
	if err := utils.ValidateUsername(creator); err != nil {
		return models.PullRequest{}, err
	}
	repo = utils.SanitizeRepo(repo)

	// Allocate the next per-repo number atomically inside the transaction.
	var repoID int64
	if err := tx.GetContext(ctx, &repoID, tx.Rebind(`SELECT id FROM repos WHERE name = ?`), repo); err != nil {
		return models.PullRequest{}, err
	}

	var next int64
	row := tx.QueryRowxContext(ctx, tx.Rebind(
		`SELECT COALESCE(MAX(number), 0) + 1 FROM pull_requests WHERE repo_id = ?`), repoID)
	if err := row.Scan(&next); err != nil {
		return models.PullRequest{}, err
	}

	query := tx.Rebind(`
		INSERT INTO pull_requests
			(repo_id, number, creator_id, source_branch, target_branch, title, body, status, updated_at)
		VALUES
			(?,
			 ?,
			 (SELECT id FROM users WHERE username = ?),
			 ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`)
	if _, err := tx.ExecContext(ctx, query, repoID, next, creator, source, target, title, body, models.PRStatusOpen); err != nil {
		return models.PullRequest{}, err
	}

	var pr models.PullRequest
	if err := tx.GetContext(ctx, &pr, tx.Rebind(
		`SELECT * FROM pull_requests WHERE repo_id = ? AND number = ?`), repoID, next); err != nil {
		return models.PullRequest{}, err
	}
	return pr, nil
}

// GetPRByNumber implements store.PullRequestStore.
func (*pullRequestStore) GetPRByNumber(ctx context.Context, tx db.Handler, repo string, number int64) (models.PullRequest, error) {
	repo = utils.SanitizeRepo(repo)
	var pr models.PullRequest
	query := tx.Rebind(`
		SELECT pr.*
		FROM pull_requests pr
		INNER JOIN repos r ON r.id = pr.repo_id
		WHERE r.name = ? AND pr.number = ?`)
	err := tx.GetContext(ctx, &pr, query, repo, number)
	return pr, err
}

// ListPRsByRepo implements store.PullRequestStore.
func (*pullRequestStore) ListPRsByRepo(ctx context.Context, tx db.Handler, repo string, status *models.PRStatus) ([]models.PullRequest, error) {
	repo = utils.SanitizeRepo(repo)
	var rows []models.PullRequest

	if status == nil {
		query := tx.Rebind(`
			SELECT pr.*
			FROM pull_requests pr
			INNER JOIN repos r ON r.id = pr.repo_id
			WHERE r.name = ?
			ORDER BY pr.number DESC`)
		err := tx.SelectContext(ctx, &rows, query, repo)
		return rows, err
	}

	query := tx.Rebind(`
		SELECT pr.*
		FROM pull_requests pr
		INNER JOIN repos r ON r.id = pr.repo_id
		WHERE r.name = ? AND pr.status = ?
		ORDER BY pr.number DESC`)
	err := tx.SelectContext(ctx, &rows, query, repo, *status)
	return rows, err
}

// SetPRStatusMerged implements store.PullRequestStore.
func (*pullRequestStore) SetPRStatusMerged(ctx context.Context, tx db.Handler, prID int64, mergeCommitSha string) error {
	query := tx.Rebind(`
		UPDATE pull_requests
		SET status = ?, merge_commit_sha = ?, merged_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`)
	_, err := tx.ExecContext(ctx, query, models.PRStatusMerged, mergeCommitSha, time.Now(), time.Now(), prID, models.PRStatusOpen)
	return err
}

// SetPRStatusClosed implements store.PullRequestStore.
func (*pullRequestStore) SetPRStatusClosed(ctx context.Context, tx db.Handler, prID int64) error {
	query := tx.Rebind(`
		UPDATE pull_requests
		SET status = ?, closed_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`)
	_, err := tx.ExecContext(ctx, query, models.PRStatusClosed, time.Now(), time.Now(), prID, models.PRStatusOpen)
	return err
}
