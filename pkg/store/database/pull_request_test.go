package database

import (
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
)

func TestPullRequest_CreateAndGet(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")

	s := &pullRequestStore{}

	var created models.PullRequest
	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		var err error
		created, err = s.CreatePR(ctx, tx, "demo", "alice", "feature/x", "main", "Add x", "body text")
		return err
	}); err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if created.Number != 1 {
		t.Fatalf("want number 1, got %d", created.Number)
	}
	if created.Status != models.PRStatusOpen {
		t.Fatalf("want PRStatusOpen, got %v", created.Status)
	}

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		got, err := s.GetPRByNumber(ctx, tx, "demo", 1)
		if err != nil {
			return err
		}
		if got.Title != "Add x" {
			t.Fatalf("got title %q", got.Title)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPullRequest_SequentialNumbering(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")
	s := &pullRequestStore{}

	for i := int64(1); i <= 3; i++ {
		var created models.PullRequest
		err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
			var err error
			created, err = s.CreatePR(ctx, tx, "demo", "alice", "feature/x", "main", "t", "b")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if created.Number != i {
			t.Fatalf("iteration %d: want number %d, got %d", i, i, created.Number)
		}
	}
}

func TestPullRequest_SetMerged(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")
	s := &pullRequestStore{}

	var pr models.PullRequest
	_ = dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		var err error
		pr, err = s.CreatePR(ctx, tx, "demo", "alice", "feature/x", "main", "t", "")
		return err
	})

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		return s.SetPRStatusMerged(ctx, tx, pr.ID, "deadbeef")
	}); err != nil {
		t.Fatalf("SetPRStatusMerged: %v", err)
	}

	if err := dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		got, err := s.GetPRByNumber(ctx, tx, "demo", 1)
		if err != nil {
			return err
		}
		if got.Status != models.PRStatusMerged {
			t.Fatalf("want merged, got %v", got.Status)
		}
		if !got.MergeCommitSha.Valid || got.MergeCommitSha.String != "deadbeef" {
			t.Fatalf("merge_commit_sha not set: %+v", got.MergeCommitSha)
		}
		if !got.MergedAt.Valid {
			t.Fatalf("merged_at not set")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPullRequest_ListByStatus(t *testing.T) {
	ctx, dbx := setupDB(t)
	seedUserAndRepo(t, ctx, dbx, "alice", "demo")
	s := &pullRequestStore{}

	_ = dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		_, _ = s.CreatePR(ctx, tx, "demo", "alice", "f1", "main", "t1", "")
		pr2, _ := s.CreatePR(ctx, tx, "demo", "alice", "f2", "main", "t2", "")
		return s.SetPRStatusClosed(ctx, tx, pr2.ID)
	})

	open := models.PRStatusOpen
	_ = dbx.TransactionContext(ctx, func(tx *db.Tx) error {
		rows, err := s.ListPRsByRepo(ctx, tx, "demo", &open)
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("want 1 open PR, got %d", len(rows))
		}
		rows, err = s.ListPRsByRepo(ctx, tx, "demo", nil)
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("want 2 total PRs, got %d", len(rows))
		}
		return nil
	})
}
