package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateTunnel(ctx context.Context, req *api.CreateTunnelRequest) (api.CreateTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized create tunnel request environment_id='%s' name='%s'", req.EnvironmentId, req.Name)
		return &api.CreateTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof(
		"creating tunnel name='%s' environment_id='%s' backend_address='%s' %s",
		req.Name,
		req.EnvironmentId,
		req.BackendAddress,
		principalLogFields(principal),
	)
	env, err := s.store.Environments.GetByID(ctx, s.store.DB(), req.EnvironmentId)
	if err != nil {
		dl.Warnf("create tunnel environment not found environment_id='%s' %s", req.EnvironmentId, principalLogFields(principal))
		return &api.CreateTunnelNotFound{Code: "not_found", Message: "environment not found"}, nil
	}
	if env.OrganizationID != principal.OrganizationID {
		dl.Warnf("create tunnel rejected for non-owned environment environment_id='%s' %s", req.EnvironmentId, principalLogFields(principal))
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
		dl.Errorf("create tunnel failed name='%s' environment_id='%s' %s: %v", req.Name, req.EnvironmentId, principalLogFields(principal), err)
		return &api.CreateTunnelConflict{Code: "conflict", Message: err.Error()}, nil
	}
	dl.Infof("created tunnel id='%s' name='%s' environment_id='%s' %s", created.ID, created.Name, created.EnvironmentID, principalLogFields(principal))
	return mapTunnel(created), nil
}
