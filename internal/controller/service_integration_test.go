package controller

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
	ctrlcfg "github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/openziti/automation"
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
	envLifecycle := &fakeEnvironmentLifecycle{
		enableResult: &automation.ProvisionedEnvironment{
			IdentityID:     "ziti-env-1",
			EnrollmentJSON: []byte(`{"id":"ziti-env-1"}`),
			PolicyID:       "erp-1",
		},
	}
	fakeTunnelLifecycle := &fakeTunnelLifecycle{}
	service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		return envLifecycle, fakeTunnelLifecycle, nil
	}
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

	createOtherOrgRes, err := adminClient.CreateOrganization(ctx, &api.CreateOrganizationRequest{Name: "beta"})
	if err != nil {
		t.Fatalf("create second organization request: %v", err)
	}
	otherOrg, ok := createOtherOrgRes.(*api.Organization)
	if !ok {
		t.Fatalf("unexpected second create organization response: %T", createOtherOrgRes)
	}

	if _, err := adminClient.CreateAccount(ctx, &api.CreateAccountRequest{
		Email:    "bob@example.com",
		Password: "other-password",
	}, api.CreateAccountParams{OrganizationId: otherOrg.ID}); err != nil {
		t.Fatalf("create second account request: %v", err)
	}

	listUsersRes, err := adminClient.ListUsers(ctx, api.ListUsersParams{})
	if err != nil {
		t.Fatalf("list users request: %v", err)
	}
	users, ok := listUsersRes.(*api.ListAccountsResponse)
	if !ok {
		t.Fatalf("unexpected list users response: %T", listUsersRes)
	}
	if len(*users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(*users))
	}

	var filteredParams api.ListUsersParams
	filteredParams.OrganizationId.SetTo(org.ID)
	filteredUsersRes, err := adminClient.ListUsers(ctx, filteredParams)
	if err != nil {
		t.Fatalf("list users by organization request: %v", err)
	}
	filteredUsers, ok := filteredUsersRes.(*api.ListAccountsResponse)
	if !ok {
		t.Fatalf("unexpected filtered list users response: %T", filteredUsersRes)
	}
	if len(*filteredUsers) != 1 {
		t.Fatalf("expected 1 filtered user, got %d", len(*filteredUsers))
	}
	if (*filteredUsers)[0].Email != "alice@example.com" {
		t.Fatalf("expected filtered user alice@example.com, got %s", (*filteredUsers)[0].Email)
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

	enableEnvRes, err := accountClient.EnableEnvironment(ctx, &api.EnableEnvironmentRequest{})
	if err != nil {
		t.Fatalf("enable environment request: %v", err)
	}
	enabledEnv, ok := enableEnvRes.(*api.EnableEnvironmentResponse)
	if !ok {
		t.Fatalf("unexpected enable environment response: %T", enableEnvRes)
	}
	env := enabledEnv.Environment

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
	_, ok = createTunnelRes.(*api.Tunnel)
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

	disableEnvRes, err := accountClient.DisableEnvironment(ctx, api.DisableEnvironmentParams{EnvironmentId: env.ID})
	if err != nil {
		t.Fatalf("disable environment request: %v", err)
	}
	if _, ok := disableEnvRes.(*api.DisableEnvironmentNoContent); !ok {
		t.Fatalf("unexpected disable environment response: %T", disableEnvRes)
	}

	postDisableEnvsRes, err := accountClient.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("list environments after disable request: %v", err)
	}
	postDisableEnvironments, ok := postDisableEnvsRes.(*api.ListEnvironmentsResponse)
	if !ok {
		t.Fatalf("unexpected post-disable list environments response: %T", postDisableEnvsRes)
	}
	if len(*postDisableEnvironments) != 0 {
		t.Fatalf("expected 0 environments after disable, got %d", len(*postDisableEnvironments))
	}

	postDisableTunnelsRes, err := accountClient.ListTunnels(ctx)
	if err != nil {
		t.Fatalf("list tunnels after disable request: %v", err)
	}
	postDisableTunnels, ok := postDisableTunnelsRes.(*api.ListTunnelsResponse)
	if !ok {
		t.Fatalf("unexpected post-disable list tunnels response: %T", postDisableTunnelsRes)
	}
	if len(*postDisableTunnels) != 0 {
		t.Fatalf("expected 0 tunnels after disable, got %d", len(*postDisableTunnels))
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

	deleteAccountRes, err := adminClient.DeleteAccount(ctx, api.DeleteAccountParams{
		OrganizationId: org.ID,
		AccountId:      (*accounts)[0].ID,
	})
	if err != nil {
		t.Fatalf("delete account request: %v", err)
	}
	if _, ok := deleteAccountRes.(*api.DeleteAccountNoContent); !ok {
		t.Fatalf("unexpected delete account response: %T", deleteAccountRes)
	}

	postDeleteAccountsRes, err := adminClient.ListAccounts(ctx, api.ListAccountsParams{OrganizationId: org.ID})
	if err != nil {
		t.Fatalf("list accounts after delete request: %v", err)
	}
	postDeleteAccounts, ok := postDeleteAccountsRes.(*api.ListAccountsResponse)
	if !ok {
		t.Fatalf("unexpected post-delete list accounts response: %T", postDeleteAccountsRes)
	}
	if len(*postDeleteAccounts) != 0 {
		t.Fatalf("expected 0 accounts after delete, got %d", len(*postDeleteAccounts))
	}

	deleteOrgRes, err := adminClient.DeleteOrganization(ctx, api.DeleteOrganizationParams{OrganizationId: org.ID})
	if err != nil {
		t.Fatalf("delete organization request: %v", err)
	}
	if _, ok := deleteOrgRes.(*api.DeleteOrganizationNoContent); !ok {
		t.Fatalf("unexpected delete organization response: %T", deleteOrgRes)
	}

	postDeleteOrgsRes, err := adminClient.ListOrganizations(ctx)
	if err != nil {
		t.Fatalf("list organizations after delete request: %v", err)
	}
	postDeleteOrgs, ok := postDeleteOrgsRes.(*api.ListOrganizationsResponse)
	if !ok {
		t.Fatalf("unexpected post-delete list organizations response: %T", postDeleteOrgsRes)
	}
	if len(*postDeleteOrgs) != 1 {
		t.Fatalf("expected 1 organization after delete, got %d", len(*postDeleteOrgs))
	}
	if (*postDeleteOrgs)[0].Name != "beta" {
		t.Fatalf("expected remaining organization beta, got %s", (*postDeleteOrgs)[0].Name)
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

type fakeEnvironmentLifecycle struct {
	enableResult *automation.ProvisionedEnvironment
	enableErr    error
	disableCalls []automation.DeprovisionEnvironmentSpec
}

func (f *fakeEnvironmentLifecycle) Enable(context.Context, automation.EnvironmentSpec) (*automation.ProvisionedEnvironment, error) {
	return f.enableResult, f.enableErr
}

func (f *fakeEnvironmentLifecycle) Disable(_ context.Context, spec automation.DeprovisionEnvironmentSpec) error {
	f.disableCalls = append(f.disableCalls, spec)
	return nil
}

type fakeTunnelLifecycle struct {
	deprovisionCalls []automation.DeprovisionTunnelSpec
}

func (f *fakeTunnelLifecycle) Deprovision(_ context.Context, spec automation.DeprovisionTunnelSpec) error {
	f.deprovisionCalls = append(f.deprovisionCalls, spec)
	return nil
}
