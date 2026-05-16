package cmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/backend"
	"github.com/charmbracelet/soft-serve/pkg/db/models"
	"github.com/charmbracelet/soft-serve/pkg/proto"
	"github.com/spf13/cobra"
)

func prCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pr",
		Aliases: []string{"prs", "pull-request", "pull-requests"},
		Short:   "Manage pull requests",
	}
	cmd.AddCommand(
		prCreateCommand(),
		prListCommand(),
		prShowCommand(),
		prMergeCommand(),
		prCloseCommand(),
	)
	return cmd
}

func prCreateCommand() *cobra.Command {
	var from, to, title, body string
	cmd := &cobra.Command{
		Use:               "create REPOSITORY",
		Short:             "Open a pull request",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: checkIfReadable,
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" || to == "" || title == "" {
				return errors.New("--from, --to, and --title are required")
			}
			be := backend.FromContext(cmd.Context())
			pr, err := be.OpenPR(cmd.Context(), args[0], from, to, title, body)
			if err != nil {
				return err
			}
			cmd.Printf("Opened PR #%d\n", pr.Number)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "source branch")
	cmd.Flags().StringVar(&to, "to", "", "target branch")
	cmd.Flags().StringVar(&title, "title", "", "PR title")
	cmd.Flags().StringVar(&body, "body", "", "PR body")
	return cmd
}

func prListCommand() *cobra.Command {
	var state string
	cmd := &cobra.Command{
		Use:               "list REPOSITORY",
		Short:             "List pull requests",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: checkIfReadable,
		RunE: func(cmd *cobra.Command, args []string) error {
			be := backend.FromContext(cmd.Context())
			var statusFilter *models.PRStatus
			switch state {
			case "", "open":
				s := models.PRStatusOpen
				statusFilter = &s
			case "merged":
				s := models.PRStatusMerged
				statusFilter = &s
			case "closed":
				s := models.PRStatusClosed
				statusFilter = &s
			case "all":
				statusFilter = nil
			default:
				return fmt.Errorf("unknown state: %q (want open|merged|closed|all)", state)
			}
			rows, err := be.ListPRs(cmd.Context(), args[0], statusFilter)
			if err != nil {
				return err
			}
			cmd.Println("NUMBER\tSTATUS\tSOURCE\tTARGET\tTITLE")
			for _, pr := range rows {
				cmd.Printf("#%d\t%s\t%s\t%s\t%s\n", pr.Number, pr.Status, pr.SourceBranch, pr.TargetBranch, pr.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&state, "state", "open", "open|merged|closed|all")
	return cmd
}

func prShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "show REPOSITORY NUMBER",
		Short:             "Show a pull request",
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: checkIfReadable,
		RunE: func(cmd *cobra.Command, args []string) error {
			be := backend.FromContext(cmd.Context())
			n, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid PR number: %w", err)
			}
			pr, err := be.GetPR(cmd.Context(), args[0], n)
			if err != nil {
				return err
			}
			cmd.Printf("PR #%d: %s\nStatus: %s\nSource: %s\nTarget: %s\n\n%s\n",
				pr.Number, pr.Title, pr.Status, pr.SourceBranch, pr.TargetBranch, pr.Body)
			if pr.MergeCommitSha.Valid {
				cmd.Printf("Merge commit: %s\n", pr.MergeCommitSha.String)
			}
			return nil
		},
	}
}

func prMergeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "merge REPOSITORY NUMBER",
		Short: "Merge an open pull request",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			be := backend.FromContext(ctx)
			n, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return err
			}
			pr, err := be.GetPR(ctx, args[0], n)
			if err != nil {
				return err
			}
			// Auth: the merger must have effective write on target.
			user := proto.UserFromContext(ctx)
			lvl := be.BranchAccessLevelForUser(ctx, args[0], user, "refs/heads/"+pr.TargetBranch)
			if lvl < access.ReadWriteAccess {
				return proto.ErrUnauthorized
			}
			merged, err := be.MergePR(ctx, args[0], n)
			if err != nil {
				// Format conflicts per the spec's UX.
				var cerr *backend.MergeConflictError
				if errors.As(err, &cerr) {
					cmd.PrintErrf("error: PR #%d cannot be merged: conflicts in %d file(s)\n", pr.Number, len(cerr.Paths))
					for _, p := range cerr.Paths {
						cmd.PrintErrln("  " + p)
					}
					cmd.PrintErrln("Rebase the source branch locally and push again.")
					return err
				}
				return err
			}
			cmd.Printf("Merged PR #%d (commit %s)\n", merged.Number, merged.MergeCommitSha.String)
			return nil
		},
	}
}

func prCloseCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "close REPOSITORY NUMBER",
		Short:             "Close (abandon) a pull request",
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: checkIfReadable,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			be := backend.FromContext(ctx)
			n, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return err
			}
			pr, err := be.GetPR(ctx, args[0], n)
			if err != nil {
				return err
			}
			// Auth: creator, target-writer, or admin.
			user := proto.UserFromContext(ctx)
			if user == nil {
				return proto.ErrUnauthorized
			}
			if user.ID() != pr.CreatorID && !user.IsAdmin() {
				lvl := be.BranchAccessLevelForUser(ctx, args[0], user, "refs/heads/"+pr.TargetBranch)
				if lvl < access.ReadWriteAccess {
					return proto.ErrUnauthorized
				}
			}
			if err := be.ClosePR(ctx, args[0], n); err != nil {
				return err
			}
			cmd.Printf("Closed PR #%d\n", n)
			return nil
		},
	}
}
