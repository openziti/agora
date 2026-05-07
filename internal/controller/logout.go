package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) Logout(ctx context.Context) error {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return nil
	}

	acct := persistence.Account{
		ID:             principal.AccountID,
		OrganizationID: principal.OrganizationID,
		Email:          principal.Email,
	}
	if err := s.store.AuditEvents.Record(ctx, s.store.DB(), persistence.NewAccountLogoutEvent(acct)); err != nil {
		dl.Warnf("account logout audit failed account_id='%s' organization_id='%s': %v", principal.AccountID, principal.OrganizationID, err)
	}

	return nil
}
