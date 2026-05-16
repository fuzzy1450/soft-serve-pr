package backend

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/charmbracelet/soft-serve/pkg/proto"
)

func TestOpenPR_HappyPath(t *testing.T) {
	ctx, be := setupBackend(t)
	owner, _ := be.CreateUser(ctx, "owner", proto.UserOptions{})
	ownerCtx := proto.WithUserContext(ctx, owner)
	if _, err := be.CreateRepository(ownerCtx, "demo", owner, proto.RepositoryOptions{}); err != nil {
		t.Fatal(err)
	}
	seedRepoWithBranches(t, be, "demo", []string{"main", "feature/x"})

	pr, err := be.OpenPR(ownerCtx, "demo", "feature/x", "main", "Add x", "")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 1 || pr.Status != models.PRStatusOpen {
		t.Fatalf("unexpected pr: %+v", pr)
	}
}

func TestOpenPR_RejectsSameBranch(t *testing.T) {
	ctx, be := setupBackend(t)
	owner, _ := be.CreateUser(ctx, "owner", proto.UserOptions{})
	ownerCtx := proto.WithUserContext(ctx, owner)
	_, _ = be.CreateRepository(ownerCtx, "demo", owner, proto.RepositoryOptions{})
	seedRepoWithBranches(t, be, "demo", []string{"main"})

	_, err := be.OpenPR(ownerCtx, "demo", "main", "main", "x", "")
	if !errors.Is(err, proto.ErrPRSameBranch) {
		t.Fatalf("want ErrPRSameBranch, got %v", err)
	}
}

func TestOpenPR_RejectsMissingBranch(t *testing.T) {
	ctx, be := setupBackend(t)
	owner, _ := be.CreateUser(ctx, "owner", proto.UserOptions{})
	ownerCtx := proto.WithUserContext(ctx, owner)
	_, _ = be.CreateRepository(ownerCtx, "demo", owner, proto.RepositoryOptions{})
	seedRepoWithBranches(t, be, "demo", []string{"main"})

	_, err := be.OpenPR(ownerCtx, "demo", "nope", "main", "x", "")
	if !errors.Is(err, proto.ErrPRBranchMissing) {
		t.Fatalf("want ErrPRBranchMissing, got %v", err)
	}
}

func TestClosePR_TransitionsAndRefusesAgain(t *testing.T) {
	ctx, be := setupBackend(t)
	owner, _ := be.CreateUser(ctx, "owner", proto.UserOptions{})
	ownerCtx := proto.WithUserContext(ctx, owner)
	_, _ = be.CreateRepository(ownerCtx, "demo", owner, proto.RepositoryOptions{})
	seedRepoWithBranches(t, be, "demo", []string{"main", "feature/x"})

	pr, err := be.OpenPR(ownerCtx, "demo", "feature/x", "main", "x", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := be.ClosePR(ownerCtx, "demo", pr.Number); err != nil {
		t.Fatal(err)
	}
	got, _ := be.GetPR(ownerCtx, "demo", pr.Number)
	if got.Status != models.PRStatusClosed {
		t.Fatalf("want closed, got %v", got.Status)
	}
	if err := be.ClosePR(ownerCtx, "demo", pr.Number); !errors.Is(err, proto.ErrPRNotOpen) {
		t.Fatalf("want ErrPRNotOpen on second close, got %v", err)
	}
}

func TestListPRs_FiltersByStatus(t *testing.T) {
	ctx, be := setupBackend(t)
	owner, _ := be.CreateUser(ctx, "owner", proto.UserOptions{})
	ownerCtx := proto.WithUserContext(ctx, owner)
	_, _ = be.CreateRepository(ownerCtx, "demo", owner, proto.RepositoryOptions{})
	seedRepoWithBranches(t, be, "demo", []string{"main", "f1", "f2"})

	pr1, _ := be.OpenPR(ownerCtx, "demo", "f1", "main", "1", "")
	_, _ = be.OpenPR(ownerCtx, "demo", "f2", "main", "2", "")
	_ = be.ClosePR(ownerCtx, "demo", pr1.Number)

	all, _ := be.ListPRs(ownerCtx, "demo", nil)
	if len(all) != 2 {
		t.Fatalf("want 2 total, got %d", len(all))
	}
	open := models.PRStatusOpen
	openOnly, _ := be.ListPRs(ownerCtx, "demo", &open)
	if len(openOnly) != 1 {
		t.Fatalf("want 1 open, got %d", len(openOnly))
	}
}

func TestMergePR_HappyPath(t *testing.T) {
	ctx, be := setupBackend(t)
	mkUserRepoWithLinearHistory(t, be, "demo", "main", "feature/x")
	owner, _ := be.User(ctx, "owner-merge")
	ownerCtx := proto.WithUserContext(ctx, owner)

	pr, err := be.OpenPR(ownerCtx, "demo", "feature/x", "main", "Add x", "")
	if err != nil {
		t.Fatal(err)
	}
	merged, err := be.MergePR(ownerCtx, "demo", pr.Number)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != models.PRStatusMerged {
		t.Fatalf("want merged, got %v", merged.Status)
	}
	if !merged.MergeCommitSha.Valid {
		t.Fatal("merge_commit_sha must be set")
	}
}

func TestMergePR_NotOpen(t *testing.T) {
	ctx, be := setupBackend(t)
	mkUserRepoWithLinearHistory(t, be, "demo", "main", "feature/x")
	owner, _ := be.User(ctx, "owner-merge")
	ownerCtx := proto.WithUserContext(ctx, owner)

	pr, _ := be.OpenPR(ownerCtx, "demo", "feature/x", "main", "x", "")
	_ = be.ClosePR(ownerCtx, "demo", pr.Number)
	if _, err := be.MergePR(ownerCtx, "demo", pr.Number); !errors.Is(err, proto.ErrPRNotOpen) {
		t.Fatalf("want ErrPRNotOpen, got %v", err)
	}
}

// seedRepoWithBranches creates one commit per branch (each branch is a
// disconnected root with a unique blob). Uses shell-out to system git, which
// soft-serve already requires. The repo's bare git dir was created by
// be.CreateRepository.
func seedRepoWithBranches(t *testing.T, be *Backend, repo string, branches []string) {
	t.Helper()
	gitDir := filepath.Join(be.cfg.DataPath, "repos", repo+".git")
	for _, br := range branches {
		commit := writeRootCommit(t, gitDir, "init "+br, "README", "branch "+br+"\n")
		runGitForTest(t, gitDir, "update-ref", "refs/heads/"+br, commit)
	}
}

// writeRootCommit creates a single-file commit with no parent. Returns commit sha.
func writeRootCommit(t *testing.T, gitDir, message, filename, content string) string {
	t.Helper()
	blob := pipeIntoGit(t, gitDir, content, "hash-object", "-w", "--stdin")
	// mktree wants "<mode> <type> <sha>\t<name>\n"
	treeIn := "100644 blob " + strings.TrimSpace(blob) + "\t" + filename + "\n"
	tree := pipeIntoGit(t, gitDir, treeIn, "mktree")
	commit := pipeIntoGit(t, gitDir, message+"\n", "commit-tree", strings.TrimSpace(tree))
	return strings.TrimSpace(commit)
}

// writeChildCommit creates a single-file commit on top of parent. Returns commit sha.
func writeChildCommit(t *testing.T, gitDir, parent, message, filename, content string) string {
	t.Helper()
	blob := pipeIntoGit(t, gitDir, content, "hash-object", "-w", "--stdin")
	treeIn := "100644 blob " + strings.TrimSpace(blob) + "\t" + filename + "\n"
	tree := pipeIntoGit(t, gitDir, treeIn, "mktree")
	commit := pipeIntoGit(t, gitDir, message+"\n", "commit-tree", strings.TrimSpace(tree), "-p", parent)
	return strings.TrimSpace(commit)
}

func pipeIntoGit(t *testing.T, gitDir, stdin string, args ...string) string {
	t.Helper()
	full := append([]string{"--git-dir=" + gitDir}, args...)
	cmd := exec.Command("git", full...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func runGitForTest(t *testing.T, gitDir string, args ...string) string {
	t.Helper()
	full := append([]string{"--git-dir=" + gitDir}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

