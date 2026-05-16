package backend

import (
	"context"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/proto"
	"github.com/charmbracelet/soft-serve/pkg/utils"
	"github.com/gobwas/glob"
)

// BranchAccessLevelForUser returns the effective access level for a user on a
// specific ref within a repository. Combines repo-level access with
// branch-level grants and protected-branch policy. Reads are never blocked by
// protection; writes on protected branches require an explicit grant.
//
// refName must be the full ref name, e.g. "refs/heads/main".
func (d *Backend) BranchAccessLevelForUser(ctx context.Context, repo string, user proto.User, refName string) access.AccessLevel {
	repo = utils.SanitizeRepo(repo)
	branchName := shortBranch(refName)

	// 1. Server admin.
	if user != nil && user.IsAdmin() {
		return access.AdminAccess
	}

	// 2. Repo owner.
	r := proto.RepositoryFromContext(ctx)
	if r == nil {
		r, _ = d.Repository(ctx, repo)
	}
	if r != nil && user != nil && r.UserID() == user.ID() {
		return access.AdminAccess
	}

	// 3. Repo-level access.
	repoLvl := d.AccessLevelForUser(ctx, repo, user)

	// 4. Best matching branch grant (max level over matching rows).
	branchLvl := access.NoAccess
	if user != nil {
		grants := d.listBranchGrantsForUser(ctx, repo, user.Username())
		for _, g := range grants {
			gl, err := glob.Compile(g.BranchPattern)
			if err != nil {
				d.logger.Warn("invalid branch grant pattern", "pattern", g.BranchPattern, "err", err)
				continue
			}
			if gl.Match(branchName) {
				if g.AccessLevel > branchLvl {
					branchLvl = g.AccessLevel
				}
			}
		}
	}

	// 5. Protected check. Write requires an explicit grant; reads fold in repo-level access.
	if d.branchIsProtected(ctx, repo, branchName) {
		if branchLvl >= access.ReadWriteAccess {
			return branchLvl
		}
		// No write grant on a protected branch. If repo-level access grants any read,
		// preserve it (capped at ReadOnlyAccess so writes remain blocked).
		if repoLvl >= access.ReadOnlyAccess {
			return access.ReadOnlyAccess
		}
		return branchLvl
	}

	// 6. Unprotected: max(repo, branch).
	if branchLvl > repoLvl {
		return branchLvl
	}
	return repoLvl
}

func (d *Backend) listBranchGrantsForUser(ctx context.Context, repo, username string) []bcRow {
	var out []bcRow
	_ = d.db.TransactionContext(ctx, func(tx *db.Tx) error {
		rows, err := d.store.ListBranchCollabsForUserAndRepo(ctx, tx, username, repo)
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, bcRow{BranchPattern: r.BranchPattern, AccessLevel: r.AccessLevel})
		}
		return nil
	})
	return out
}

func (d *Backend) branchIsProtected(ctx context.Context, repo, branchName string) bool {
	var protected bool
	_ = d.db.TransactionContext(ctx, func(tx *db.Tx) error {
		rows, err := d.store.ListProtectedBranchesByRepo(ctx, tx, repo)
		if err != nil {
			return err
		}
		for _, p := range rows {
			gl, err := glob.Compile(p.BranchPattern)
			if err != nil {
				d.logger.Warn("invalid protected branch pattern", "pattern", p.BranchPattern, "err", err)
				continue
			}
			if gl.Match(branchName) {
				protected = true
				return nil
			}
		}
		return nil
	})
	return protected
}

type bcRow struct {
	BranchPattern string
	AccessLevel   access.AccessLevel
}

func shortBranch(refName string) string {
	const prefix = "refs/heads/"
	if len(refName) > len(prefix) && refName[:len(prefix)] == prefix {
		return refName[len(prefix):]
	}
	return refName
}
