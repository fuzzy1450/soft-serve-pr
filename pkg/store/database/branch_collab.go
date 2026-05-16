package database

import (
	"context"
	"strings"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/charmbracelet/soft-serve/pkg/store"
	"github.com/charmbracelet/soft-serve/pkg/utils"
)

type branchCollabStore struct{}

var _ store.BranchCollabStore = (*branchCollabStore)(nil)

// AddBranchCollab implements store.BranchCollabStore.
func (*branchCollabStore) AddBranchCollab(ctx context.Context, tx db.Handler, username, repo, pattern string, level access.AccessLevel) error {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}
	repo = utils.SanitizeRepo(repo)

	query := tx.Rebind(`
		INSERT INTO branch_collabs (user_id, repo_id, branch_pattern, access_level, updated_at)
		VALUES (
			(SELECT id FROM users WHERE username = ?),
			(SELECT id FROM repos WHERE name = ?),
			?, ?, CURRENT_TIMESTAMP
		)`)
	_, err := tx.ExecContext(ctx, query, username, repo, pattern, level)
	return err
}

// RemoveBranchCollab implements store.BranchCollabStore.
func (*branchCollabStore) RemoveBranchCollab(ctx context.Context, tx db.Handler, username, repo, pattern string) error {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return err
	}
	repo = utils.SanitizeRepo(repo)

	query := tx.Rebind(`
		DELETE FROM branch_collabs
		WHERE
			user_id = (SELECT id FROM users WHERE username = ?)
			AND repo_id = (SELECT id FROM repos WHERE name = ?)
			AND branch_pattern = ?`)
	_, err := tx.ExecContext(ctx, query, username, repo, pattern)
	return err
}

// ListBranchCollabsByRepo implements store.BranchCollabStore.
func (*branchCollabStore) ListBranchCollabsByRepo(ctx context.Context, tx db.Handler, repo string) ([]models.BranchCollab, error) {
	repo = utils.SanitizeRepo(repo)
	var rows []models.BranchCollab
	query := tx.Rebind(`
		SELECT bc.*
		FROM branch_collabs bc
		INNER JOIN repos r ON r.id = bc.repo_id
		WHERE r.name = ?
		ORDER BY bc.id`)
	err := tx.SelectContext(ctx, &rows, query, repo)
	return rows, err
}

// ListBranchCollabsForUserAndRepo implements store.BranchCollabStore.
func (*branchCollabStore) ListBranchCollabsForUserAndRepo(ctx context.Context, tx db.Handler, username, repo string) ([]models.BranchCollab, error) {
	username = strings.ToLower(username)
	if err := utils.ValidateUsername(username); err != nil {
		return nil, err
	}
	repo = utils.SanitizeRepo(repo)
	var rows []models.BranchCollab
	query := tx.Rebind(`
		SELECT bc.*
		FROM branch_collabs bc
		INNER JOIN users u ON u.id = bc.user_id
		INNER JOIN repos r ON r.id = bc.repo_id
		WHERE u.username = ? AND r.name = ?
		ORDER BY bc.id`)
	err := tx.SelectContext(ctx, &rows, query, username, repo)
	return rows, err
}
