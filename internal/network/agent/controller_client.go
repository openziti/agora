package agent

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/openziti/agora/internal/api"
)

type accountSecuritySource struct {
	accountToken string
}

func (s accountSecuritySource) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	if s.accountToken == "" {
		return api.AccountTokenAuth{}, fmt.Errorf("account token auth not configured")
	}
	return api.AccountTokenAuth{APIKey: s.accountToken}, nil
}

func (accountSecuritySource) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{}, nil
}

func newAccountAPIClient(apiEndpoint, accountToken string) (*api.Client, error) {
	baseURL := strings.TrimRight(apiEndpoint, "/") + "/v1"
	return api.NewClient(baseURL, accountSecuritySource{accountToken: accountToken}, api.WithClient(http.DefaultClient))
}
