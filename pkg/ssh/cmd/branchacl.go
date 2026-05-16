package cmd

import (
	"github.com/charmbracelet/soft-serve/pkg/access"
	"github.com/charmbracelet/soft-serve/pkg/backend"
	"github.com/spf13/cobra"
)

func branchGrantCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "grant REPOSITORY USERNAME PATTERN",
		Short:             "Grant a user write access to a branch pattern",
		Args:              cobra.ExactArgs(3),
		PersistentPreRunE: checkIfAdmin,
		RunE: func(cmd *cobra.Command, args []string) error {
			be := backend.FromContext(cmd.Context())
			return be.AddBranchCollab(cmd.Context(), args[0], args[1], args[2], access.ReadWriteAccess)
		},
	}
}

func branchRevokeCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "revoke REPOSITORY USERNAME PATTERN",
		Short:             "Revoke a branch grant",
		Args:              cobra.ExactArgs(3),
		PersistentPreRunE: checkIfAdmin,
		RunE: func(cmd *cobra.Command, args []string) error {
			be := backend.FromContext(cmd.Context())
			return be.RemoveBranchCollab(cmd.Context(), args[0], args[1], args[2])
		},
	}
}

func branchGrantsCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "grants REPOSITORY",
		Short:             "List branch grants",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: checkIfReadable,
		RunE: func(cmd *cobra.Command, args []string) error {
			be := backend.FromContext(cmd.Context())
			rows, err := be.ListBranchCollabs(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			// Resolve user ID to username for display. The simplest approach is
			// to call be.UserByID once per row; for now, print the user id.
			cmd.Println("USER_ID\tPATTERN\tLEVEL")
			for _, r := range rows {
				cmd.Printf("%d\t%s\t%s\n", r.UserID, r.BranchPattern, r.AccessLevel)
			}
			return nil
		},
	}
}

func branchProtectCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "protect REPOSITORY PATTERN",
		Short:             "Mark a branch pattern protected",
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: checkIfAdmin,
		RunE: func(cmd *cobra.Command, args []string) error {
			be := backend.FromContext(cmd.Context())
			return be.ProtectBranch(cmd.Context(), args[0], args[1])
		},
	}
}

func branchUnprotectCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "unprotect REPOSITORY PATTERN",
		Short:             "Remove branch protection",
		Args:              cobra.ExactArgs(2),
		PersistentPreRunE: checkIfAdmin,
		RunE: func(cmd *cobra.Command, args []string) error {
			be := backend.FromContext(cmd.Context())
			return be.UnprotectBranch(cmd.Context(), args[0], args[1])
		},
	}
}

func branchProtectedCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "protected REPOSITORY",
		Short:             "List protected branch patterns",
		Args:              cobra.ExactArgs(1),
		PersistentPreRunE: checkIfReadable,
		RunE: func(cmd *cobra.Command, args []string) error {
			be := backend.FromContext(cmd.Context())
			rows, err := be.ListProtectedBranches(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for _, r := range rows {
				cmd.Println(r.BranchPattern)
			}
			return nil
		},
	}
}
