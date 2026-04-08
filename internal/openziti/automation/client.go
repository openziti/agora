package automation

import (
	"context"
	"crypto/x509"
	"fmt"

	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_util"
)

type Client struct {
	cfg                       Config
	edge                      *rest_management_api_client.ZitiEdgeManagement
	Identities                IdentityOperations
	Services                  ServiceOperations
	EdgeRouterPolicies        EdgeRouterPolicyOperations
	ServicePolicies           ServicePolicyManagerOperations
	ServiceEdgeRouterPolicies ServiceEdgeRouterPolicyOperations
}

func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	return newClient(ctx, cfg, nil)
}

func newClient(_ context.Context, cfg *Config, auth Authenticator) (*Client, error) {
	normalized := cfg.normalized()
	if normalized.APIEndpoint == "" {
		return nil, fmt.Errorf("openziti apiEndpoint is required")
	}

	if auth == nil {
		var err error
		auth, err = selectAuthenticator(normalized)
		if err != nil {
			return nil, err
		}
	}

	caPool, err := buildCAPool(normalized.APIEndpoint)
	if err != nil {
		return nil, fmt.Errorf("load openziti controller CAs: %w", err)
	}
	edge, err := auth.NewManagementClient(normalized, caPool)
	if err != nil {
		return nil, fmt.Errorf("create openziti management client: %w", err)
	}

	client := &Client{
		cfg:  normalized,
		edge: edge,
	}
	client.Identities = NewIdentityManager(client)
	client.Services = NewServiceManager(client)
	client.EdgeRouterPolicies = NewEdgeRouterPolicyManager(client)
	client.ServicePolicies = NewServicePolicyManager(client)
	client.ServiceEdgeRouterPolicies = NewServiceEdgeRouterPolicyManager(client)
	return client, nil
}

func buildCAPool(apiEndpoint string) (*x509.CertPool, error) {
	certs, err := rest_util.GetControllerWellKnownCas(apiEndpoint)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	for _, cert := range certs {
		pool.AddCert(cert)
	}
	return pool, nil
}

func (c *Client) Edge() *rest_management_api_client.ZitiEdgeManagement {
	return c.edge
}
