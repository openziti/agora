package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListContracts(ctx context.Context) (api.ListContractsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ListContractsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	rows, err := s.store.Contracts.ListByAccount(ctx, s.store.DB(), principal.AccountID)
	if err != nil {
		dl.Errorf("list contracts failed %s: %v", principalLogFields(principal), err)
		return &api.ListContractsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	out := make(api.ListContractsResponse, 0, len(rows))
	for i := range rows {
		out = append(out, *mapContract(&rows[i]))
	}
	return &out, nil
}
