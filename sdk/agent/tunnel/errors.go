package tunnel

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNotFound marks a tunnel runtime lookup that found no actor.
	ErrNotFound = errors.New("tunnel: not found")

	// ErrInvalidSpec marks invalid caller input.
	ErrInvalidSpec = errors.New("tunnel: invalid spec")

	// ErrRuntimeMissing marks an agent without a started embedded runtime.
	ErrRuntimeMissing = errors.New("tunnel: agent has no embedded runtime")

	// ErrUnsupportedMode marks an unsupported tunnel mode.
	ErrUnsupportedMode = errors.New("tunnel: unsupported mode")

	// ErrConflict marks a controller-side resource conflict.
	ErrConflict = errors.New("tunnel: conflict")

	// ErrTransient marks a runtime or controller failure that may later recover.
	ErrTransient = errors.New("tunnel: transient runtime error")
)

func invalidSpec(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, ErrInvalidSpec)...)
}

func unsupportedMode(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, ErrUnsupportedMode)...)
}

func runtimeMissing(op string) error {
	return fmt.Errorf("%s: %w", op, ErrRuntimeMissing)
}

func transientRuntimeError(op string, err error) error {
	if isUnsupportedRuntimeMode(err) {
		return fmt.Errorf("%s: %v: %w", op, err, ErrUnsupportedMode)
	}
	return fmt.Errorf("%s: %v: %w", op, err, ErrTransient)
}

func transientRuntimeMessage(op, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "runtime entered error state"
	}
	return fmt.Errorf("%s: %s: %w", op, message, ErrTransient)
}

type unsupportedRuntimeMode interface {
	UnsupportedTunnelMode() string
}

func isUnsupportedRuntimeMode(err error) bool {
	var unsupported unsupportedRuntimeMode
	return errors.As(err, &unsupported)
}

type runtimeManagedActorNotFound interface {
	ManagedActorNotFound() bool
}

func isManagedActorNotFound(err error) bool {
	var notFound runtimeManagedActorNotFound
	if errors.As(err, &notFound) && notFound.ManagedActorNotFound() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "managed serve not found") || strings.Contains(msg, "managed connect not found")
}
