package controller

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
	ctrlcfg "github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/persistence"
	"github.com/openziti/agora/internal/persistence/testutil"
)

func TestServiceHTTPFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := persistence.Open(ctx, persistence.Config{
		DSN:             testutil.StartPostgres(t),
		MaxOpenConns:    4,
		MaxIdleConns:    4,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := persistence.MigrateUp(ctx, store); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	cfg := ctrlcfg.DefaultConfig()
	cfg.AdminTokens = []string{"admin-token"}

	service := NewService(cfg, store)
	handler, err := NewHandler(service)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	baseURL := ts.URL + "/v1"

	adminClient, err := api.NewClient(baseURL, staticSecuritySource{adminToken: "admin-token"}, api.WithClient(ts.Client()))
	if err != nil {
		t.Fatalf("new admin client: %v", err)
	}

	createOrgRes, err := adminClient.CreateOrganization(ctx, &api.CreateOrganizationRequest{Name: "acme"})
	if err != nil {
		t.Fatalf("create organization request: %v", err)
	}
	org, ok := createOrgRes.(*api.Organization)
	if !ok {
		t.Fatalf("unexpected create organization response: %T", createOrgRes)
	}

	createAccountRes, err := adminClient.CreateAccount(ctx, &api.CreateAccountRequest{
		Email:    "alice@example.com",
		Password: "test-password",
	}, api.CreateAccountParams{OrganizationId: org.ID})
	if err != nil {
		t.Fatalf("create account request: %v", err)
	}
	accountTokenResp, ok := createAccountRes.(*api.AccountTokenResponse)
	if !ok {
		t.Fatalf("unexpected create account response: %T", createAccountRes)
	}

	listAccountsRes, err := adminClient.ListAccounts(ctx, api.ListAccountsParams{OrganizationId: org.ID})
	if err != nil {
		t.Fatalf("list accounts request: %v", err)
	}
	accounts, ok := listAccountsRes.(*api.ListAccountsResponse)
	if !ok {
		t.Fatalf("unexpected list accounts response: %T", listAccountsRes)
	}
	if len(*accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(*accounts))
	}

	loginClient, err := api.NewClient(baseURL, staticSecuritySource{}, api.WithClient(ts.Client()))
	if err != nil {
		t.Fatalf("new login client: %v", err)
	}

	loginRes, err := loginClient.Login(ctx, &api.LoginRequest{
		Email:    "alice@example.com",
		Password: "test-password",
	})
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	loginTokenResp, ok := loginRes.(*api.AccountTokenResponse)
	if !ok {
		t.Fatalf("unexpected login response: %T", loginRes)
	}
	if loginTokenResp.AccountToken != accountTokenResp.AccountToken {
		t.Fatalf("expected login token to match created account token")
	}

	accountClient, err := api.NewClient(baseURL, staticSecuritySource{accountToken: loginTokenResp.AccountToken}, api.WithClient(ts.Client()))
	if err != nil {
		t.Fatalf("new account client: %v", err)
	}

	createEnvRes, err := accountClient.CreateEnvironment(ctx, &api.CreateEnvironmentRequest{
		ZitiIdentityId: "ziti-env-1",
	})
	if err != nil {
		t.Fatalf("create environment request: %v", err)
	}
	env, ok := createEnvRes.(*api.Environment)
	if !ok {
		t.Fatalf("unexpected create environment response: %T", createEnvRes)
	}

	listEnvsRes, err := accountClient.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("list environments request: %v", err)
	}
	environments, ok := listEnvsRes.(*api.ListEnvironmentsResponse)
	if !ok {
		t.Fatalf("unexpected list environments response: %T", listEnvsRes)
	}
	if len(*environments) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(*environments))
	}

	createTunnelRes, err := accountClient.CreateTunnel(ctx, &api.CreateTunnelRequest{
		EnvironmentId:  env.ID,
		Name:           "llm-gateway",
		BackendAddress: "127.0.0.1:8443",
	})
	if err != nil {
		t.Fatalf("create tunnel request: %v", err)
	}
	tunnel, ok := createTunnelRes.(*api.Tunnel)
	if !ok {
		t.Fatalf("unexpected create tunnel response: %T", createTunnelRes)
	}

	listTunnelsRes, err := accountClient.ListTunnels(ctx)
	if err != nil {
		t.Fatalf("list tunnels request: %v", err)
	}
	tunnels, ok := listTunnelsRes.(*api.ListTunnelsResponse)
	if !ok {
		t.Fatalf("unexpected list tunnels response: %T", listTunnelsRes)
	}
	if len(*tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(*tunnels))
	}

	changePasswordRes, err := accountClient.ChangePassword(ctx, &api.ChangePasswordRequest{
		Email:       "alice@example.com",
		OldPassword: "test-password",
		NewPassword: "updated-password",
	})
	if err != nil {
		t.Fatalf("change password request: %v", err)
	}
	if _, ok := changePasswordRes.(*api.ChangePasswordOK); !ok {
		t.Fatalf("unexpected change password response: %T", changePasswordRes)
	}

	newLoginRes, err := loginClient.Login(ctx, &api.LoginRequest{
		Email:    "alice@example.com",
		Password: "updated-password",
	})
	if err != nil {
		t.Fatalf("login after password change request: %v", err)
	}
	newLoginTokenResp, ok := newLoginRes.(*api.AccountTokenResponse)
	if !ok {
		t.Fatalf("unexpected login-after-change response: %T", newLoginRes)
	}

	rotatingClient, err := api.NewClient(baseURL, staticSecuritySource{accountToken: newLoginTokenResp.AccountToken}, api.WithClient(ts.Client()))
	if err != nil {
		t.Fatalf("new rotating client: %v", err)
	}

	rotateRes, err := rotatingClient.RegenerateAccountToken(ctx, &api.RegenerateTokenRequest{
		Email: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("regenerate token request: %v", err)
	}
	rotatedTokenResp, ok := rotateRes.(*api.AccountTokenResponse)
	if !ok {
		t.Fatalf("unexpected regenerate token response: %T", rotateRes)
	}
	if rotatedTokenResp.AccountToken == newLoginTokenResp.AccountToken {
		t.Fatalf("expected token rotation to change the account token")
	}

	rotatedClient, err := api.NewClient(baseURL, staticSecuritySource{accountToken: rotatedTokenResp.AccountToken}, api.WithClient(ts.Client()))
	if err != nil {
		t.Fatalf("new rotated client: %v", err)
	}

	if _, err := rotatedClient.GetTunnel(ctx, api.GetTunnelParams{TunnelId: tunnel.ID}); err != nil {
		t.Fatalf("get tunnel with rotated token: %v", err)
	}
}

type staticSecuritySource struct {
	accountToken string
	adminToken   string
}

func (s staticSecuritySource) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	return api.AccountTokenAuth{APIKey: s.accountToken}, nil
}

func (s staticSecuritySource) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{APIKey: s.adminToken}, nil
}
