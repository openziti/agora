package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListTunnelGrants(ctx context.Context, params api.ListTunnelGrantsParams) (api.ListTunnelGrantsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized list tunnel grants request tunnel_id='%s'", params.TunnelId)
		return &api.ListTunnelGrantsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Debugf("listing tunnel grants tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))

	tunnel, err := s.requireManagedTunnel(ctx, principal, params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ListTunnelGrantsNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		return &api.ListTunnelGrantsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	grants, err := s.store.TunnelGrants.ListByTunnel(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID)
	if err != nil {
		dl.Errorf("list tunnel grants failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
		return &api.ListTunnelGrantsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	resp := make(api.ListTunnelGrantsResponse, 0, len(grants))
	for i := range grants {
		resp = append(resp, *mapTunnelGrant(&grants[i]))
	}
	return &resp, nil
}
