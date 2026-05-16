package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/migrate"
)

func setupDB(t *testing.T) (context.Context, *db.DB) {
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
	return ctx, dbx
}

func seedUserAndRepo(t *testing.T, ctx context.Context, dbx *db.DB, username, reponame string) {
	t.Helper()
	err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		if _, err := tx.ExecContext(ctx, tx.Rebind(
			`INSERT INTO users (username, admin, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`),
			username, false); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, tx.Rebind(
			`INSERT INTO repos (name, project_name, description, private, mirror, hidden, user_id, updated_at)
			 VALUES (?, ?, '', false, false, false, (SELECT id FROM users WHERE username = ?), CURRENT_TIMESTAMP)`),
			reponame, reponame, username); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBranchCollab_AddAndList(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")
	seedUserAndRepo(t, ctx, dbx, "bob", "other") // separate; should not appear

	s := &branchCollabStore{}

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		return s.AddBranchCollab(ctx, tx, "alice", "demo", "feature/*", access.ReadWriteAccess)
	}); err != nil {
		t.Fatalf("AddBranchCollab: %v", err)
	}

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		rows, err := s.ListBranchCollabsByRepo(ctx, tx, "demo")
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("want 1 row, got %d", len(rows))
		}
		if rows[0].BranchPattern != "feature/*" {
			t.Fatalf("want pattern feature/*, got %q", rows[0].BranchPattern)
		}
		if rows[0].AccessLevel != access.ReadWriteAccess {
			t.Fatalf("want ReadWriteAccess, got %v", rows[0].AccessLevel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBranchCollab_Remove(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")

	s := &branchCollabStore{}
	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		return s.AddBranchCollab(ctx, tx, "alice", "demo", "feature/*", access.ReadWriteAccess)
	}); err != nil {
		t.Fatal(err)
	}

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		return s.RemoveBranchCollab(ctx, tx, "alice", "demo", "feature/*")
	}); err != nil {
		t.Fatalf("RemoveBranchCollab: %v", err)
	}

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		rows, err := s.ListBranchCollabsByRepo(ctx, tx, "demo")
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatalf("want 0 rows after delete, got %d", len(rows))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBranchCollab_ListForUserAndRepo(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")
	seedUserAndRepo(t, ctx, dbx, "bob", "demo2")

	s := &branchCollabStore{}
	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		if err := s.AddBranchCollab(ctx, tx, "alice", "demo", "feature/*", access.ReadWriteAccess); err != nil {
			return err
		}
		if err := s.AddBranchCollab(ctx, tx, "alice", "demo", "hotfix/*", access.ReadWriteAccess); err != nil {
			return err
		}
		// Different repo — must not appear in result.
		return s.AddBranchCollab(ctx, tx, "bob", "demo2", "feature/*", access.ReadWriteAccess)
	}); err != nil {
		t.Fatal(err)
	}

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		rows, err := s.ListBranchCollabsForUserAndRepo(ctx, tx, "alice", "demo")
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("want 2 rows, got %d", len(rows))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
