package backend

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/migrate"
	"github.com/charmbracelet/soft-serve/pkg/proto"
	"github.com/charmbracelet/soft-serve/pkg/store/database"
)

// setupBackend builds an in-memory-ish sqlite-backed *Backend for tests.
// `pkg/db/internal/test.OpenSqlite` is not importable from here (Go's
// internal-package rules restrict it to pkg/db/...); the open logic is
// inlined.
func setupBackend(t *testing.T) (context.Context, *Backend) {
	t.Helper()
	ctx := config.WithContext(context.TODO(), config.DefaultConfig())
	dbpath := filepath.Join(t.TempDir(), "test.db")
	dbx, err := db.Open(ctx, "sqlite", dbpath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dbx.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := migrate.Migrate(ctx, dbx); err != nil {
		t.Fatal(err)
	}
	st := database.New(ctx, dbx)
	be := New(ctx, config.DefaultConfig(), dbx, st)
	return ctx, be
}

type aclCase struct {
	name             string
	isAdmin          bool
	isOwner          bool
	repoLevel        access.AccessLevel // simulated existing collab level on the repo
	branchPattern    string             // empty = no branch grant
	branchLevel      access.AccessLevel
	protectPattern   string // empty = unprotected
	refName          string
	wantWriteAllowed bool // i.e. effective level >= ReadWriteAccess
	wantReadAllowed  bool // i.e. effective level >= ReadOnlyAccess
}

func TestBranchAccessLevelForUser_Matrix(t *testing.T) {
	cases := []aclCase{
		// Admins always win.
		{name: "admin_protected_no_grant", isAdmin: true, protectPattern: "main", refName: "refs/heads/main", wantWriteAllowed: true, wantReadAllowed: true},
		// Owner is admin-equivalent for their repo.
		{name: "owner_protected_no_grant", isOwner: true, protectPattern: "main", refName: "refs/heads/main", wantWriteAllowed: true, wantReadAllowed: true},

		// Repo-only access, unprotected branch.
		{name: "repo_rw_unprotected", repoLevel: access.ReadWriteAccess, refName: "refs/heads/feature/x", wantWriteAllowed: true, wantReadAllowed: true},
		{name: "repo_ro_unprotected", repoLevel: access.ReadOnlyAccess, refName: "refs/heads/feature/x", wantWriteAllowed: false, wantReadAllowed: true},
		{name: "repo_none_unprotected", repoLevel: access.NoAccess, refName: "refs/heads/feature/x", wantWriteAllowed: false, wantReadAllowed: false},

		// Repo-level write, protected branch, no grant -> write denied; read allowed (repoLvl).
		{name: "repo_rw_protected_no_grant", repoLevel: access.ReadWriteAccess, protectPattern: "main", refName: "refs/heads/main", wantWriteAllowed: false, wantReadAllowed: true},

		// Branch grant promotes to write on unprotected branch.
		{name: "no_repo_branch_grant_unprotected", repoLevel: access.NoAccess, branchPattern: "feature/*", branchLevel: access.ReadWriteAccess, refName: "refs/heads/feature/x", wantWriteAllowed: true, wantReadAllowed: true},
		// Glob does not match the ref -> grant inert.
		{name: "branch_grant_pattern_no_match", repoLevel: access.NoAccess, branchPattern: "hotfix/*", branchLevel: access.ReadWriteAccess, refName: "refs/heads/feature/x", wantWriteAllowed: false, wantReadAllowed: false},

		// Protected branch + matching grant -> write allowed.
		{name: "protected_with_grant", repoLevel: access.NoAccess, branchPattern: "main", branchLevel: access.ReadWriteAccess, protectPattern: "main", refName: "refs/heads/main", wantWriteAllowed: true, wantReadAllowed: true},

		// Protected branch with grant on different pattern -> write denied.
		{name: "protected_grant_other_branch", repoLevel: access.NoAccess, branchPattern: "feature/*", branchLevel: access.ReadWriteAccess, protectPattern: "main", refName: "refs/heads/main", wantWriteAllowed: false, wantReadAllowed: false},

		// Read-only repo + matching grant on protected branch -> write allowed (grant additive), read allowed.
		{name: "repo_ro_protected_grant", repoLevel: access.ReadOnlyAccess, branchPattern: "main", branchLevel: access.ReadWriteAccess, protectPattern: "main", refName: "refs/heads/main", wantWriteAllowed: true, wantReadAllowed: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, be := setupBackend(t)
			// Seed user, repo, optional collab, optional branch grant, optional protected branch.
			seedAclScenario(t, ctx, be, tc)

			user, _ := be.User(ctx, "alice")
			lvl := be.BranchAccessLevelForUser(ctx, "demo", user, tc.refName)

			gotWrite := lvl >= access.ReadWriteAccess
			gotRead := lvl >= access.ReadOnlyAccess
			if gotWrite != tc.wantWriteAllowed {
				t.Errorf("write: got %v, want %v (lvl=%v)", gotWrite, tc.wantWriteAllowed, lvl)
			}
			if gotRead != tc.wantReadAllowed {
				t.Errorf("read: got %v, want %v (lvl=%v)", gotRead, tc.wantReadAllowed, lvl)
			}
		})
	}
}

// seedAclScenario sets up the test fixture per case. Lives in this test file.
func seedAclScenario(t *testing.T, ctx context.Context, be *Backend, tc aclCase) {
	t.Helper()

	// Create owner "owner" and target user "alice" (the user being checked).
	if _, err := be.CreateUser(ctx, "owner", proto.UserOptions{Admin: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := be.CreateUser(ctx, "alice", proto.UserOptions{Admin: tc.isAdmin}); err != nil {
		t.Fatal(err)
	}

	// Create the repo under owner. If isOwner is true, switch alice to the owner.
	ownerCtx := proto.WithUserContext(ctx, mustUser(t, ctx, be, "owner"))
	if _, err := be.CreateRepository(ownerCtx, "demo", mustUser(t, ctx, be, "owner"), proto.RepositoryOptions{}); err != nil {
		t.Fatal(err)
	}
	if tc.isOwner {
		// Re-own to alice via direct SQL since we don't have a public setter handy.
		_ = be.db.TransactionContext(ctx, func(tx *db.Tx) error {
			_, err := tx.ExecContext(ctx, tx.Rebind(
				`UPDATE repos SET user_id = (SELECT id FROM users WHERE username = ?) WHERE name = ?`),
				"alice", "demo")
			return err
		})
	}

	if tc.repoLevel > access.NoAccess && !tc.isOwner {
		if err := be.AddCollaborator(ctx, "demo", "alice", tc.repoLevel); err != nil {
			t.Fatal(err)
		}
	}
	if tc.branchPattern != "" {
		if err := be.AddBranchCollab(ctx, "demo", "alice", tc.branchPattern, tc.branchLevel); err != nil {
			t.Fatal(err)
		}
	}
	if tc.protectPattern != "" {
		if err := be.ProtectBranch(ctx, "demo", tc.protectPattern); err != nil {
			t.Fatal(err)
		}
	}
}

func mustUser(t *testing.T, ctx context.Context, be *Backend, username string) proto.User {
	t.Helper()
	u, err := be.User(ctx, username)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
