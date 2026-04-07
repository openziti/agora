package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetTunnel(ctx context.Context, params api.GetTunnelParams) (api.GetTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.GetTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	tunnel, err := s.store.Tunnels.GetByID(ctx, s.store.DB(), params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.GetTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		return &api.GetTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if tunnel.OrganizationID != principal.OrganizationID {
		return &api.GetTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
	}
	return mapTunnel(tunnel), nil
}
