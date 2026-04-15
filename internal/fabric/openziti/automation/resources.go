package automation

import "context"

type ResourceOperations[T any] interface {
	Find(context.Context, *FilterOptions) ([]*T, error)
	Delete(context.Context, string) error
}

func deleteWithFilter[T any](ctx context.Context, ops ResourceOperations[T], filter string, getID func(*T) string) error {
	items, err := ops.Find(ctx, &FilterOptions{Filter: filter})
	if err != nil {
		return err
	}
	for _, item := range items {
		id := getID(item)
		if id == "" {
			continue
		}
		if err := ops.Delete(ctx, id); err != nil && !IsNotFound(err) {
			return err
		}
	}
	return nil
}
