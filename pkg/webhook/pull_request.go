package webhook

import (
	"context"
	"fmt"

	"github.com/charmbracelet/soft-serve/pkg/config"
	"github.com/charmbracelet/soft-serve/pkg/db"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/charmbracelet/soft-serve/pkg/proto"
	"github.com/charmbracelet/soft-serve/pkg/store"
)

// PullRequestEvent is a pull request lifecycle event.
type PullRequestEvent struct {
	Common

	// Action is one of "opened", "merged", "closed".
	Action string `json:"action" url:"action"`
	// PRNumber is the per-repo PR number.
	PRNumber int64 `json:"pr_number" url:"pr_number"`
	// Title is the PR title.
	Title string `json:"title" url:"title"`
	// Body is the PR body.
	Body string `json:"body" url:"body"`
	// SourceBranch is the head branch.
	SourceBranch string `json:"source_branch" url:"source_branch"`
	// TargetBranch is the base branch.
	TargetBranch string `json:"target_branch" url:"target_branch"`
	// Status mirrors the PR status string ("open"/"merged"/"closed").
	Status string `json:"status" url:"status"`
	// MergeCommitSha is set when action == "merged".
	MergeCommitSha string `json:"merge_commit_sha,omitempty" url:"merge_commit_sha,omitempty"`
	// Creator is the username of the PR's original author.
	Creator string `json:"creator" url:"creator"`
}

// NewPullRequestEvent constructs a PullRequestEvent for the given action.
// `action` must be "opened", "merged", or "closed".
func NewPullRequestEvent(ctx context.Context, actor proto.User, repo proto.Repository, pr models.PullRequest, action string) (PullRequestEvent, error) {
	var event Event
	switch action {
	case "opened":
		event = EventPullRequestOpened
	case "merged":
		event = EventPullRequestMerged
	case "closed":
		event = EventPullRequestClosed
	default:
		return PullRequestEvent{}, fmt.Errorf("invalid pull request action: %q", action)
	}

	payload := PullRequestEvent{
		Action:       action,
		PRNumber:     pr.Number,
		Title:        pr.Title,
		Body:         pr.Body,
		SourceBranch: pr.SourceBranch,
		TargetBranch: pr.TargetBranch,
		Status:       pr.Status.String(),
		Common: Common{
			EventType: event,
			Repository: Repository{
				ID:          repo.ID(),
				Name:        repo.Name(),
				Description: repo.Description(),
				ProjectName: repo.ProjectName(),
				Private:     repo.IsPrivate(),
				CreatedAt:   repo.CreatedAt(),
				UpdatedAt:   repo.UpdatedAt(),
			},
			Sender: User{
				ID:       actor.ID(),
				Username: actor.Username(),
			},
		},
	}
	if pr.MergeCommitSha.Valid {
		payload.MergeCommitSha = pr.MergeCommitSha.String
	}

	cfg := config.FromContext(ctx)
	payload.Repository.HTTPURL = repoURL(cfg.HTTP.PublicURL, repo.Name())
	payload.Repository.SSHURL = repoURL(cfg.SSH.PublicURL, repo.Name())
	payload.Repository.GitURL = repoURL(cfg.Git.PublicURL, repo.Name())

	dbx := db.FromContext(ctx)
	datastore := store.FromContext(ctx)
	owner, err := datastore.GetUserByID(ctx, dbx, repo.UserID())
	if err != nil {
		return PullRequestEvent{}, db.WrapError(err)
	}
	payload.Repository.Owner.ID = owner.ID
	payload.Repository.Owner.Username = owner.Username
	payload.Repository.DefaultBranch, _ = getDefaultBranch(repo)

	// Resolve creator's username for the payload.
	creator, err := datastore.GetUserByID(ctx, dbx, pr.CreatorID)
	if err == nil {
		payload.Creator = creator.Username
	}

	return payload, nil
}
