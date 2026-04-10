package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListTunnels(ctx context.Context, params api.ListTunnelsParams) (api.ListTunnelsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warn("unauthorized list tunnels request")
		return &api.ListTunnelsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	scope := params.Scope.Or(api.ListTunnelsScopeOwned)
	dl.Debugf("listing tunnels scope='%s' %s", scope, principalLogFields(principal))

	var tunnels []persistence.Tunnel
	switch scope {
	case api.ListTunnelsScopeAccessible:
		tunnels, err = s.store.Tunnels.ListAccessibleByAccount(ctx, s.store.DB(), principal.OrganizationID, principal.AccountID, false)
	case api.ListTunnelsScopeAll:
		if principal.Role == persistence.AccountRoleAdmin {
			tunnels, err = s.store.Tunnels.ListByOrganization(ctx, s.store.DB(), principal.OrganizationID)
		} else {
			tunnels, err = s.store.Tunnels.ListAccessibleByAccount(ctx, s.store.DB(), principal.OrganizationID, principal.AccountID, true)
		}
	default:
		tunnels, err = s.store.Tunnels.ListOwnedByAccount(ctx, s.store.DB(), principal.OrganizationID, principal.AccountID)
	}
	if err != nil {
		dl.Errorf("list tunnels failed scope='%s' %s: %v", scope, principalLogFields(principal), err)
		return &api.ListTunnelsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	resp := make(api.ListTunnelsResponse, 0, len(tunnels))
	for i := range tunnels {
		resp = append(resp, *mapTunnel(&tunnels[i]))
	}
	dl.Infof("listed %d tunnel(s) scope='%s' %s", len(resp), scope, principalLogFields(principal))
	return &resp, nil
}
