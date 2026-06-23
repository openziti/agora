package automation

import (
	"context"
	"fmt"

	"github.com/openziti/edge-api/rest_model"
)

type TunnelSpec struct {
	OrganizationID string
	AccountID      string
	EnvironmentID  string
	TunnelID       string
	TunnelName     string
	ServiceName    string
	// BindIdentityRoles selects which identities may host (bind) the tunnel's service. A standalone
	// (account-owned) tunnel passes the account role attribute ("#agora-account:<id>") so any of the
	// account's environments may host it; a Layer 2 session tunnel passes a single env identity
	// ("@<envIdentity>"), keeping it environment-scoped.
	BindIdentityRoles []string
	EdgeRouterRoles   []string
	Version           string
}

type ProvisionedTunnel struct {
	ServiceID                 string
	BindPolicyID              string
	ServiceEdgeRouterPolicyID string
}

type TunnelAccessSpec struct {
	OrganizationID        string
	AccountID             string
	EnvironmentID         string
	TunnelID              string
	TunnelName            string
	AttachmentID          string
	EnvironmentIdentityID string
	ServiceID             string
	Version               string
}

type TunnelProvisioner struct {
	services                  ServiceOperations
	servicePolicies           ServicePolicyManagerOperations
	serviceEdgeRouterPolicies ServiceEdgeRouterPolicyOperations
	terminators               TerminatorOperations
}

func NewTunnelProvisioner(client *Client) *TunnelProvisioner {
	return &TunnelProvisioner{
		services:                  client.Services,
		servicePolicies:           client.ServicePolicies,
		serviceEdgeRouterPolicies: client.ServiceEdgeRouterPolicies,
		terminators:               client.Terminators,
	}
}

// EvictTerminatorsByService deletes every fabric terminator advertising the given service, removing
// whatever host is currently bound to it. This is the shape-agnostic eviction takeover uses: a proxy
// tunnel and a direct tunnel both surface their host as terminators on the service.
func (p *TunnelProvisioner) EvictTerminatorsByService(ctx context.Context, serviceID string) error {
	if serviceID == "" {
		return nil
	}
	return p.terminators.DeleteWithFilter(ctx, BuildFilter("service", serviceID))
}

// EvictTerminatorsByIdentity deletes every fabric terminator owned by the given identity, removing
// everything that identity is currently hosting. Environment retirement uses this to clear the
// retiring environment's residue from the standalone tunnels it was hosting -- a direct tunnel has no
// serve row, so the identity-scoped terminator list is the only way to enumerate them.
func (p *TunnelProvisioner) EvictTerminatorsByIdentity(ctx context.Context, identityID string) error {
	if identityID == "" {
		return nil
	}
	return p.terminators.DeleteWithFilter(ctx, BuildFilter("identity", identityID))
}

func (p *TunnelProvisioner) Provision(ctx context.Context, spec TunnelSpec) (*ProvisionedTunnel, error) {
	tags := AgoraTags(spec.Version).
		WithResourceKind("tunnel").
		WithOrganizationID(spec.OrganizationID).
		WithAccountID(spec.AccountID).
		WithEnvironmentID(spec.EnvironmentID).
		WithTunnelID(spec.TunnelID).
		WithTunnelName(spec.TunnelName)

	serviceID, err := p.services.Create(ctx, &ServiceOptions{
		BaseOptions: BaseOptions{
			Name: spec.ServiceName,
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
			Name: serviceID + "-bind",
			Tags: tags,
		},
		IdentityRoles: spec.BindIdentityRoles,
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
		edgeRouterRoles = []string{defaultEdgeRouterRole}
	}
	serviceEdgeRouterPolicyID, err := p.serviceEdgeRouterPolicies.Create(ctx, &ServiceEdgeRouterPolicyOptions{
		BaseOptions: BaseOptions{
			Name: spec.TunnelID + "-serp",
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

	return result, nil
}

func (p *TunnelProvisioner) CreateAttachmentDialPolicy(ctx context.Context, spec TunnelAccessSpec) (string, error) {
	tags := AgoraTags(spec.Version).
		WithResourceKind("tunnel_attachment").
		WithOrganizationID(spec.OrganizationID).
		WithAccountID(spec.AccountID).
		WithEnvironmentID(spec.EnvironmentID).
		WithTunnelID(spec.TunnelID).
		WithTunnelName(spec.TunnelName).
		WithTag("agoraAttachmentId", spec.AttachmentID)

	dialPolicyID, err := p.servicePolicies.CreateDial(ctx, &ServicePolicyOptions{
		BaseOptions: BaseOptions{
			Name: spec.AttachmentID + "-" + spec.ServiceID + "-dial",
			Tags: tags,
		},
		IdentityRoles: []string{"@" + spec.EnvironmentIdentityID},
		ServiceRoles:  []string{"@" + spec.ServiceID},
		Semantic:      rest_model.SemanticAllOf,
	})
	if err != nil {
		return "", fmt.Errorf("create tunnel attachment dial policy: %w", err)
	}
	return dialPolicyID, nil
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
