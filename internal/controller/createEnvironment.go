package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateEnvironment(ctx context.Context, req *api.CreateEnvironmentRequest) (api.CreateEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.CreateEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	env := persistence.Environment{
		OrganizationID: principal.OrganizationID,
		AccountID:      principal.AccountID,
		ZitiIdentityID: req.ZitiIdentityId,
	}
	if description, ok := req.Description.Get(); ok {
		env.Description = &description
	}
	if host, ok := req.Host.Get(); ok {
		env.Host = &host
	}
	created, err := s.store.Environments.Create(ctx, s.store.DB(), env)
	if err != nil {
		return &api.CreateEnvironmentConflict{Code: "conflict", Message: err.Error()}, nil
	}
	return mapEnvironment(created), nil
}
