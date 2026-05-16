# Pull Request System for soft-serve — Design

Date: 2026-05-15
Status: Approved (pending written-spec review)
Branch: `prs`

## Context

Soft-serve currently enforces access control strictly per-repo via four levels
(`NoAccess`, `ReadOnlyAccess`, `ReadWriteAccess`, `AdminAccess`) checked through
`AccessLevelForUser` in `pkg/backend/user.go:46`. A `collab` row binds
`(user, repo, level)` — there is no notion of per-branch access. The update hook
at `pkg/backend/hooks.go:Update()` receives `(user, repo, refName, oldSha, newSha)`
per pushed ref but currently logs only and enforces nothing at the branch level.

A self-hosted soft-serve operator wants to bring in contributors without
handing them total control of a repo. Today the only options are "read-only"
(can't contribute) or "read-write" (can push to any ref including `main`). The
goal of this feature is to fill that gap.

## Goals

- Let an admin grant a user push access to specific branches (or branch
  patterns) without granting repo-wide write access.
- Let an admin mark a branch as protected so direct pushes are rejected and
  changes must go through a PR merge.
- Provide a minimal PR lifecycle (open / merge / close) as the delivery
  mechanism for contributions into protected branches.
- Keep every existing soft-serve setup working unchanged until an admin opts
  in to the new features.

## Non-goals (v1, explicit)

- No code review surface: no comments, no approvals, no draft state, no
  line annotations, no reviewers.
- No fork-based PRs. Same-repo branch → branch only.
- No HTTP/JSON API endpoints for PRs in v1.
- No TUI page for PRs in v1.
- No merge strategy options. One strategy: fast-forward when possible,
  merge commit otherwise.
- No reopen of merged or closed PRs.
- No labels, milestones, assignees, CODEOWNERS routing.

If a future request feels like a step toward GitHub parity, that is the
signal it is out of scope for v1.

## Architecture

### 1. Auth model

Two new tables — branch grants (additive) and protected branches
(restrictive) — and one new helper function that combines them with existing
repo-level access.

#### Schema

```sql
CREATE TABLE branch_collabs (
    id              INTEGER PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id         INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    branch_pattern  TEXT    NOT NULL,    -- gobwas/glob, e.g. "feature/*"
    access_level    INTEGER NOT NULL,    -- reuses pkg/access.AccessLevel
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    UNIQUE(user_id, repo_id, branch_pattern)
);

CREATE TABLE protected_branches (
    id              INTEGER PRIMARY KEY,
    repo_id         INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    branch_pattern  TEXT    NOT NULL,    -- gobwas/glob
    created_at      DATETIME NOT NULL,
    UNIQUE(repo_id, branch_pattern)
);
```

Glob patterns use `github.com/gobwas/glob`, already vendored.

Only `ReadWriteAccess` is meaningful on a branch grant in v1. `ReadOnlyAccess`
on a branch is a no-op (reads are repo-level). `AdminAccess` on a branch is
disallowed at the command layer to keep semantics clear.

#### Decision function

New helper in `pkg/backend/user.go`:

```
BranchAccessLevelForUser(ctx, repo, user, refName) AccessLevel
```

1. `user.IsAdmin()` → `AdminAccess`.
2. `user == repo.Owner` → `AdminAccess`.
3. `repoLvl = AccessLevelForUser(ctx, repo, user)` (existing repo-level check).
4. `branchLvl = max(level)` over `branch_collabs` rows matching `(user, repo)`
   where any pattern glob-matches `refName`.
5. If any `protected_branches` pattern for this repo glob-matches `refName`:
     - Effective level = `branchLvl` (repo-level write is ignored on protected
       branches).
     - If `branchLvl` is unset, effective level for write actions is
       `NoAccess`; reads still flow from `repoLvl`.
6. Otherwise (unprotected):
     - Effective level = `max(repoLvl, branchLvl)`. Branch grants only add.

Reads are not gated by branch grants in v1. If a user can read the repo, they
can read every branch.

#### Enforcement points

Two call sites for the helper:

- `pkg/backend/hooks.go:Update()` — extends the existing per-ref hook to call
  `BranchAccessLevelForUser` with `refName` from the hook arg. Reject the ref
  update (write to stderr, exit non-zero in the hook protocol) when the
  effective level is below `ReadWriteAccess`.
- PR merge code path — calls the helper with `refName = target_branch` before
  attempting the merge.

The auth check is purely a function of the predicate above. Server-initiated
merges (Section 3) propagate the merger's user identity into the hook, so the
hook re-evaluates the same predicate with the same identity and allows it
naturally. No special bypass.

### 2. PR data model

#### Schema

```sql
CREATE TABLE pull_requests (
    id                INTEGER PRIMARY KEY,
    repo_id           INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    number            INTEGER NOT NULL,   -- per-repo sequence
    creator_id        INTEGER NOT NULL REFERENCES users(id),
    source_branch     TEXT    NOT NULL,   -- short ref name
    target_branch     TEXT    NOT NULL,
    title             TEXT    NOT NULL,
    body              TEXT    NOT NULL DEFAULT '',
    status            INTEGER NOT NULL,   -- 0=open 1=merged 2=closed
    merge_commit_sha  TEXT,               -- set on merge
    created_at        DATETIME NOT NULL,
    updated_at        DATETIME NOT NULL,
    merged_at         DATETIME,
    closed_at         DATETIME,
    UNIQUE(repo_id, number)
);
```

#### Numbering

Per-repo sequential, GitHub-style. Allocation:
`SELECT MAX(number)+1 FROM pull_requests WHERE repo_id = ?` inside the insert
transaction. The `UNIQUE(repo_id, number)` constraint catches the rare race;
the caller retries once.

#### Lifecycle

```
       create
         │
         ▼
       open ──merge──▶ merged   (terminal; records merge_commit_sha)
         │
         └──close──▶ closed     (terminal; abandoned)
```

A PR stores branch names, not SHAs. The diff and mergeability are computed on
demand from the live tips using `go-git` (`merge-base` + tree compare). If the
source branch is deleted while the PR is open, reads surface a "source missing"
indicator; the PR is not auto-closed.

#### Authorization per action

| Action       | Allowed when |
|--------------|--------------|
| Open PR      | Read access on repo; both src and target exist; src ≠ target |
| List / show  | Read access on repo |
| Close        | Creator, anyone with effective write on target, or admin |
| Merge        | `BranchAccessLevelForUser(merger, target) ≥ ReadWriteAccess` |

### 3. Server-side merge mechanism

The merge runs in soft-serve's process, executed under the merger's identity.
Implementation shells out to system `git` via the existing
`github.com/aymanbagabas/git-module` abstraction. Uses
`git merge-tree --write-tree` (git ≥ 2.38, already a soft-serve runtime
requirement) so no working tree is needed.

#### Algorithm

```
1. Resolve src_tip = sha(source_branch); tgt_tip = sha(target_branch).
2. If src_tip is ancestor of tgt_tip → reject "nothing to merge".
3. If tgt_tip is ancestor of src_tip → fast-forward:
     git update-ref refs/heads/<target> <src_tip> <tgt_tip>     # CAS
     merge_commit_sha = src_tip
4. Else → true merge:
     base = git merge-base <tgt_tip> <src_tip>
     tree = git merge-tree --write-tree --merge-base=<base> <tgt_tip> <src_tip>
            (non-zero exit or conflict markers → refuse with conflict error)
     commit = git commit-tree <tree> -p <tgt_tip> -p <src_tip> -m "<msg>"
              (author = committer = merger;
               msg = "Merge pull request #N: <title>\n\n<body>")
     git update-ref refs/heads/<target> <commit> <tgt_tip>      # CAS
     merge_commit_sha = commit
5. Mark PR merged, set merged_at and merge_commit_sha. Fire webhook.
```

Steps 3 and 4 use `git update-ref … --old <tgt_tip>` (CAS). A concurrent push
that moves the target between mergeability check and ref update fails the CAS
and the merger sees a "target advanced, retry" error rather than a silent
overwrite.

#### Conflict UX

On conflict, the SSH command exits non-zero and prints:

```
error: PR #5 cannot be merged: conflicts in 2 files
  src/foo.go
  README.md
Rebase the source branch locally and push again.
```

#### Edge cases handled in v1

- Source branch deleted between PR open and merge → refuse, "source branch no
  longer exists".
- Target branch deleted → refuse, "target branch no longer exists".
- PR already merged or closed → refuse, "PR is not open".
- Target tip moved after mergeability check → CAS fails, retry surfaced to
  user.

### 4. SSH command surface

New commands slot into the existing `pkg/ssh/cmd/repo.go` cobra tree following
the `collab.go` convention (one file per command group).

```
soft repo pr create  REPO --from <src> --to <target> --title <T> [--body <B>]
soft repo pr list    REPO [--state open|merged|closed|all]
soft repo pr show    REPO <#>
soft repo pr merge   REPO <#>
soft repo pr close   REPO <#>

soft repo branch grant     REPO <user> <pattern>
soft repo branch revoke    REPO <user> <pattern>
soft repo branch grants    REPO
soft repo branch protect   REPO <pattern>
soft repo branch unprotect REPO <pattern>
soft repo branch protected REPO
```

New files: `pkg/ssh/cmd/pr.go`, `pkg/ssh/cmd/branchacl.go`.

Each command does access check → backend call → render. Output mirrors
existing commands' tabular style. The cobra `PersistentPreRunE` hooks use the
existing `checkIfReadable` and `checkIfReadableAndCollab` helpers. The
PR-action-specific checks (creator vs merger vs admin) happen inside the run
function. Branch-ACL management commands require `AdminAccess` on the repo.

### 5. Webhooks

Add `pkg/webhook/pull_request.go` mirroring the patterns in `branch_tag.go`
and `push.go`. Three new events: `pr_opened`, `pr_merged`, `pr_closed`. Fire
from the corresponding backend methods. No schema change — uses the existing
`webhooks` table and event-dispatch path.

## Migration and backwards compatibility

Single migration: `pkg/db/migrate/0004_branch_acls_and_prs.go`. Creates the
three new tables. Makes **zero changes** to existing tables.

Backwards compatibility is automatic. With no rows in `branch_collabs` or
`protected_branches`:

- `BranchAccessLevelForUser` short-circuits to `AccessLevelForUser` (the
  existing function).
- The `Update` hook's new check is a no-op (no policy to enforce).
- Existing collab and SSH command behavior is identical.

The new features are entirely opt-in at the per-branch level.

## Testing

| Layer | What's tested | Where |
|-------|---------------|-------|
| Unit, table-driven | `BranchAccessLevelForUser` across admin / owner / repo-only / grant-only / both / protected / unprotected (matrix) | `pkg/backend/user_test.go` |
| Unit | PR state transitions; only valid transitions allowed | `pkg/backend/pr_test.go` |
| Unit | Merge algorithm: ancestor short-circuit, FF, true-merge, conflict, CAS race | `pkg/backend/merge_test.go` against a temp bare repo |
| Integration | End-to-end against a real soft-serve instance: create PR, push to non-protected vs protected branches as different users, merge | follow existing `cmd/soft/serve/*_test.go` patterns |
| Integration | Hook rejection on push to protected branch without grant | same |

Migration also gets a smoke test that idempotently applies and rolls back.

## Risk register

- **`git merge-tree --write-tree` availability.** Requires git ≥ 2.38 (Aug 2022).
  Soft-serve already needs system git; the requirement bump is small. The
  install docs and Dockerfile should note this.
- **Concurrent merge into same target.** Handled by CAS on `update-ref`. A
  retry loop in the merge command can paper over the rare conflict, but v1
  surfaces the retry to the user (simpler, no hidden state machine).
- **Glob pattern footguns.** A wildly broad pattern like `*` in either table
  applies to all branches. Documented in the command help text; not
  prevented in code.
- **Branch grants on deleted users / repos.** Foreign keys cascade.
- **Reads of grant tables on every push.** The `Update` hook fires per ref;
  on a 500-ref push, the helper runs 500 times. The two grant tables are tiny
  per repo (rows ~= dozens at most). Indexed by `repo_id` this is negligible.

## Out of v1, on the roadmap

- TUI page listing PRs and showing diffs.
- HTTP/JSON API for PRs and branch ACLs.
- Comments and lightweight review (approve / request-changes).
- Reopen of closed PRs.
- Additional merge strategies (squash, rebase).
- Per-branch read restrictions.
- Fork-based PRs (depends on a fork concept that does not exist today).
