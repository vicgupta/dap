package core

import "errors"

var (
	// ErrBlocked means guardrails refused the action.
	ErrBlocked = errors.New("blocked by guardrails")
	// ErrNotFound means the card or plugin is not in the data directory.
	ErrNotFound = errors.New("not found")
	// ErrDenied means the call is outside the plugin's declared permissions.
	ErrDenied = errors.New("permission denied")
	// ErrBadState means the card is not in a state that allows the action.
	ErrBadState = errors.New("invalid card state")
)
