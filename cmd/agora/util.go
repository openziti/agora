package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func openStore(cfg persistence.Config) *persistence.Store {
	store, err := persistence.Open(context.Background(), cfg)
	if err != nil {
		panic(err)
	}
	return store
}

func appliedLabel(applied bool) string {
	if applied {
		return "applied"
	}
	return "pending"
}

type staticSecuritySource struct {
	accountToken string
	adminToken   string
}

func (s staticSecuritySource) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	if s.accountToken == "" {
		return api.AccountTokenAuth{}, fmt.Errorf("account token auth not configured")
	}
	return api.AccountTokenAuth{APIKey: s.accountToken}, nil
}

func (s staticSecuritySource) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{APIKey: s.adminToken}, nil
}

func openAdminAPIClient() *api.Client {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}
	apiEndpoint, from := root.APIEndpoint()
	if apiEndpoint == "" {
		panic("api endpoint is not configured; set AGORA_API_ENDPOINT or run 'agora config set api_endpoint <url>'")
	}
	adminToken := os.Getenv("AGORA_ADMIN_TOKEN")
	if adminToken == "" {
		panic("please set AGORA_ADMIN_TOKEN to a valid admin token")
	}
	_ = from
	client, err := newAPIClient(apiEndpoint, staticSecuritySource{adminToken: adminToken})
	if err != nil {
		panic(err)
	}
	return client
}

func openAccountAPIClient(apiEndpoint, accountToken string) *api.Client {
	client, err := newAPIClient(apiEndpoint, staticSecuritySource{accountToken: accountToken})
	if err != nil {
		panic(err)
	}
	return client
}

func newAPIClient(apiEndpoint string, source staticSecuritySource) (*api.Client, error) {
	baseURL := strings.TrimRight(apiEndpoint, "/") + "/v1"
	return api.NewClient(baseURL, source, api.WithClient(http.DefaultClient))
}
