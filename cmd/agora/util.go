package main

import (
	"context"

	"github.com/openziti/agora/internal/persistence"
)

func openStore(cfg persistence.Config) *persistence.Store {
	store, err := persistence.Open(context.Background(), cfg)
	if err != nil {
		panic(err)
	}
	return store
}

func appliedLabel(applied bool) string {
	if applied {
		return "applied"
	}
	return "pending"
}
