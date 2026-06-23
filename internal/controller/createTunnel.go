package controller

import (
	"context"
	"errors"
	"strings"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) CreateTunnel(ctx context.Context, req *api.CreateTunnelRequest) (api.CreateTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized create tunnel request environment_id='%s' name='%s'", req.EnvironmentId.Or(""), req.Name)
		return &api.CreateTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	kind, backendTarget, err := inferTunnelKind(req.BackendTarget)
	if err != nil {
		dl.Warnf("create tunnel rejected due to invalid backend target name='%s' %s: %v", req.Name, principalLogFields(principal), err)
		return &api.CreateTunnelConflict{Code: "conflict", Message: err.Error()}, nil
	}
	dl.Infof(
		"creating tunnel name='%s' environment_id='%s' mode='%s' kind='%s' backend_target='%s' %s",
		req.Name,
		req.EnvironmentId.Or(""),
		req.Mode,
		kind,
		optionalStringValue(backendTarget),
		principalLogFields(principal),
	)

	existing, err := s.store.Tunnels.GetByName(ctx, s.store.DB(), principal.OrganizationID, req.Name)
	if err == nil {
		dl.Warnf(
			"create tunnel rejected due to existing name name='%s' existing_tunnel_id='%s' existing_environment_id='%s' %s",
			req.Name,
			existing.ID,
			optionalStringValue(existing.EnvironmentID),
			principalLogFields(principal),
		)
		return &api.CreateTunnelConflict{Code: "conflict", Message: "tunnel name already exists"}, nil
	}
	if !errors.Is(err, persistence.ErrNotFound) {
		dl.Errorf("create tunnel existing-name lookup failed name='%s' %s: %v", req.Name, principalLogFields(principal), err)
		return &api.CreateTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	grantedAccounts, err := s.resolveGrantAccounts(ctx, principal, req.GrantEmails)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("create tunnel grant resolution failed name='%s' %s: %v", req.Name, principalLogFields(principal), err)
			return &api.CreateTunnelNotFound{Code: "not_found", Message: err.Error()}, nil
		}
		dl.Errorf("create tunnel grant resolution failed name='%s' %s: %v", req.Name, principalLogFields(principal), err)
		return &api.CreateTunnelConflict{Code: "conflict", Message: err.Error()}, nil
	}

	tunnelID := persistence.NewResourceID(persistence.PrefixTunnel)
	serviceName := tunnelServiceName(tunnelID)

	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		dl.Errorf("create tunnel lifecycle initialization failed tunnel_id='%s' %s: %v", tunnelID, principalLogFields(principal), err)
		return &api.CreateTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	var created *persistence.Tunnel
	var provisioned *automation.ProvisionedTunnel
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		// a standalone tunnel is account-owned and never bound to an environment. a supplied
		// environmentId is treated as caller context only: validate the caller owns it, then discard
		// it. the created row always stores EnvironmentID = nil regardless of what the caller sent
		// (finding R2-A1) -- a persisted environment_id would masquerade as a Layer 2 session tunnel
		// under the null/set discriminator and silently reintroduce environment binding.
		if envID, ok := req.EnvironmentId.Get(); ok {
			if err := lockEnvironmentScope(ctx, tx, envID); err != nil {
				return err
			}
			if _, err := s.requireOwnedEnvironmentWithQuery(ctx, tx, principal, envID); err != nil {
				return err
			}
		}
		if _, err := s.store.Tunnels.GetByName(ctx, tx, principal.OrganizationID, req.Name); err == nil {
			return errTunnelNameExists
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return err
		}

		provisioned, err = tunnelLifecycle.Provision(ctx, automation.TunnelSpec{
			OrganizationID:    principal.OrganizationID,
			AccountID:         principal.AccountID,
			TunnelID:          tunnelID,
			TunnelName:        req.Name,
			ServiceName:       serviceName,
			BindIdentityRoles: []string{"#" + accountRoleAttribute(principal.AccountID)},
			Version:           automation.DefaultAgoraVersion,
		})
		if err != nil {
			dl.Errorf("create tunnel provisioning failed tunnel_id='%s' %s: %v", tunnelID, principalLogFields(principal), err)
			return err
		}

		tunnel := persistence.Tunnel{
			ID:                        tunnelID,
			OrganizationID:            principal.OrganizationID,
			AccountID:                 principal.AccountID,
			EnvironmentID:             nil,
			Name:                      req.Name,
			Mode:                      persistence.TunnelMode(req.Mode),
			Kind:                      kind,
			BackendTarget:             backendTarget,
			ZitiServiceID:             &provisioned.ServiceID,
			BindPolicyID:              &provisioned.BindPolicyID,
			ServiceEdgeRouterPolicyID: &provisioned.ServiceEdgeRouterPolicyID,
			State:                     persistence.TunnelStateActive,
		}

		created, err = s.store.Tunnels.Create(ctx, tx, tunnel)
		if err != nil {
			return err
		}
		for _, acct := range grantedAccounts {
			if _, err := s.store.TunnelGrants.Create(ctx, tx, persistence.TunnelAccountGrant{
				TunnelID:       created.ID,
				AccountID:      acct.ID,
				OrganizationID: acct.OrganizationID,
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		dl.Errorf("create tunnel persistence failed tunnel_id='%s' %s: %v", tunnelID, principalLogFields(principal), err)
		if provisioned != nil {
			_ = tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{
				ServiceID:                 provisioned.ServiceID,
				BindPolicyID:              provisioned.BindPolicyID,
				ServiceEdgeRouterPolicyID: provisioned.ServiceEdgeRouterPolicyID,
			})
		}
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("create tunnel environment not found environment_id='%s' %s", req.EnvironmentId.Or(""), principalLogFields(principal))
			return &api.CreateTunnelNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		if errors.Is(err, errTunnelNameExists) {
			return &api.CreateTunnelConflict{Code: "conflict", Message: "tunnel name already exists"}, nil
		}
		return &api.CreateTunnelConflict{Code: "conflict", Message: err.Error()}, nil
	}

	dl.Infof("created tunnel id='%s' name='%s' environment_id='%s' %s", created.ID, created.Name, optionalStringValue(created.EnvironmentID), principalLogFields(principal))
	return mapTunnel(created), nil
}

var errTunnelNameExists = errors.New("tunnel name already exists")

func inferTunnelKind(backend api.OptString) (persistence.TunnelKind, *string, error) {
	if !backend.Set {
		return persistence.TunnelKindDirect, nil, nil
	}
	trimmed := strings.TrimSpace(backend.Value)
	if trimmed == "" {
		return "", nil, errors.New("backendTarget must be non-empty when present")
	}
	return persistence.TunnelKindProxy, &trimmed, nil
}
