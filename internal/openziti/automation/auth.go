package automation

import (
	"crypto/x509"
	"fmt"

	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_util"
)

type Authenticator interface {
	NewManagementClient(cfg Config, caPool *x509.CertPool) (*rest_management_api_client.ZitiEdgeManagement, error)
}

type UPDBAuthenticator struct{}

func (a *UPDBAuthenticator) NewManagementClient(cfg Config, caPool *x509.CertPool) (*rest_management_api_client.ZitiEdgeManagement, error) {
	return rest_util.NewEdgeManagementClientWithUpdb(
		cfg.Auth.UPDB.Username,
		cfg.Auth.UPDB.Password,
		cfg.APIEndpoint,
		caPool,
	)
}

func selectAuthenticator(cfg Config) (Authenticator, error) {
	mode := cfg.Auth.Mode
	if mode == "" || mode == "updb" {
		if cfg.Auth.UPDB.Username == "" {
			return nil, fmt.Errorf("openziti auth.updb.username is required")
		}
		if cfg.Auth.UPDB.Password == "" {
			return nil, fmt.Errorf("openziti auth.updb.password is required")
		}
		return &UPDBAuthenticator{}, nil
	}
	return nil, fmt.Errorf("unsupported openziti auth mode %q", mode)
}
