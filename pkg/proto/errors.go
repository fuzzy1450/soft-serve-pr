package proto

import (
	"errors"
)

var (
	// ErrUnauthorized is returned when the user is not authorized to perform action.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrInvalidRemote is returned when a repository import remote is invalid.
	ErrInvalidRemote = errors.New("remote must be a network URL")
	// ErrFileNotFound is returned when the file is not found.
	ErrFileNotFound = errors.New("file not found")
	// ErrRepoNotFound is returned when a repository is not found.
	ErrRepoNotFound = errors.New("repository not found")
	// ErrRepoExist is returned when a repository already exists.
	ErrRepoExist = errors.New("repository already exists")
	// ErrUserNotFound is returned when a user is not found.
	ErrUserNotFound = errors.New("user not found")
	// ErrTokenNotFound is returned when a token is not found.
	ErrTokenNotFound = errors.New("token not found")
	// ErrTokenExpired is returned when a token is expired.
	ErrTokenExpired = errors.New("token expired")
	// ErrCollaboratorNotFound is returned when a collaborator is not found.
	ErrCollaboratorNotFound = errors.New("collaborator not found")
	// ErrCollaboratorExist is returned when a collaborator already exists.
	ErrCollaboratorExist = errors.New("collaborator already exists")

	// ErrPRNotFound is returned when a pull request is not found.
	ErrPRNotFound = errors.New("pull request not found")

	// ErrPRNotOpen is returned for actions that require an open PR.
	ErrPRNotOpen = errors.New("pull request is not open")

	// ErrPRSameBranch is returned when source and target branches are equal.
	ErrPRSameBranch = errors.New("source and target branches must differ")

	// ErrPRBranchMissing is returned when a referenced branch does not exist.
	ErrPRBranchMissing = errors.New("branch does not exist")
)
