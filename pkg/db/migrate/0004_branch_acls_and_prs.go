package migrate

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/db"
)

const (
	branchAclsAndPrsName    = "branch acls and prs"
	branchAclsAndPrsVersion = 4
)

var branchAclsAndPrs = Migration{
	Name:    branchAclsAndPrsName,
	Version: branchAclsAndPrsVersion,
	Migrate: func(ctx context.Context, tx *db.Tx) error {
		return migrateUp(ctx, tx, branchAclsAndPrsVersion, branchAclsAndPrsName)
	},
	Rollback: func(ctx context.Context, tx *db.Tx) error {
		return migrateDown(ctx, tx, branchAclsAndPrsVersion, branchAclsAndPrsName)
	},
}
