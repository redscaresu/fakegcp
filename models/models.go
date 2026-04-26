package models

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")

	// ErrConflict signals a generic 409. Prefer one of the more
	// specific sentinels below — they map to distinct GCP error
	// payloads. Code that imports ErrConflict directly still works
	// (it inherits the generic mapping in writeDomainError).
	ErrConflict = errors.New("conflict")

	// ErrInUse means the resource has live dependents and cannot
	// be deleted. Maps to the 409 "resourceInUseByAnotherResource"
	// reason that real Cloud APIs return for FK-protected deletes.
	ErrInUse = errors.New("resource in use by another resource")

	// ErrTerminalState means the resource is in a state from
	// which the requested transition is not permitted (e.g. a
	// Secret Manager version that has already been DESTROYED).
	// Maps to the neutral 409 "conflict" reason.
	ErrTerminalState = errors.New("resource is in a terminal state")
)
