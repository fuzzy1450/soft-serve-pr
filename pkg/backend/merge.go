package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/utils"
)

var (
	errMergeConflict  = errors.New("merge has conflicts")
	errNothingToMerge = errors.New("nothing to merge: source already merged into target")
	errBranchMissing  = errors.New("branch missing")
	errTargetMoved    = errors.New("target advanced concurrently; retry")
)

// MergeConflictError carries the conflicted file paths from a merge attempt.
// Implements error and matches errMergeConflict via errors.Is for callers that
// only need to detect "is this a conflict?".
type MergeConflictError struct {
	Paths []string
}

func (e *MergeConflictError) Error() string {
	if len(e.Paths) == 0 {
		return "merge has conflicts"
	}
	return fmt.Sprintf("merge has conflicts in %d file(s)", len(e.Paths))
}

func (e *MergeConflictError) Is(target error) bool {
	return target == errMergeConflict
}

type mergeResult struct {
	FastForward    bool
	MergeCommitSha string
}

// performMerge merges sourceBranch into targetBranch in the bare repo for `repo`.
// authorEmail may be empty.
func (d *Backend) performMerge(ctx context.Context, repo, sourceBranch, targetBranch, authorName, message, authorEmail string) (mergeResult, error) {
	repo = utils.SanitizeRepo(repo)
	cfg := config.FromContext(ctx)
	if cfg == nil {
		cfg = d.cfg
	}
	gitDir := filepath.Join(cfg.DataPath, "repos", repo+".git")

	srcTip, err := runGit(ctx, gitDir, "rev-parse", "refs/heads/"+sourceBranch)
	if err != nil {
		return mergeResult{}, fmt.Errorf("%w: source %q", errBranchMissing, sourceBranch)
	}
	tgtTip, err := runGit(ctx, gitDir, "rev-parse", "refs/heads/"+targetBranch)
	if err != nil {
		return mergeResult{}, fmt.Errorf("%w: target %q", errBranchMissing, targetBranch)
	}
	srcTip, tgtTip = strings.TrimSpace(srcTip), strings.TrimSpace(tgtTip)

	// Is src already ancestor of tgt? -> nothing to merge.
	if _, err := runGit(ctx, gitDir, "merge-base", "--is-ancestor", srcTip, tgtTip); err == nil {
		return mergeResult{}, errNothingToMerge
	}

	// Is tgt ancestor of src? -> fast-forward.
	if _, err := runGit(ctx, gitDir, "merge-base", "--is-ancestor", tgtTip, srcTip); err == nil {
		if _, err := runGit(ctx, gitDir, "update-ref", "refs/heads/"+targetBranch, srcTip, tgtTip); err != nil {
			return mergeResult{}, errTargetMoved
		}
		return mergeResult{FastForward: true, MergeCommitSha: srcTip}, nil
	}

	// True merge.
	base, err := runGit(ctx, gitDir, "merge-base", tgtTip, srcTip)
	if err != nil {
		return mergeResult{}, fmt.Errorf("merge-base: %w", err)
	}
	base = strings.TrimSpace(base)

	treeOut, treeErr := runGitWithStderr(ctx, gitDir, "merge-tree", "--write-tree", "--merge-base="+base, tgtTip, srcTip)
	if treeErr != nil {
		// Conflicts produce non-zero exit AND a conflict report on stdout.
		paths := parseConflictPaths(treeOut)
		return mergeResult{}, &MergeConflictError{Paths: paths}
	}
	tree := strings.TrimSpace(strings.Split(treeOut, "\n")[0])

	// commit-tree with two parents.
	email := authorEmail
	if email == "" {
		email = authorName + "@soft-serve.local"
	}
	env := []string{
		"GIT_AUTHOR_NAME=" + authorName,
		"GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + authorName,
		"GIT_COMMITTER_EMAIL=" + email,
	}
	commit, err := runGitEnv(ctx, gitDir, env, "commit-tree", tree, "-p", tgtTip, "-p", srcTip, "-m", message)
	if err != nil {
		return mergeResult{}, fmt.Errorf("commit-tree: %w", err)
	}
	commit = strings.TrimSpace(commit)

	if _, err := runGit(ctx, gitDir, "update-ref", "refs/heads/"+targetBranch, commit, tgtTip); err != nil {
		return mergeResult{}, errTargetMoved
	}
	return mergeResult{FastForward: false, MergeCommitSha: commit}, nil
}

func runGit(ctx context.Context, gitDir string, args ...string) (string, error) {
	return runGitEnv(ctx, gitDir, nil, args...)
}

func runGitEnv(ctx context.Context, gitDir string, env []string, args ...string) (string, error) {
	full := append([]string{"--git-dir=" + gitDir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(cmd.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %v: %w (stderr=%q)", args, err, stderr.String())
	}
	return stdout.String(), nil
}

// runGitWithStderr returns combined stdout, and an error containing stderr when exit non-zero.
// Used for merge-tree where conflict info appears on stdout.
func runGitWithStderr(ctx context.Context, gitDir string, args ...string) (string, error) {
	full := append([]string{"--git-dir=" + gitDir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("git %v: %w (stderr=%q)", args, err, stderr.String())
	}
	return stdout.String(), nil
}

// parseConflictPaths extracts conflicted file paths from merge-tree output.
// merge-tree --write-tree with conflicts prints the partial tree sha on line 1
// and then a list of "<oid> <stage> <path>" lines for conflicted entries.
func parseConflictPaths(out string) []string {
	lines := strings.Split(out, "\n")
	seen := map[string]struct{}{}
	for i, line := range lines {
		if i == 0 {
			continue // tree sha
		}
		fields := strings.Fields(line)
		// Index entry lines have format: "<mode> <sha> <stage>\t<path>"
		// Stage is always "1", "2", or "3". Skip human-readable CONFLICT lines.
		if len(fields) >= 4 && (fields[2] == "1" || fields[2] == "2" || fields[2] == "3") {
			seen[strings.Join(fields[3:], " ")] = struct{}{}
		}
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	return paths
}
