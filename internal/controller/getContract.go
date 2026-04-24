package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetContract(ctx context.Context, params api.GetContractParams) (api.GetContractRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetContractUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	c, err := s.requireVisibleContract(ctx, principal, params.ContractId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.GetContractNotFound{Code: "not_found", Message: "contract not found"}, nil
		}
		return &api.GetContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return mapContract(c), nil
}
