package automation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
)

type TunnelSpec struct {
	OrganizationID        uuid.UUID
	EnvironmentID         uuid.UUID
	TunnelID              uuid.UUID
	TunnelName            string
	EnvironmentIdentityID string
	DialIdentityRoles     []string
	EdgeRouterRoles       []string
	Version               string
}

type ProvisionedTunnel struct {
	ServiceID                 string
	BindPolicyID              string
	DialPolicyID              string
	ServiceEdgeRouterPolicyID string
}

type TunnelProvisioner struct {
	services                  ServiceOperations
	servicePolicies           ServicePolicyManagerOperations
	serviceEdgeRouterPolicies ServiceEdgeRouterPolicyOperations
}

func NewTunnelProvisioner(client *Client) *TunnelProvisioner {
	return &TunnelProvisioner{
		services:                  client.Services,
		servicePolicies:           client.ServicePolicies,
		serviceEdgeRouterPolicies: client.ServiceEdgeRouterPolicies,
	}
}

func (p *TunnelProvisioner) Provision(ctx context.Context, spec TunnelSpec) (*ProvisionedTunnel, error) {
	tags := AgoraTags(spec.Version).
		WithResourceKind("tunnel").
		WithOrganizationID(spec.OrganizationID).
		WithEnvironmentID(spec.EnvironmentID).
		WithTunnelID(spec.TunnelID).
		WithTunnelName(spec.TunnelName)

	serviceID, err := p.services.Create(ctx, &ServiceOptions{
		BaseOptions: BaseOptions{
			Name: spec.TunnelName,
			Tags: tags,
		},
		EncryptionRequired: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create tunnel service: %w", err)
	}

	result := &ProvisionedTunnel{ServiceID: serviceID}

	bindPolicyID, err := p.servicePolicies.CreateBind(ctx, &ServicePolicyOptions{
		BaseOptions: BaseOptions{
			Name: spec.EnvironmentIdentityID + "-" + serviceID + "-bind",
			Tags: tags,
		},
		IdentityRoles: []string{"@" + spec.EnvironmentIdentityID},
		ServiceRoles:  []string{"@" + serviceID},
		Semantic:      rest_model.SemanticAllOf,
	})
	if err != nil {
		_ = p.services.Delete(ctx, serviceID)
		return nil, fmt.Errorf("create tunnel bind policy: %w", err)
	}
	result.BindPolicyID = bindPolicyID

	edgeRouterRoles := spec.EdgeRouterRoles
	if len(edgeRouterRoles) == 0 {
		edgeRouterRoles = []string{"#all"}
	}
	serviceEdgeRouterPolicyID, err := p.serviceEdgeRouterPolicies.Create(ctx, &ServiceEdgeRouterPolicyOptions{
		BaseOptions: BaseOptions{
			Name: spec.TunnelName + "-serp",
			Tags: tags,
		},
		ServiceRoles:    []string{"@" + serviceID},
		EdgeRouterRoles: edgeRouterRoles,
		Semantic:        rest_model.SemanticAllOf,
	})
	if err != nil {
		_ = p.servicePolicies.Delete(ctx, bindPolicyID)
		_ = p.services.Delete(ctx, serviceID)
		return nil, fmt.Errorf("create tunnel service-edge-router policy: %w", err)
	}
	result.ServiceEdgeRouterPolicyID = serviceEdgeRouterPolicyID

	if len(spec.DialIdentityRoles) > 0 {
		dialPolicyID, err := p.servicePolicies.CreateDial(ctx, &ServicePolicyOptions{
			BaseOptions: BaseOptions{
				Name: spec.TunnelName + "-dial",
				Tags: tags,
			},
			IdentityRoles: spec.DialIdentityRoles,
			ServiceRoles:  []string{"@" + serviceID},
			Semantic:      rest_model.SemanticAllOf,
		})
		if err != nil {
			_ = p.serviceEdgeRouterPolicies.Delete(ctx, serviceEdgeRouterPolicyID)
			_ = p.servicePolicies.Delete(ctx, bindPolicyID)
			_ = p.services.Delete(ctx, serviceID)
			return nil, fmt.Errorf("create tunnel dial policy: %w", err)
		}
		result.DialPolicyID = dialPolicyID
	}

	return result, nil
}

type DeprovisionTunnelSpec struct {
	ServiceID                 string
	BindPolicyID              string
	DialPolicyID              string
	ServiceEdgeRouterPolicyID string
}

func (p *TunnelProvisioner) Deprovision(ctx context.Context, spec DeprovisionTunnelSpec) error {
	if spec.DialPolicyID != "" {
		if err := p.servicePolicies.Delete(ctx, spec.DialPolicyID); err != nil && !IsNotFound(err) {
			return fmt.Errorf("delete tunnel dial policy: %w", err)
		}
	}
	if spec.ServiceEdgeRouterPolicyID != "" {
		if err := p.serviceEdgeRouterPolicies.Delete(ctx, spec.ServiceEdgeRouterPolicyID); err != nil && !IsNotFound(err) {
			return fmt.Errorf("delete tunnel service-edge-router policy: %w", err)
		}
	}
	if spec.BindPolicyID != "" {
		if err := p.servicePolicies.Delete(ctx, spec.BindPolicyID); err != nil && !IsNotFound(err) {
			return fmt.Errorf("delete tunnel bind policy: %w", err)
		}
	}
	if spec.ServiceID != "" {
		if err := p.services.Delete(ctx, spec.ServiceID); err != nil && !IsNotFound(err) {
			return fmt.Errorf("delete tunnel service: %w", err)
		}
	}
	return nil
}
