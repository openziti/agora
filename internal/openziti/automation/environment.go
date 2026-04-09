package automation

import (
	"context"
	"fmt"

	"github.com/openziti/edge-api/rest_model"
)

type EnvironmentSpec struct {
	OrganizationID string
	AccountID      string
	EnvironmentID  string
	IdentityName   string
	RoleAttributes []string
	Version        string
}

type ProvisionedEnvironment struct {
	IdentityID     string
	EnrollmentJSON []byte
	PolicyID       string
}

type EnvironmentProvisioner struct {
	identities         IdentityOperations
	edgeRouterPolicies EdgeRouterPolicyOperations
}

func NewEnvironmentProvisioner(client *Client) *EnvironmentProvisioner {
	return &EnvironmentProvisioner{
		identities:         client.Identities,
		edgeRouterPolicies: client.EdgeRouterPolicies,
	}
}

func (p *EnvironmentProvisioner) Enable(ctx context.Context, spec EnvironmentSpec) (*ProvisionedEnvironment, error) {
	tags := AgoraTags(spec.Version).
		WithResourceKind("environment").
		WithOrganizationID(spec.OrganizationID).
		WithAccountID(spec.AccountID).
		WithEnvironmentID(spec.EnvironmentID)

	identityID, err := p.identities.Create(ctx, &IdentityOptions{
		BaseOptions: BaseOptions{
			Name: spec.IdentityName,
			Tags: tags,
		},
		Type:           rest_model.IdentityTypeDevice,
		IsAdmin:        false,
		RoleAttributes: spec.RoleAttributes,
	})
	if err != nil {
		return nil, fmt.Errorf("create environment identity: %w", err)
	}

	enrollmentJSON, err := p.identities.Enroll(ctx, identityID)
	if err != nil {
		_ = p.identities.Delete(ctx, identityID)
		return nil, fmt.Errorf("enroll environment identity: %w", err)
	}

	policyID, err := p.edgeRouterPolicies.Create(ctx, &EdgeRouterPolicyOptions{
		BaseOptions: BaseOptions{
			Name: spec.IdentityName,
			Tags: tags,
		},
		IdentityRoles:   []string{"@" + identityID},
		EdgeRouterRoles: []string{"#all"},
		Semantic:        rest_model.SemanticAllOf,
	})
	if err != nil {
		_ = p.identities.Delete(ctx, identityID)
		return nil, fmt.Errorf("create environment edge-router policy: %w", err)
	}

	return &ProvisionedEnvironment{
		IdentityID:     identityID,
		EnrollmentJSON: enrollmentJSON,
		PolicyID:       policyID,
	}, nil
}

type DeprovisionEnvironmentSpec struct {
	IdentityID         string
	EdgeRouterPolicyID string
}

func (p *EnvironmentProvisioner) Disable(ctx context.Context, spec DeprovisionEnvironmentSpec) error {
	if spec.EdgeRouterPolicyID != "" {
		if err := p.edgeRouterPolicies.Delete(ctx, spec.EdgeRouterPolicyID); err != nil && !IsNotFound(err) {
			return fmt.Errorf("delete environment edge-router policy: %w", err)
		}
	}
	if spec.IdentityID != "" {
		if err := p.identities.Delete(ctx, spec.IdentityID); err != nil && !IsNotFound(err) {
			return fmt.Errorf("delete environment identity: %w", err)
		}
	}
	return nil
}
