package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteContract(ctx context.Context, params api.DeleteContractParams) (api.DeleteContractRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.DeleteContractUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	c, err := s.requireOwnedContract(ctx, principal, params.ContractId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteContractNotFound{Code: "not_found", Message: "contract not found"}, nil
		}
		return &api.DeleteContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	inUse, err := s.store.Contracts.IsReferencedByActiveAdvertisement(ctx, s.store.DB(), c.ID)
	if err != nil {
		return &api.DeleteContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if inUse {
		return &api.DeleteContractConflict{Code: "contract_in_use", Message: "cannot delete a contract referenced by an active advertisement"}, nil
	}

	if err := s.store.Contracts.Delete(ctx, s.store.DB(), c.ID); err != nil {
		dl.Errorf("delete contract persistence failed id='%s' %s: %v", c.ID, principalLogFields(principal), err)
		return &api.DeleteContractInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("deleted contract id='%s' %s", c.ID, principalLogFields(principal))
	return &api.DeleteContractNoContent{}, nil
}
