package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateTunnel(ctx context.Context, req *api.CreateTunnelRequest) (api.CreateTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.CreateTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	env, err := s.store.Environments.GetByID(ctx, s.store.DB(), req.EnvironmentId)
	if err != nil {
		return &api.CreateTunnelNotFound{Code: "not_found", Message: "environment not found"}, nil
	}
	if env.OrganizationID != principal.OrganizationID {
		return &api.CreateTunnelNotFound{Code: "not_found", Message: "environment not found"}, nil
	}
	tunnel := persistence.Tunnel{
		OrganizationID: principal.OrganizationID,
		EnvironmentID:  req.EnvironmentId,
		Name:           req.Name,
		BackendAddress: req.BackendAddress,
	}
	if zitiServiceID, ok := req.ZitiServiceId.Get(); ok {
		tunnel.ZitiServiceID = &zitiServiceID
	}
	created, err := s.store.Tunnels.Create(ctx, s.store.DB(), tunnel)
	if err != nil {
		return &api.CreateTunnelConflict{Code: "conflict", Message: err.Error()}, nil
	}
	return mapTunnel(created), nil
}
