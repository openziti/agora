package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

var errAccountOwnsStandaloneTunnels = errors.New("account still owns standalone tunnels")

func (s *Service) DeleteAccount(ctx context.Context, params api.DeleteAccountParams) (api.DeleteAccountRes, error) {
	if _, err := requireAdminPrincipal(ctx); err != nil {
		dl.Warn("unauthorized delete account request")
		return &api.DeleteAccountUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("deleting account account_id='%s' organization_id='%s' %s", params.AccountId, params.OrganizationId, adminLogFields())

	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := lockAccountScope(ctx, tx, params.AccountId); err != nil {
			return err
		}
		locked, err := s.store.Accounts.GetByIDForUpdate(ctx, tx, params.AccountId)
		if err != nil {
			return err
		}
		if locked.OrganizationID != params.OrganizationId {
			return persistence.ErrNotFound
		}
		active, err := s.store.Sessions.HasActiveTunnelForAccount(ctx, tx, params.AccountId)
		if err != nil {
			return err
		}
		if active {
			return persistence.ErrConflict
		}
		// account-owning a standalone tunnel makes the account the deprovision-responsible party.
		// standalone tunnels (NULL env_id) are not reached by environment-retire cleanup below, so a
		// bare account cascade would strand their OpenZiti service / bind policy / terminators. refuse
		// while any remain; the operator deletes them first via deleteTunnel, which deprovisions their
		// fabric objects (finding R2-C2). full automatic teardown is deferred.
		standalone, err := s.store.Tunnels.HasStandaloneByAccount(ctx, tx, locked.ID)
		if err != nil {
			return err
		}
		if standalone {
			return errAccountOwnsStandaloneTunnels
		}
		envs, err := s.store.Environments.ListByAccount(ctx, tx, locked.ID)
		if err != nil {
			return err
		}
		for i := range envs {
			if envs[i].OrganizationID != locked.OrganizationID {
				continue
			}
			if err := s.disableEnvironmentWithLocks(ctx, tx, envs[i].ID, locked.OrganizationID, locked.ID, adminLogFields()); err != nil {
				return err
			}
		}
		return s.store.Accounts.Delete(ctx, tx, params.OrganizationId, params.AccountId)
	}); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("delete account not found account_id='%s' organization_id='%s'", params.AccountId, params.OrganizationId)
			return &api.DeleteAccountNotFound{Code: "not_found", Message: "account not found"}, nil
		}
		if errors.Is(err, errAccountOwnsStandaloneTunnels) {
			dl.Warnf("delete account conflict account_id='%s' organization_id='%s': standalone tunnels remain", params.AccountId, params.OrganizationId)
			return &api.DeleteAccountConflict{Code: "conflict", Message: "account still owns standalone tunnels; delete them first"}, nil
		}
		if errors.Is(err, persistence.ErrConflict) {
			dl.Warnf("delete account conflict account_id='%s' organization_id='%s': active sessions remain", params.AccountId, params.OrganizationId)
			return &api.DeleteAccountConflict{Code: "conflict", Message: "account still participates in active sessions"}, nil
		}
		dl.Errorf("delete account guard failed account_id='%s' organization_id='%s': %v", params.AccountId, params.OrganizationId, err)
		return &api.DeleteAccountInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	dl.Infof("deleted account account_id='%s' organization_id='%s'", params.AccountId, params.OrganizationId)
	return &api.DeleteAccountNoContent{}, nil
}
