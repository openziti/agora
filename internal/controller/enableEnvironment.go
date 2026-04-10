package controller

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) EnableEnvironment(ctx context.Context, req *api.EnableEnvironmentRequest) (api.EnableEnvironmentRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
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

	envLifecycle, _, err := s.lifecycleFactory(ctx)
	if err != nil {
		return &api.EnableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	provisioned, err := envLifecycle.Enable(ctx, automation.EnvironmentSpec{
		OrganizationID: principal.OrganizationID,
		AccountID:      principal.AccountID,
		EnvironmentID:  envID,
		IdentityName:   environmentIdentityName(principal.OrganizationID, principal.AccountID, envID),
		Version:        automation.DefaultAgoraVersion,
	})
	if err != nil {
		return &api.EnableEnvironmentInternalServerError{Code: "internal_error", Message: err.Error()}, nil
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

	created, err := s.store.Environments.Create(ctx, s.store.DB(), env)
	if err != nil {
		_ = envLifecycle.Disable(ctx, automation.DeprovisionEnvironmentSpec{
			IdentityID:         provisioned.IdentityID,
			EdgeRouterPolicyID: provisioned.PolicyID,
		})
		return &api.EnableEnvironmentConflict{Code: "conflict", Message: err.Error()}, nil
	}

	return mapEnableEnvironmentResponse(created, provisioned.EnrollmentJSON), nil
}

func environmentIdentityName(organizationID, accountID, environmentID string) string {
	return fmt.Sprintf("%s-%s-%s", organizationID, accountID, environmentID)
}
