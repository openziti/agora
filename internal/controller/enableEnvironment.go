package controller

import (
	"context"
	"fmt"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) EnableEnvironment(ctx context.Context, req *api.EnableEnvironmentRequest) (api.EnableEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warn("unauthorized enable environment request")
		return &api.EnableEnvironmentUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	envID := persistence.NewResourceID(persistence.PrefixEnvironment)
	host := ""
	if value, ok := req.Host.Get(); ok {
		host = value
	}
	description := ""
	if value, ok := req.Description.Get(); ok {
		description = value
	}
	dl.Infof(
		"enabling environment host='%s' description='%s' environment_id='%s' %s",
		host,
		description,
		envID,
		principalLogFields(principal),
	)

	envLifecycle, _, err := s.lifecycleFactory(ctx)
	if err != nil {
		dl.Errorf("enable environment lifecycle initialization failed environment_id='%s' %s: %v", envID, principalLogFields(principal), err)
		return &api.EnableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	var provisioned *automation.ProvisionedEnvironment
	var created *persistence.Environment
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := lockAccountScope(ctx, tx, principal.AccountID); err != nil {
			return err
		}
		acct, err := s.store.Accounts.GetByIDForUpdate(ctx, tx, principal.AccountID)
		if err != nil {
			return err
		}
		if acct.OrganizationID != principal.OrganizationID {
			return persistence.ErrNotFound
		}

		provisioned, err = envLifecycle.Enable(ctx, automation.EnvironmentSpec{
			OrganizationID: principal.OrganizationID,
			AccountID:      principal.AccountID,
			EnvironmentID:  envID,
			IdentityName:   environmentIdentityName(principal.OrganizationID, principal.AccountID, envID),
			RoleAttributes: environmentRoleAttributes(principal.OrganizationID, principal.AccountID, envID),
			Version:        automation.DefaultAgoraVersion,
		})
		if err != nil {
			dl.Errorf("enable environment provisioning failed environment_id='%s' %s: %v", envID, principalLogFields(principal), err)
			return err
		}

		env := persistence.Environment{
			ID:                 envID,
			OrganizationID:     principal.OrganizationID,
			AccountID:          principal.AccountID,
			ZitiIdentityID:     provisioned.IdentityID,
			State:              persistence.EnvironmentStateEnabled,
			EdgeRouterPolicyID: &provisioned.PolicyID,
		}
		if description != "" {
			env.Description = &description
		}
		if host != "" {
			env.Host = &host
		}

		created, err = s.store.Environments.Create(ctx, tx, env)
		return err
	}); err != nil {
		if provisioned != nil {
			_ = envLifecycle.Disable(ctx, automation.DeprovisionEnvironmentSpec{
				IdentityID:         provisioned.IdentityID,
				EdgeRouterPolicyID: provisioned.PolicyID,
			})
		}
		dl.Errorf("enable environment persistence failed environment_id='%s' %s: %v", envID, principalLogFields(principal), err)
		return &api.EnableEnvironmentConflict{Code: "conflict", Message: err.Error()}, nil
	}
	dl.Infof("enabled environment environment_id='%s' ziti_identity_id='%s' %s", created.ID, created.ZitiIdentityID, principalLogFields(principal))

	return mapEnableEnvironmentResponse(created, provisioned.EnrollmentJSON), nil
}

func environmentIdentityName(organizationID, accountID, environmentID string) string {
	return fmt.Sprintf("%s-%s-%s", organizationID, accountID, environmentID)
}
