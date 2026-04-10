package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) GetTunnel(ctx context.Context, params api.GetTunnelParams) (api.GetTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized get tunnel request tunnel_id='%s'", params.TunnelId)
		return &api.GetTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Debugf("getting tunnel tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))

	tunnel, err := s.store.Tunnels.GetByID(ctx, s.store.DB(), params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("get tunnel not found tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
			return &api.GetTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		dl.Errorf("get tunnel failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.GetTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if tunnel.OrganizationID != principal.OrganizationID {
		dl.Warnf("get tunnel rejected for foreign organization tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
		return &api.GetTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
	}
	if !canManageTunnel(principal, tunnel) {
		accessible, err := s.isTunnelAccessibleToAccount(ctx, tunnel, principal)
		if err != nil {
			dl.Errorf("get tunnel accessibility failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
			return &api.GetTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
		}
		if !accessible {
			dl.Warnf("get tunnel rejected for inaccessible tunnel tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
			return &api.GetTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
	}

	dl.Debugf("got tunnel tunnel_id='%s' name='%s' %s", tunnel.ID, tunnel.Name, principalLogFields(principal))
	return mapTunnel(tunnel), nil
}
