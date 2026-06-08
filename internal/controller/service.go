package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

type Service struct {
	cfg              *config.Config
	store            *persistence.Store
	lifecycleFactory environmentLifecycleFactory
}

type environmentLifecycle interface {
	Enable(context.Context, automation.EnvironmentSpec) (*automation.ProvisionedEnvironment, error)
	Disable(context.Context, automation.DeprovisionEnvironmentSpec) error
}

type tunnelLifecycle interface {
	Provision(context.Context, automation.TunnelSpec) (*automation.ProvisionedTunnel, error)
	CreateAttachmentDialPolicy(context.Context, automation.TunnelAccessSpec) (string, error)
	Deprovision(context.Context, automation.DeprovisionTunnelSpec) error
}

type environmentLifecycleFactory func(context.Context) (environmentLifecycle, tunnelLifecycle, error)

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
	return &Service{
		cfg:              cfg,
		store:            store,
		lifecycleFactory: defaultEnvironmentLifecycleFactory(cfg),
	}
}

func NewHandler(svc *Service) (http.Handler, error) {
	return api.NewServer(svc, svc, api.WithPathPrefix("/v1"))
}

func (s *Service) HandleAccountTokenAuth(ctx context.Context, _ api.OperationName, token api.AccountTokenAuth) (context.Context, error) {
	return s.attachAccountPrincipal(ctx, token.APIKey)
}

func (s *Service) attachAccountPrincipal(ctx context.Context, token string) (context.Context, error) {
	acct, err := s.store.Accounts.FindByToken(ctx, s.store.DB(), token)
	if err != nil {
		dl.Warnf("account token authentication failed: %v", err)
		return ctx, err
	}
	if acct.Status != persistence.AccountStatusActive {
		dl.Warnf(
			"account token authentication rejected for email='%s' account_id='%s' organization_id='%s': account disabled",
			acct.Email,
			acct.ID,
			acct.OrganizationID,
		)
		return ctx, errors.New("account disabled")
	}
	dl.Debugf(
		"authenticated account token for email='%s' account_id='%s' organization_id='%s'",
		acct.Email,
		acct.ID,
		acct.OrganizationID,
	)
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
			dl.Debug("authenticated admin token")
			return context.WithValue(ctx, adminPrincipalKey{}, &adminPrincipal{Token: token.APIKey}), nil
		}
	}
	dl.Warn("admin token authentication failed")
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

func mapEnableEnvironmentResponse(env *persistence.Environment, enrollmentJSON []byte) *api.EnableEnvironmentResponse {
	return &api.EnableEnvironmentResponse{
		Environment:    *mapEnvironment(env),
		EnrollmentJson: string(enrollmentJSON),
	}
}

func mapTunnel(tunnel *persistence.Tunnel) *api.Tunnel {
	kind := tunnel.Kind
	if kind == "" {
		kind = persistence.TunnelKindProxy
	}
	result := &api.Tunnel{
		ID:             tunnel.ID,
		OrganizationId: tunnel.OrganizationID,
		AccountId:      tunnel.AccountID,
		EnvironmentId:  tunnel.EnvironmentID,
		Name:           tunnel.Name,
		Mode:           api.TunnelMode(tunnel.Mode),
		Kind:           api.TunnelKind(kind),
		State:          api.TunnelState(tunnel.State),
		CreatedAt:      tunnel.CreatedAt,
		UpdatedAt:      tunnel.UpdatedAt,
	}
	if tunnel.BackendTarget != nil {
		result.BackendTarget.SetTo(*tunnel.BackendTarget)
	}
	if tunnel.ZitiServiceID != nil {
		result.ZitiServiceId.SetTo(*tunnel.ZitiServiceID)
	}
	if tunnel.BindPolicyID != nil {
		result.BindPolicyId.SetTo(*tunnel.BindPolicyID)
	}
	if tunnel.ServiceEdgeRouterPolicyID != nil {
		result.ServiceEdgeRouterPolicyId.SetTo(*tunnel.ServiceEdgeRouterPolicyID)
	}
	return result
}

func mapTunnelGrant(grant *persistence.TunnelGrant) *api.TunnelGrant {
	return &api.TunnelGrant{
		TunnelId:  grant.TunnelID,
		AccountId: grant.AccountID,
		Email:     grant.Email,
		CreatedAt: grant.CreatedAt,
	}
}

func mapTunnelAttachment(attachment *persistence.TunnelAttachment) *api.TunnelAttachment {
	result := &api.TunnelAttachment{
		ID:              attachment.ID,
		TunnelId:        attachment.TunnelID,
		OrganizationId:  attachment.OrganizationID,
		AccountId:       attachment.AccountID,
		EnvironmentId:   attachment.EnvironmentID,
		ListenAddress:   attachment.ListenAddress,
		State:           api.TunnelAttachmentState(attachment.State),
		LastHeartbeatAt: attachment.LastHeartbeatAt,
		CreatedAt:       attachment.CreatedAt,
		UpdatedAt:       attachment.UpdatedAt,
	}
	if attachment.DialPolicyID != nil {
		result.DialPolicyId.SetTo(*attachment.DialPolicyID)
	}
	if attachment.DisconnectedAt != nil {
		result.DisconnectedAt.SetTo(*attachment.DisconnectedAt)
	}
	return result
}

func mapTunnelAttachmentDetail(attachment *persistence.TunnelAttachmentDetail) *api.TunnelAttachment {
	result := mapTunnelAttachment(&attachment.TunnelAttachment)
	result.AccountEmail.SetTo(attachment.AccountEmail)
	result.TunnelName.SetTo(attachment.TunnelName)
	result.TunnelMode.SetTo(api.TunnelMode(attachment.TunnelMode))
	return result
}

func mapTunnelServe(serve *persistence.TunnelServeDetail) *api.TunnelServe {
	result := &api.TunnelServe{
		ID:              serve.ID,
		TunnelId:        serve.TunnelID,
		OrganizationId:  serve.OrganizationID,
		AccountId:       serve.AccountID,
		EnvironmentId:   serve.EnvironmentID,
		State:           api.TunnelServeState(serve.State),
		LastHeartbeatAt: serve.LastHeartbeatAt,
		CreatedAt:       serve.CreatedAt,
		UpdatedAt:       serve.UpdatedAt,
	}
	if serve.EnvironmentHost != nil {
		result.EnvironmentHost.SetTo(*serve.EnvironmentHost)
	}
	if serve.TunnelName != "" {
		result.TunnelName.SetTo(serve.TunnelName)
	}
	if serve.TunnelMode != "" {
		result.TunnelMode.SetTo(api.TunnelMode(serve.TunnelMode))
	}
	if serve.DisconnectedAt != nil {
		result.DisconnectedAt.SetTo(*serve.DisconnectedAt)
	}
	return result
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func defaultEnvironmentLifecycleFactory(cfg *config.Config) environmentLifecycleFactory {
	return func(ctx context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		client, err := openZitiClient(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}
		return automation.NewEnvironmentProvisioner(client), automation.NewTunnelProvisioner(client), nil
	}
}
