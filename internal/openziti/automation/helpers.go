package automation

import "fmt"

func oneResult[T any](resource, operation, selector string, items []*T) (*T, error) {
	switch len(items) {
	case 0:
		return nil, &AutomationError{
			Type:      ErrorTypeNotFound,
			Resource:  resource,
			Operation: operation,
			Cause:     fmt.Errorf("%s %q not found", resource, selector),
		}
	case 1:
		return items[0], nil
	default:
		return nil, &AutomationError{
			Type:      ErrorTypeConflict,
			Resource:  resource,
			Operation: operation,
			Cause:     fmt.Errorf("expected one %s for %q, found %d", resource, selector, len(items)),
		}
	}
}
