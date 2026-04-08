package automation

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorType string

const (
	ErrorTypeNotFound ErrorType = "not_found"
	ErrorTypeConflict ErrorType = "conflict"
	ErrorTypeNetwork  ErrorType = "network"
	ErrorTypeInternal ErrorType = "internal"
	ErrorTypeAuth     ErrorType = "auth"
	ErrorTypeInvalid  ErrorType = "invalid"
)

type AutomationError struct {
	Type      ErrorType
	Resource  string
	Operation string
	Cause     error
}

func (e *AutomationError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.Operation, e.Resource, e.Cause)
}

func (e *AutomationError) Unwrap() error {
	return e.Cause
}

func IsNotFound(err error) bool {
	var target *AutomationError
	return errors.As(err, &target) && target.Type == ErrorTypeNotFound
}

func IsConflict(err error) bool {
	var target *AutomationError
	return errors.As(err, &target) && target.Type == ErrorTypeConflict
}

func IsRetryable(err error) bool {
	var target *AutomationError
	return errors.As(err, &target) && target.Type == ErrorTypeNetwork
}

func wrapError(resource, operation string, err error) error {
	if err == nil {
		return nil
	}
	return &AutomationError{
		Type:      classifyError(err),
		Resource:  resource,
		Operation: operation,
		Cause:     err,
	}
}

func classifyError(err error) ErrorType {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not found"),
		strings.Contains(message, "could not find"),
		strings.Contains(message, "404"):
		return ErrorTypeNotFound
	case strings.Contains(message, "already exists"),
		strings.Contains(message, "conflict"),
		strings.Contains(message, "409"):
		return ErrorTypeConflict
	case strings.Contains(message, "unauthorized"),
		strings.Contains(message, "forbidden"),
		strings.Contains(message, "authentication"),
		strings.Contains(message, "401"),
		strings.Contains(message, "403"):
		return ErrorTypeAuth
	case strings.Contains(message, "timeout"),
		strings.Contains(message, "connection refused"),
		strings.Contains(message, "connection reset"),
		strings.Contains(message, "eof"),
		strings.Contains(message, "tls"):
		return ErrorTypeNetwork
	case strings.Contains(message, "invalid"),
		strings.Contains(message, "bad request"),
		strings.Contains(message, "400"),
		strings.Contains(message, "422"):
		return ErrorTypeInvalid
	default:
		return ErrorTypeInternal
	}
}
