package database

import (
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/db"
)

func TestProtectedBranch_AddAndList(t *testing.T) {
	ctx, dbx := setupDB(t) // helper from branch_collab_test.go
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")

	s := &protectedBranchStore{}
	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		return s.AddProtectedBranch(ctx, tx, "demo", "main")
	}); err != nil {
		t.Fatalf("AddProtectedBranch: %v", err)
	}

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		rows, err := s.ListProtectedBranchesByRepo(ctx, tx, "demo")
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].BranchPattern != "main" {
			t.Fatalf("unexpected rows: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedBranch_Remove(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")

	s := &protectedBranchStore{}
	_ = dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		return s.AddProtectedBranch(ctx, tx, "demo", "main")
	})
	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		return s.RemoveProtectedBranch(ctx, tx, "demo", "main")
	}); err != nil {
		t.Fatal(err)
	}
	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		rows, err := s.ListProtectedBranchesByRepo(ctx, tx, "demo")
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatalf("want 0 after remove, got %d", len(rows))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedBranch_DuplicateAddFails(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")
	s := &protectedBranchStore{}

	add := func() error {
		return dbx.TransactionContext(ctx, func(tx *db.Tx) error {
			return s.AddProtectedBranch(ctx, tx, "demo", "main")
		})
	}
	if err := add(); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := add(); err == nil {
		t.Fatal("want error on duplicate add, got nil")
	}
}
