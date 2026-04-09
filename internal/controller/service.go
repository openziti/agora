package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/persistence"
)

type Service struct {
	cfg   *config.Config
	store *persistence.Store
}

type accountPrincipal struct {
	AccountID      string
	OrganizationID string
	Email          string
	Role           persistence.AccountRole
}

type adminPrincipal struct {
	Token string
}

func NewService(cfg *config.Config, store *persistence.Store) *Service {
	return &Service{cfg: cfg, store: store}
}

func NewHandler(svc *Service) (http.Handler, error) {
	return api.NewServer(svc, svc, api.WithPathPrefix("/v1"))
}

func (s *Service) HandleAccountTokenAuth(ctx context.Context, _ api.OperationName, token api.AccountTokenAuth) (context.Context, error) {
	acct, err := s.store.Accounts.FindByToken(ctx, s.store.DB(), token.APIKey)
	if err != nil {
		return ctx, err
	}
	if acct.Status != persistence.AccountStatusActive {
		return ctx, errors.New("account disabled")
	}
	return context.WithValue(ctx, accountPrincipalKey{}, &accountPrincipal{
		AccountID:      acct.ID,
		OrganizationID: acct.OrganizationID,
		Email:          acct.Email,
		Role:           acct.Role,
	}), nil
}

func (s *Service) HandleAdminTokenAuth(ctx context.Context, _ api.OperationName, token api.AdminTokenAuth) (context.Context, error) {
	for _, allowed := range s.cfg.AdminTokens {
		if token.APIKey == allowed {
			return context.WithValue(ctx, adminPrincipalKey{}, &adminPrincipal{Token: token.APIKey}), nil
		}
	}
	return ctx, errors.New("unauthorized admin token")
}

type accountPrincipalKey struct{}
type adminPrincipalKey struct{}

func requireAccountPrincipal(ctx context.Context) (*accountPrincipal, error) {
	principal, ok := ctx.Value(accountPrincipalKey{}).(*accountPrincipal)
	if !ok || principal == nil {
		return nil, errors.New("missing account principal")
	}
	return principal, nil
}

func requireAdminPrincipal(ctx context.Context) (*adminPrincipal, error) {
	principal, ok := ctx.Value(adminPrincipalKey{}).(*adminPrincipal)
	if !ok || principal == nil {
		return nil, errors.New("missing admin principal")
	}
	return principal, nil
}

func mapOrganization(org *persistence.Organization) *api.Organization {
	return &api.Organization{
		ID:        org.ID,
		Name:      org.Name,
		CreatedAt: org.CreatedAt,
		UpdatedAt: org.UpdatedAt,
	}
}

func mapAccount(acct *persistence.Account) *api.Account {
	result := &api.Account{
		ID:             acct.ID,
		OrganizationId: acct.OrganizationID,
		Email:          acct.Email,
		Role:           api.AccountRole(acct.Role),
		Status:         api.AccountStatus(acct.Status),
		CreatedAt:      acct.CreatedAt,
		UpdatedAt:      acct.UpdatedAt,
	}
	if acct.DisplayName != nil {
		result.DisplayName.SetTo(*acct.DisplayName)
	}
	return result
}

func mapEnvironment(env *persistence.Environment) *api.Environment {
	result := &api.Environment{
		ID:             env.ID,
		OrganizationId: env.OrganizationID,
		AccountId:      env.AccountID,
		ZitiIdentityId: env.ZitiIdentityID,
		State:          api.EnvironmentState(env.State),
		CreatedAt:      env.CreatedAt,
		UpdatedAt:      env.UpdatedAt,
	}
	if env.Description != nil {
		result.Description.SetTo(*env.Description)
	}
	if env.Host != nil {
		result.Host.SetTo(*env.Host)
	}
	if env.LastSeenAt != nil {
		result.LastSeenAt.SetTo(*env.LastSeenAt)
	}
	return result
}

func mapTunnel(tunnel *persistence.Tunnel) *api.Tunnel {
	result := &api.Tunnel{
		ID:             tunnel.ID,
		OrganizationId: tunnel.OrganizationID,
		EnvironmentId:  tunnel.EnvironmentID,
		Name:           tunnel.Name,
		BackendAddress: tunnel.BackendAddress,
		State:          api.TunnelState(tunnel.State),
		CreatedAt:      tunnel.CreatedAt,
		UpdatedAt:      tunnel.UpdatedAt,
	}
	if tunnel.ZitiServiceID != nil {
		result.ZitiServiceId.SetTo(*tunnel.ZitiServiceID)
	}
	return result
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
