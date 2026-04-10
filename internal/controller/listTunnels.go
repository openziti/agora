package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
)

func (s *Service) ListTunnels(ctx context.Context) (api.ListTunnelsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warn("unauthorized list tunnels request")
		return &api.ListTunnelsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Debugf("listing tunnels %s", principalLogFields(principal))
	tunnels, err := s.store.Tunnels.ListByOrganization(ctx, s.store.DB(), principal.OrganizationID)
	if err != nil {
		dl.Errorf("list tunnels failed %s: %v", principalLogFields(principal), err)
		return &api.ListTunnelsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	resp := make(api.ListTunnelsResponse, 0, len(tunnels))
	for i := range tunnels {
		resp = append(resp, *mapTunnel(&tunnels[i]))
	}
	dl.Infof("listed %d tunnel(s) %s", len(resp), principalLogFields(principal))
	return &resp, nil
}
