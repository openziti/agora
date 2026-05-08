package catalog

import "errors"

var (
	// ErrNotFound marks a catalog resource lookup that found no record.
	ErrNotFound = errors.New("catalog: not found")

	// ErrConflict marks a catalog operation that conflicted with existing state.
	ErrConflict = errors.New("catalog: conflict")

	// ErrUnauthorized marks a controller response for missing or invalid credentials.
	ErrUnauthorized = errors.New("catalog: unauthorized")

	// ErrForbidden marks a controller response for authenticated but disallowed access.
	ErrForbidden = errors.New("catalog: forbidden")

	// ErrBadRequest marks invalid caller input or a controller validation error.
	ErrBadRequest = errors.New("catalog: bad request")

	// ErrTransient marks a controller or transport failure that may succeed later.
	ErrTransient = errors.New("catalog: transient controller error")
)
