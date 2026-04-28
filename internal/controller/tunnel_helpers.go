package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openziti/agora/internal/persistence"
)

func environmentRoleAttributes(organizationID, accountID, environmentID string) []string {
	return []string{
		organizationRoleAttribute(organizationID),
		accountRoleAttribute(accountID),
		environmentRoleAttribute(environmentID),
	}
}

func organizationRoleAttribute(organizationID string) string {
	return "agora-org:" + organizationID
}

func accountRoleAttribute(accountID string) string {
	return "agora-account:" + accountID
}

func environmentRoleAttribute(environmentID string) string {
	return "agora-environment:" + environmentID
}

func accountRoleSelector(accountID string) string {
	return "#" + accountRoleAttribute(accountID)
}

func environmentRoleSelector(environmentID string) string {
	return "#" + environmentRoleAttribute(environmentID)
}

func tunnelServiceName(tunnelID string) string {
	return tunnelID
}

func optionalStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func canManageTunnel(principal *accountPrincipal, tunnel *persistence.Tunnel) bool {
	return tunnel.AccountID == principal.AccountID || principal.Role == persistence.AccountRoleAdmin
}

func (s *Service) requireManagedTunnel(ctx context.Context, principal *accountPrincipal, tunnelID string) (*persistence.Tunnel, error) {
	tunnel, err := s.store.Tunnels.GetByID(ctx, s.store.DB(), tunnelID)
	if err != nil {
		return nil, err
	}
	if tunnel.OrganizationID != principal.OrganizationID {
		return nil, persistence.ErrNotFound
	}
	if !canManageTunnel(principal, tunnel) {
		return nil, persistence.ErrNotFound
	}
	return tunnel, nil
}

func (s *Service) isTunnelAccessibleToAccount(ctx context.Context, tunnel *persistence.Tunnel, principal *accountPrincipal) (bool, error) {
	if tunnel.OrganizationID != principal.OrganizationID {
		return false, nil
	}
	if tunnel.AccountID == principal.AccountID {
		return true, nil
	}
	granted, err := s.store.TunnelGrants.IsGranted(ctx, s.store.DB(), tunnel.ID, principal.AccountID)
	if err != nil {
		return false, err
	}
	return granted, nil
}

func (s *Service) lookupTunnelByName(ctx context.Context, principal *accountPrincipal, name string) (*persistence.Tunnel, error) {
	trimmed := trimTunnelName(name)
	// Same-org lookup first.
	if tunnel, err := s.store.Tunnels.GetByName(ctx, s.store.DB(), principal.OrganizationID, trimmed); err == nil {
		if tunnel.OrganizationID == principal.OrganizationID {
			return tunnel, nil
		}
	} else if !errors.Is(err, persistence.ErrNotFound) {
		return nil, err
	}
	// Cross-org fallback: surface tunnels granted to the caller's
	// account regardless of which org owns them. Used by inter-org
	// sessions where the consumer dials a tunnel owned by the
	// provider's organization. canConnectToTunnel still gates the
	// access decision.
	if tunnel, err := s.store.Tunnels.GetByNameGrantedToAccount(ctx, s.store.DB(), trimmed, principal.AccountID); err == nil {
		return tunnel, nil
	} else if !errors.Is(err, persistence.ErrNotFound) {
		return nil, err
	}
	return nil, persistence.ErrNotFound
}

func (s *Service) canConnectToTunnel(ctx context.Context, tunnel *persistence.Tunnel, env *persistence.Environment, principal *accountPrincipal) (bool, error) {
	// The caller's environment must always belong to the caller's
	// organization (each account enrolls envs only in its own org).
	if env.OrganizationID != principal.OrganizationID {
		return false, nil
	}
	// Same-org caller is the tunnel owner: full access.
	if tunnel.OrganizationID == principal.OrganizationID &&
		tunnel.EnvironmentID == env.ID &&
		tunnel.AccountID == principal.AccountID {
		return true, nil
	}
	// Same-org caller, same account but different env: not allowed.
	if tunnel.OrganizationID == principal.OrganizationID && principal.AccountID == tunnel.AccountID {
		return false, nil
	}
	// Same- or cross-org: grants table is authoritative.
	return s.store.TunnelGrants.IsGranted(ctx, s.store.DB(), tunnel.ID, principal.AccountID)
}

func (s *Service) requireOwnedEnvironment(ctx context.Context, principal *accountPrincipal, environmentID string) (*persistence.Environment, error) {
	env, err := s.store.Environments.GetByID(ctx, s.store.DB(), environmentID)
	if err != nil {
		return nil, err
	}
	if env.OrganizationID != principal.OrganizationID || env.AccountID != principal.AccountID {
		return nil, persistence.ErrNotFound
	}
	return env, nil
}

func (s *Service) resolveGrantAccounts(ctx context.Context, principal *accountPrincipal, emails []string) ([]persistence.Account, error) {
	seen := map[string]struct{}{}
	result := make([]persistence.Account, 0, len(emails))
	for _, raw := range emails {
		email := normalizeEmail(raw)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}

		acct, err := s.store.Accounts.FindByEmail(ctx, s.store.DB(), email)
		if err != nil {
			return nil, fmt.Errorf("grant account '%s' not found", email)
		}
		if acct.OrganizationID != principal.OrganizationID {
			return nil, fmt.Errorf("grant account '%s' not found", email)
		}
		if acct.ID == principal.AccountID {
			continue
		}
		if acct.Status != persistence.AccountStatusActive {
			return nil, fmt.Errorf("grant account '%s' is disabled", email)
		}
		result = append(result, *acct)
	}
	return result, nil
}

func trimTunnelName(name string) string {
	return strings.TrimSpace(name)
}
