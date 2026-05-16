package backend

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/hooks"
	"github.com/charmbracelet/soft-serve/pkg/proto"
)

// TestE2E_PRFlow exercises the complete pull-request workflow against a real
// sqlite-backed backend without starting an SSH server:
//
//  1. Owner creates repo; Carol gets a branch grant on feature/*.
//  2. Main is protected.
//  3. Carol pushes to feature/x (Update hook) → allowed.
//  4. Carol pushes to main (Update hook) → rejected; stderr mentions "protected".
//  5. Carol opens a PR: feature/x → main.
//  6. BranchAccessLevelForUser confirms Carol lacks write on main (i.e. the SSH
//     layer would block her from invoking MergePR). The backend method itself
//     does not enforce this — that is intentional per the spec (auth lives in
//     the SSH command layer, Task 14).
//  7. Owner merges the PR → succeeds; MergeCommitSha is set.
//  8. A second merge attempt → ErrPRNotOpen.
func TestE2E_PRFlow(t *testing.T) {
	ctx, be := setupBackend(t)

	// mkUserRepoWithLinearHistory creates "owner-merge" + "demo" repo with a
	// real git history: main ← feature/x (fast-forward possible).
	mkUserRepoWithLinearHistory(t, be, "demo", "main", "feature/x")

	owner, err := be.User(ctx, "owner-merge")
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx := hookCtx(ctx, be, owner)

	carol, err := be.CreateUser(ctx, "carol", proto.UserOptions{})
	if err != nil {
		t.Fatal(err)
	}
	carolCtx := hookCtx(ctx, be, carol)

	// Owner: protect main and grant Carol push rights on feature/*.
	if err := be.ProtectBranch(ownerCtx, "demo", "main"); err != nil {
		t.Fatal(err)
	}
	if err := be.AddBranchCollab(ownerCtx, "demo", "carol", "feature/*", access.ReadWriteAccess); err != nil {
		t.Fatal(err)
	}

	// ── Step 3: Carol pushes to feature/x ────────────────────────────────────
	t.Setenv("SOFT_SERVE_USERNAME", "carol")
	if err := be.Update(carolCtx, nil, &bytes.Buffer{}, "demo", hooks.HookArg{
		RefName: "refs/heads/feature/x",
		OldSha:  "0000000000000000000000000000000000000000",
		NewSha:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}); err != nil {
		t.Fatalf("carol push to feature/x: expected success, got %v", err)
	}

	// ── Step 4: Carol pushes to main → rejected ───────────────────────────────
	var rejectBuf bytes.Buffer
	if err := be.Update(carolCtx, nil, &rejectBuf, "demo", hooks.HookArg{
		RefName: "refs/heads/main",
		OldSha:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		NewSha:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}); err == nil {
		t.Fatal("carol push to main: expected rejection, got success")
	}
	if !strings.Contains(rejectBuf.String(), "protected") {
		t.Fatalf("rejection stderr does not mention protection: %q", rejectBuf.String())
	}

	// ── Step 5: Carol opens a PR ──────────────────────────────────────────────
	pr, err := be.OpenPR(carolCtx, "demo", "feature/x", "main", "Add x", "")
	if err != nil {
		t.Fatal(err)
	}

	// ── Step 6: Carol lacks write access on main (SSH layer would block her) ──
	// MergePR itself does not enforce target write access — that is the SSH
	// command layer's responsibility (Task 14). We verify the access level
	// directly to confirm the gatekeeping logic is in place.
	carolMainLvl := be.BranchAccessLevelForUser(ctx, "demo", carol, "refs/heads/main")
	if carolMainLvl >= access.ReadWriteAccess {
		t.Fatalf("carol should NOT have write access on main; got %v", carolMainLvl)
	}

	// ── Step 7: Owner merges the PR ───────────────────────────────────────────
	merged, err := be.MergePR(ownerCtx, "demo", pr.Number)
	if err != nil {
		t.Fatal(err)
	}
	if !merged.MergeCommitSha.Valid {
		t.Fatal("merge commit sha must be set after merge")
	}

	// ── Step 8: Second merge → ErrPRNotOpen ──────────────────────────────────
	if _, err := be.MergePR(ownerCtx, "demo", pr.Number); !errors.Is(err, proto.ErrPRNotOpen) {
		t.Fatalf("want ErrPRNotOpen on second merge attempt, got %v", err)
	}
}
