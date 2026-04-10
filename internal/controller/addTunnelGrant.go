package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) AddTunnelGrant(ctx context.Context, req *api.AddTunnelGrantRequest, params api.AddTunnelGrantParams) (api.AddTunnelGrantRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized add tunnel grant request tunnel_id='%s'", params.TunnelId)
		return &api.AddTunnelGrantUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("adding tunnel grant tunnel_id='%s' email='%s' %s", params.TunnelId, normalizeEmail(req.Email), principalLogFields(principal))

	tunnel, err := s.requireManagedTunnel(ctx, principal, params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("add tunnel grant tunnel not found tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
			return &api.AddTunnelGrantNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		dl.Errorf("add tunnel grant tunnel lookup failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.AddTunnelGrantInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	email := normalizeEmail(req.Email)
	acct, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), email)
	if err != nil || acct.OrganizationID != principal.OrganizationID {
		dl.Warnf("add tunnel grant account not found tunnel_id='%s' email='%s' %s", tunnel.ID, email, principalLogFields(principal))
		return &api.AddTunnelGrantNotFound{Code: "not_found", Message: fmt.Sprintf("grant account '%s' not found", email)}, nil
	}
	if acct.Status != persistence.AccountStatusActive {
		dl.Warnf("add tunnel grant rejected for disabled account tunnel_id='%s' email='%s' %s", tunnel.ID, email, principalLogFields(principal))
		return &api.AddTunnelGrantConflict{Code: "conflict", Message: fmt.Sprintf("grant account '%s' is disabled", email)}, nil
	}
	if acct.ID == tunnel.AccountID {
		return &api.AddTunnelGrantConflict{Code: "conflict", Message: "tunnel owner already has access"}, nil
	}

	grant, err := s.store.TunnelGrants.Create(ctx, s.store.DB(), persistence.TunnelAccountGrant{
		TunnelID:       tunnel.ID,
		AccountID:      acct.ID,
		OrganizationID: acct.OrganizationID,
	})
	if err != nil {
		dl.Errorf("add tunnel grant failed tunnel_id='%s' email='%s' %s: %v", tunnel.ID, email, principalLogFields(principal), err)
		return &api.AddTunnelGrantConflict{Code: "conflict", Message: err.Error()}, nil
	}

	resp := mapTunnelGrant(&persistence.TunnelGrant{
		TunnelID:  grant.TunnelID,
		AccountID: grant.AccountID,
		Email:     acct.Email,
		CreatedAt: grant.CreatedAt,
	})
	dl.Infof("added tunnel grant tunnel_id='%s' account_id='%s' %s", tunnel.ID, acct.ID, principalLogFields(principal))
	return resp, nil
}
