package controller

import (
	"context"
	"errors"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteTunnel(ctx context.Context, params api.DeleteTunnelParams) (api.DeleteTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.DeleteTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	if err := s.store.Tunnels.Delete(ctx, s.store.DB(), params.TunnelId, principal.OrganizationID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	return &api.DeleteTunnelNoContent{}, nil
}
