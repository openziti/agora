package persistence

import "errors"

var (
	ErrNotFound = errors.New("persistence: not found")
	ErrConflict = errors.New("persistence: conflict")
)
