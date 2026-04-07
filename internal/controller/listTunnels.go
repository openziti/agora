package controller

import (
	"context"

	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListTunnels(ctx context.Context) (api.ListTunnelsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ListTunnelsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	tunnels, err := s.store.Tunnels.ListByOrganization(ctx, s.store.DB(), principal.OrganizationID)
	if err != nil {
		return &api.ListTunnelsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListTunnelsResponse, 0, len(tunnels))
	for i := range tunnels {
		resp = append(resp, *mapTunnel(&tunnels[i]))
	}
	return &resp, nil
}
