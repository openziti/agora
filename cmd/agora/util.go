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
	adminToken string
}

func (s staticSecuritySource) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	return api.AccountTokenAuth{}, fmt.Errorf("account token auth not configured for admin command")
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
	baseURL := strings.TrimRight(apiEndpoint, "/") + "/v1"
	client, err := api.NewClient(baseURL, staticSecuritySource{adminToken: adminToken}, api.WithClient(http.DefaultClient))
	if err != nil {
		panic(err)
	}
	return client
}
