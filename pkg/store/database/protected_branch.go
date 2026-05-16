package database

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/charmbracelet/soft-serve/pkg/store"
	"github.com/charmbracelet/soft-serve/pkg/utils"
)

type protectedBranchStore struct{}

var _ store.ProtectedBranchStore = (*protectedBranchStore)(nil)

// AddProtectedBranch implements store.ProtectedBranchStore.
func (*protectedBranchStore) AddProtectedBranch(ctx context.Context, tx db.Handler, repo, pattern string) error {
	repo = utils.SanitizeRepo(repo)
	query := tx.Rebind(`
		INSERT INTO protected_branches (repo_id, branch_pattern)
		VALUES ((SELECT id FROM repos WHERE name = ?), ?)`)
	_, err := tx.ExecContext(ctx, query, repo, pattern)
	return err
}

// RemoveProtectedBranch implements store.ProtectedBranchStore.
func (*protectedBranchStore) RemoveProtectedBranch(ctx context.Context, tx db.Handler, repo, pattern string) error {
	repo = utils.SanitizeRepo(repo)
	query := tx.Rebind(`
		DELETE FROM protected_branches
		WHERE
			repo_id = (SELECT id FROM repos WHERE name = ?)
			AND branch_pattern = ?`)
	_, err := tx.ExecContext(ctx, query, repo, pattern)
	return err
}

// ListProtectedBranchesByRepo implements store.ProtectedBranchStore.
func (*protectedBranchStore) ListProtectedBranchesByRepo(ctx context.Context, tx db.Handler, repo string) ([]models.ProtectedBranch, error) {
	repo = utils.SanitizeRepo(repo)
	var rows []models.ProtectedBranch
	query := tx.Rebind(`
		SELECT pb.*
		FROM protected_branches pb
		INNER JOIN repos r ON r.id = pb.repo_id
		WHERE r.name = ?
		ORDER BY pb.id`)
	err := tx.SelectContext(ctx, &rows, query, repo)
	return rows, err
}
