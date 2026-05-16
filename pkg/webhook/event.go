package webhook

import (
	"encoding"
	"errors"
)

// Event is a webhook event.
type Event int

const (
	// EventBranchTagCreate is a branch or tag create event.
	EventBranchTagCreate Event = 1

	// EventBranchTagDelete is a branch or tag delete event.
	EventBranchTagDelete Event = 2

	// EventCollaborator is a collaborator change event.
	EventCollaborator Event = 3

	// EventPush is a push event.
	EventPush Event = 4

	// EventRepository is a repository create, delete, rename event.
	EventRepository Event = 5

	// EventRepositoryVisibilityChange is a repository visibility change event.
	EventRepositoryVisibilityChange Event = 6

	// EventPullRequestOpened is a pull request opened event.
	EventPullRequestOpened Event = 7

	// EventPullRequestMerged is a pull request merged event.
	EventPullRequestMerged Event = 8

	// EventPullRequestClosed is a pull request closed event.
	EventPullRequestClosed Event = 9
)

// Events return all events.
func Events() []Event {
	return []Event{
		EventBranchTagCreate,
		EventBranchTagDelete,
		EventCollaborator,
		EventPush,
		EventRepository,
		EventRepositoryVisibilityChange,
		EventPullRequestOpened,
		EventPullRequestMerged,
		EventPullRequestClosed,
	}
}

var eventStrings = map[Event]string{
	EventBranchTagCreate:            "branch_tag_create",
	EventBranchTagDelete:            "branch_tag_delete",
	EventCollaborator:               "collaborator",
	EventPush:                       "push",
	EventRepository:                 "repository",
	EventRepositoryVisibilityChange: "repository_visibility_change",
	EventPullRequestOpened:          "pull_request_opened",
	EventPullRequestMerged:          "pull_request_merged",
	EventPullRequestClosed:          "pull_request_closed",
}

// String returns the string representation of the event.
func (e Event) String() string {
	return eventStrings[e]
}

var stringEvent = map[string]Event{
	"branch_tag_create":            EventBranchTagCreate,
	"branch_tag_delete":            EventBranchTagDelete,
	"collaborator":                 EventCollaborator,
	"push":                         EventPush,
	"repository":                   EventRepository,
	"repository_visibility_change": EventRepositoryVisibilityChange,
	"pull_request_opened":          EventPullRequestOpened,
	"pull_request_merged":          EventPullRequestMerged,
	"pull_request_closed":          EventPullRequestClosed,
}

// ErrInvalidEvent is returned when the event is invalid.
var ErrInvalidEvent = errors.New("invalid event")

// ParseEvent parses an event string and returns the event.
func ParseEvent(s string) (Event, error) {
	e, ok := stringEvent[s]
	if !ok {
		return -1, ErrInvalidEvent
	}

	return e, nil
}

var (
	_ encoding.TextMarshaler   = Event(0)
	_ encoding.TextUnmarshaler = (*Event)(nil)
)

// UnmarshalText implements encoding.TextUnmarshaler.
func (e *Event) UnmarshalText(text []byte) error {
	ev, err := ParseEvent(string(text))
	if err != nil {
		return err
	}

	*e = ev
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (e Event) MarshalText() (text []byte, err error) {
	ev := e.String()
	if ev == "" {
		return nil, ErrInvalidEvent
	}

	return []byte(ev), nil
}
