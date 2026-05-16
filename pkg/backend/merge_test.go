package backend

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/proto"
)

func TestPerformMerge_FastForward(t *testing.T) {
	ctx, be := setupBackend(t)
	mkUserRepoWithLinearHistory(t, be, "demo", "main", "feature/x") // feature/x is ahead of main

	res, err := be.performMerge(ctx, "demo", "feature/x", "main", "merger", "Merge feature/x", "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.FastForward {
		t.Fatalf("expected fast-forward, got merge commit")
	}
	if res.MergeCommitSha == "" {
		t.Fatal("merge commit sha must be non-empty")
	}
}

func TestPerformMerge_TrueMerge(t *testing.T) {
	ctx, be := setupBackend(t)
	mkUserRepoWithDivergedHistory(t, be, "demo", "main", "feature/x", false /* no conflict */)

	res, err := be.performMerge(ctx, "demo", "feature/x", "main", "merger", "Merge feature/x", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.FastForward {
		t.Fatalf("expected merge commit, got fast-forward")
	}
}

func TestPerformMerge_Conflict(t *testing.T) {
	ctx, be := setupBackend(t)
	mkUserRepoWithDivergedHistory(t, be, "demo", "main", "feature/x", true /* conflict */)

	_, err := be.performMerge(ctx, "demo", "feature/x", "main", "merger", "Merge feature/x", "")
	if !errors.Is(err, errMergeConflict) {
		t.Fatalf("want errMergeConflict, got %v", err)
	}
}

func TestPerformMerge_NothingToMerge(t *testing.T) {
	ctx, be := setupBackend(t)
	// Linear history: base 'main', then 'feature/x' ahead of main.
	mkUserRepoWithLinearHistory(t, be, "demo", "main", "feature/x")
	// Now advance main to feature/x's tip so feature/x becomes an ancestor of main.
	gitDir := filepath.Join(be.cfg.DataPath, "repos", "demo.git")
	tip := strings.TrimSpace(runGitForTest(t, gitDir, "rev-parse", "refs/heads/feature/x"))
	runGitForTest(t, gitDir, "update-ref", "refs/heads/main", tip)

	_, err := be.performMerge(ctx, "demo", "feature/x", "main", "merger", "Merge", "")
	if !errors.Is(err, errNothingToMerge) {
		t.Fatalf("want errNothingToMerge, got %v", err)
	}
}

// mkUserRepoWithLinearHistory creates owner+repo and seeds a linear history:
// 'base' has one root commit; 'ahead' is a child of that root commit.
// 'ahead' is strictly ahead of 'base' (FF possible from base to ahead).
func mkUserRepoWithLinearHistory(t *testing.T, be *Backend, repo, base, ahead string) {
	t.Helper()
	owner, err := be.CreateUser(context.TODO(), "owner-merge", proto.UserOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatal(err)
	}
	if owner == nil {
		owner, _ = be.User(context.TODO(), "owner-merge")
	}
	ownerCtx := proto.WithUserContext(context.TODO(), owner)
	if _, err := be.CreateRepository(ownerCtx, repo, owner, proto.RepositoryOptions{}); err != nil {
		t.Fatal(err)
	}

	gitDir := filepath.Join(be.cfg.DataPath, "repos", repo+".git")
	root := writeRootCommit(t, gitDir, "root", "README", "hello\n")
	runGitForTest(t, gitDir, "update-ref", "refs/heads/"+base, root)
	child := writeChildCommit(t, gitDir, root, "feature commit", "README", "hello\nfeature\n")
	runGitForTest(t, gitDir, "update-ref", "refs/heads/"+ahead, child)
}

// mkUserRepoWithDivergedHistory creates owner+repo and seeds a Y-shaped history:
// shared root; 'base' adds one commit; 'branch' adds a different commit.
// If conflict=true, both diverging commits modify the same line of the same file.
func mkUserRepoWithDivergedHistory(t *testing.T, be *Backend, repo, base, branch string, conflict bool) {
	t.Helper()
	owner, err := be.CreateUser(context.TODO(), "owner-merge", proto.UserOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatal(err)
	}
	if owner == nil {
		owner, _ = be.User(context.TODO(), "owner-merge")
	}
	ownerCtx := proto.WithUserContext(context.TODO(), owner)
	if _, err := be.CreateRepository(ownerCtx, repo, owner, proto.RepositoryOptions{}); err != nil {
		t.Fatal(err)
	}

	gitDir := filepath.Join(be.cfg.DataPath, "repos", repo+".git")
	root := writeRootCommit(t, gitDir, "root", "README", "shared\n")

	baseCommit := writeChildCommit(t, gitDir, root, "base change", "README", "shared\nbase-line\n")
	runGitForTest(t, gitDir, "update-ref", "refs/heads/"+base, baseCommit)

	if !conflict {
		// Conflict-free: add a new file while keeping README from root unchanged.
		// We must build a 2-file tree explicitly; writeChildCommit only creates a
		// single-file tree and would "delete" README, causing a modify/delete conflict.
		readmeBlob := pipeIntoGit(t, gitDir, "shared\n", "hash-object", "-w", "--stdin")
		otherBlob := pipeIntoGit(t, gitDir, "branch only\n", "hash-object", "-w", "--stdin")
		treeIn := "100644 blob " + strings.TrimSpace(otherBlob) + "\tOTHER\n" +
			"100644 blob " + strings.TrimSpace(readmeBlob) + "\tREADME\n"
		tree := pipeIntoGit(t, gitDir, treeIn, "mktree")
		branchCommit := pipeIntoGit(t, gitDir, "branch change\n", "commit-tree", strings.TrimSpace(tree), "-p", root)
		runGitForTest(t, gitDir, "update-ref", "refs/heads/"+branch, strings.TrimSpace(branchCommit))
		return
	}
	// Conflict: same file as base, different second line.
	branchCommit := writeChildCommit(t, gitDir, root, "branch change", "README", "shared\nbranch-line\n")
	runGitForTest(t, gitDir, "update-ref", "refs/heads/"+branch, branchCommit)
}
