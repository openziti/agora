package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) DeleteTunnel(ctx context.Context, params api.DeleteTunnelParams) (api.DeleteTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized delete tunnel request tunnel_id='%s'", params.TunnelId)
		return &api.DeleteTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("deleting tunnel tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
	if err := s.store.Tunnels.Delete(ctx, s.store.DB(), params.TunnelId, principal.OrganizationID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("delete tunnel not found tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
			return &api.DeleteTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		dl.Errorf("delete tunnel failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.DeleteTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("deleted tunnel tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
	return &api.DeleteTunnelNoContent{}, nil
}
