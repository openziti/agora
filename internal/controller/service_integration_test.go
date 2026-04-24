package controller

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
	ctrlcfg "github.com/openziti/agora/internal/controller/config"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
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
	if (*environments)[0].LastSeenAt.Set {
		t.Fatalf("expected no last seen before heartbeat, got %#v", (*environments)[0].LastSeenAt)
	}

	heartbeatRes, err := accountClient.HeartbeatEnvironment(ctx, api.HeartbeatEnvironmentParams{EnvironmentId: env.ID})
	if err != nil {
		t.Fatalf("heartbeat environment request: %v", err)
	}
	if _, ok := heartbeatRes.(*api.HeartbeatEnvironmentNoContent); !ok {
		t.Fatalf("unexpected heartbeat environment response: %T", heartbeatRes)
	}

	getEnvRes, err := accountClient.GetEnvironment(ctx, api.GetEnvironmentParams{EnvironmentId: env.ID})
	if err != nil {
		t.Fatalf("get environment request: %v", err)
	}
	gotEnv, ok := getEnvRes.(*api.Environment)
	if !ok {
		t.Fatalf("unexpected get environment response: %T", getEnvRes)
	}
	if !gotEnv.LastSeenAt.Set {
		t.Fatalf("expected last seen after heartbeat")
	}

	createTunnelRes, err := accountClient.CreateTunnel(ctx, &api.CreateTunnelRequest{
		EnvironmentId: env.ID,
		Name:          "llm-gateway",
		Mode:          api.TunnelModeTCP,
		BackendTarget: "127.0.0.1:8443",
	})
	if err != nil {
		t.Fatalf("create tunnel request: %v", err)
	}
	tunnel, ok := createTunnelRes.(*api.Tunnel)
	if !ok {
		t.Fatalf("unexpected create tunnel response: %T", createTunnelRes)
	}

	startServeRes, err := accountClient.StartTunnelServe(ctx, &api.StartTunnelServeRequest{
		EnvironmentId: env.ID,
	}, api.StartTunnelServeParams{TunnelId: tunnel.ID})
	if err != nil {
		t.Fatalf("start tunnel serve request: %v", err)
	}
	startedServe, ok := startServeRes.(*api.TunnelServe)
	if !ok {
		t.Fatalf("unexpected start tunnel serve response: %T", startServeRes)
	}

	getServeRes, err := accountClient.GetTunnelServe(ctx, api.GetTunnelServeParams{TunnelId: tunnel.ID})
	if err != nil {
		t.Fatalf("get tunnel serve request: %v", err)
	}
	gotServe, ok := getServeRes.(*api.TunnelServe)
	if !ok {
		t.Fatalf("unexpected get tunnel serve response: %T", getServeRes)
	}
	if gotServe.ID != startedServe.ID {
		t.Fatalf("expected serve id %s, got %s", startedServe.ID, gotServe.ID)
	}

	listTunnelsRes, err := accountClient.ListTunnels(ctx, api.ListTunnelsParams{})
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

	postDisableTunnelsRes, err := accountClient.ListTunnels(ctx, api.ListTunnelsParams{})
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
	provisionResult  *automation.ProvisionedTunnel
	provisionCalls   []automation.TunnelSpec
	attachmentResult string
	attachmentCalls  []automation.TunnelAccessSpec
	deprovisionCalls []automation.DeprovisionTunnelSpec
}

func (f *fakeTunnelLifecycle) Provision(_ context.Context, spec automation.TunnelSpec) (*automation.ProvisionedTunnel, error) {
	f.provisionCalls = append(f.provisionCalls, spec)
	if f.provisionResult != nil {
		return f.provisionResult, nil
	}
	return &automation.ProvisionedTunnel{
		ServiceID:                 "service-1",
		BindPolicyID:              "bind-1",
		ServiceEdgeRouterPolicyID: "serp-1",
	}, nil
}

func (f *fakeTunnelLifecycle) CreateAttachmentDialPolicy(_ context.Context, spec automation.TunnelAccessSpec) (string, error) {
	f.attachmentCalls = append(f.attachmentCalls, spec)
	if f.attachmentResult != "" {
		return f.attachmentResult, nil
	}
	return "dial-1", nil
}

func (f *fakeTunnelLifecycle) Deprovision(_ context.Context, spec automation.DeprovisionTunnelSpec) error {
	f.deprovisionCalls = append(f.deprovisionCalls, spec)
	return nil
}

func newWorkgroupTestEnv(t *testing.T) (*workgroupTestEnv, func()) {
	t.Helper()
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
	if _, err := persistence.MigrateUp(ctx, store); err != nil {
		_ = store.Close()
		t.Fatalf("migrate up: %v", err)
	}

	cfg := ctrlcfg.DefaultConfig()
	cfg.AdminTokens = []string{"admin-token"}

	service := NewService(cfg, store)
	envLifecycle := &fakeEnvironmentLifecycle{enableResult: &automation.ProvisionedEnvironment{IdentityID: "ziti-env-1", EnrollmentJSON: []byte(`{}`), PolicyID: "erp-1"}}
	tunnelLC := &fakeTunnelLifecycle{}
	service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		return envLifecycle, tunnelLC, nil
	}
	handler, err := NewHandler(service)
	if err != nil {
		_ = store.Close()
		t.Fatalf("new handler: %v", err)
	}
	ts := httptest.NewServer(handler)

	cleanup := func() {
		ts.Close()
		_ = store.Close()
	}

	return &workgroupTestEnv{
		ctx:     ctx,
		store:   store,
		service: service,
		ts:      ts,
		baseURL: ts.URL + "/v1",
	}, cleanup
}

type workgroupTestEnv struct {
	ctx     context.Context
	store   *persistence.Store
	service *Service
	ts      *httptest.Server
	baseURL string
}

func (e *workgroupTestEnv) adminClient(t *testing.T) *api.Client {
	t.Helper()
	c, err := api.NewClient(e.baseURL, staticSecuritySource{adminToken: "admin-token"}, api.WithClient(e.ts.Client()))
	if err != nil {
		t.Fatalf("new admin client: %v", err)
	}
	return c
}

func (e *workgroupTestEnv) accountClient(t *testing.T, token string) *api.Client {
	t.Helper()
	c, err := api.NewClient(e.baseURL, staticSecuritySource{accountToken: token}, api.WithClient(e.ts.Client()))
	if err != nil {
		t.Fatalf("new account client: %v", err)
	}
	return c
}

func (e *workgroupTestEnv) createOrgWithAccount(t *testing.T, orgName, email string) (orgID, accountID, accountToken string) {
	t.Helper()
	admin := e.adminClient(t)
	orgRes, err := admin.CreateOrganization(e.ctx, &api.CreateOrganizationRequest{Name: orgName})
	if err != nil {
		t.Fatalf("create org %q: %v", orgName, err)
	}
	org, ok := orgRes.(*api.Organization)
	if !ok {
		t.Fatalf("create org %q unexpected response: %T", orgName, orgRes)
	}
	id, token := e.addAccountToOrg(t, org.ID, email)
	return org.ID, id, token
}

func (e *workgroupTestEnv) addAccountToOrg(t *testing.T, orgID, email string) (accountID, accountToken string) {
	t.Helper()
	admin := e.adminClient(t)
	acctRes, err := admin.CreateAccount(e.ctx, &api.CreateAccountRequest{Email: email, Password: "test-password-1"}, api.CreateAccountParams{OrganizationId: orgID})
	if err != nil {
		t.Fatalf("create account %q: %v", email, err)
	}
	tokenResp, ok := acctRes.(*api.AccountTokenResponse)
	if !ok {
		t.Fatalf("create account %q unexpected response: %T", email, acctRes)
	}
	accounts, err := e.store.Accounts.ListByOrganization(e.ctx, e.store.DB(), orgID)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	for _, a := range accounts {
		if a.Email == email {
			return a.ID, tokenResp.AccountToken
		}
	}
	t.Fatalf("account %q not found after creation", email)
	return "", ""
}

func TestWorkgroupIntraOrgCreateAndMembershipFlow(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	bobID, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	_ = bobID

	admin := env.adminClient(t)
	createRes, err := admin.CreateWorkgroup(env.ctx, &api.CreateWorkgroupRequest{
		Name:                  "research",
		Scope:                 api.CreateWorkgroupRequestScopeIntraOrg,
		OwnerOrganizationId:   orgA,
		InitialAdminAccountId: aliceID,
	})
	if err != nil {
		t.Fatalf("create workgroup: %v", err)
	}
	created, ok := createRes.(*api.CreateWorkgroupResponse)
	if !ok {
		t.Fatalf("create workgroup unexpected response: %T", createRes)
	}
	if created.Workgroup.State != api.WorkgroupStateActive {
		t.Fatalf("expected intra-org workgroup to be active, got %s", created.Workgroup.State)
	}
	if len(created.Invitations) != 0 {
		t.Fatalf("expected no invitations for intra-org, got %d", len(created.Invitations))
	}

	wgID := created.Workgroup.ID
	aliceClient := env.accountClient(t, aliceToken)
	bobClient := env.accountClient(t, bobToken)

	getRes, err := aliceClient.GetWorkgroup(env.ctx, api.GetWorkgroupParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("alice get workgroup: %v", err)
	}
	if _, ok := getRes.(*api.Workgroup); !ok {
		t.Fatalf("alice get workgroup unexpected response: %T", getRes)
	}

	bobGet, err := bobClient.GetWorkgroup(env.ctx, api.GetWorkgroupParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("bob get workgroup: %v", err)
	}
	if _, ok := bobGet.(*api.GetWorkgroupNotFound); !ok {
		t.Fatalf("expected bob to receive 404, got %T", bobGet)
	}

	addRes, err := aliceClient.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bobMembership, ok := addRes.(*api.WorkgroupMembership)
	if !ok {
		t.Fatalf("add bob unexpected response: %T", addRes)
	}

	dupRes, err := aliceClient.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("add bob duplicate: %v", err)
	}
	if _, ok := dupRes.(*api.AddWorkgroupMemberConflict); !ok {
		t.Fatalf("expected conflict on duplicate add, got %T", dupRes)
	}

	roleRes, err := aliceClient.ChangeWorkgroupMembershipRole(env.ctx, &api.ChangeWorkgroupMembershipRoleRequest{Role: api.WorkgroupMembershipRoleAdmin}, api.ChangeWorkgroupMembershipRoleParams{WorkgroupId: wgID, MembershipId: bobMembership.ID})
	if err != nil {
		t.Fatalf("promote bob: %v", err)
	}
	if _, ok := roleRes.(*api.WorkgroupMembership); !ok {
		t.Fatalf("promote bob unexpected response: %T", roleRes)
	}

	aliceMemberships, err := env.store.WorkgroupMemberships.ListByWorkgroup(env.ctx, env.store.DB(), wgID)
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	var aliceMID string
	for _, m := range aliceMemberships {
		if m.AccountID == aliceID {
			aliceMID = m.ID
		}
	}
	if aliceMID == "" {
		t.Fatalf("alice membership missing")
	}

	demoteAlice, err := aliceClient.ChangeWorkgroupMembershipRole(env.ctx, &api.ChangeWorkgroupMembershipRoleRequest{Role: api.WorkgroupMembershipRoleMember}, api.ChangeWorkgroupMembershipRoleParams{WorkgroupId: wgID, MembershipId: aliceMID})
	if err != nil {
		t.Fatalf("demote alice: %v", err)
	}
	if _, ok := demoteAlice.(*api.WorkgroupMembership); !ok {
		t.Fatalf("demote alice unexpected response: %T", demoteAlice)
	}

	demoteBob, err := bobClient.ChangeWorkgroupMembershipRole(env.ctx, &api.ChangeWorkgroupMembershipRoleRequest{Role: api.WorkgroupMembershipRoleMember}, api.ChangeWorkgroupMembershipRoleParams{WorkgroupId: wgID, MembershipId: bobMembership.ID})
	if err != nil {
		t.Fatalf("demote bob attempt: %v", err)
	}
	if _, ok := demoteBob.(*api.ChangeWorkgroupMembershipRoleConflict); !ok {
		t.Fatalf("expected last_admin conflict, got %T", demoteBob)
	}

	delRes, err := bobClient.DeleteWorkgroup(env.ctx, api.DeleteWorkgroupParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("bob delete workgroup: %v", err)
	}
	if _, ok := delRes.(*api.DeleteWorkgroupNoContent); !ok {
		t.Fatalf("delete unexpected response: %T", delRes)
	}

	getAfterDel, err := bobClient.GetWorkgroup(env.ctx, api.GetWorkgroupParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if _, ok := getAfterDel.(*api.GetWorkgroupNotFound); !ok {
		t.Fatalf("expected 404 after delete, got %T", getAfterDel)
	}
}

func TestWorkgroupAddMemberCrossOrgRejected(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, _, _ = env.createOrgWithAccount(t, "org-b", "bob@example.com")

	admin := env.adminClient(t)
	createRes, err := admin.CreateWorkgroup(env.ctx, &api.CreateWorkgroupRequest{
		Name:                  "internal",
		Scope:                 api.CreateWorkgroupRequestScopeIntraOrg,
		OwnerOrganizationId:   orgA,
		InitialAdminAccountId: aliceID,
	})
	if err != nil {
		t.Fatalf("create workgroup: %v", err)
	}
	wgID := createRes.(*api.CreateWorkgroupResponse).Workgroup.ID

	aliceClient := env.accountClient(t, aliceToken)
	res, err := aliceClient.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("add cross-org member: %v", err)
	}
	if _, ok := res.(*api.AddWorkgroupMemberForbidden); !ok {
		t.Fatalf("expected 403 cross_org_membership, got %T", res)
	}
}

func TestWorkgroupInterOrgInvitationFlow(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	orgB, bobID, bobToken := env.createOrgWithAccount(t, "org-b", "bob@example.com")
	orgC, carolID, _ := env.createOrgWithAccount(t, "org-c", "carol@example.com")

	admin := env.adminClient(t)
	createRes, err := admin.CreateWorkgroup(env.ctx, &api.CreateWorkgroupRequest{
		Name:                         "shared",
		Scope:                        api.CreateWorkgroupRequestScopeInterOrg,
		OwnerOrganizationId:          orgA,
		InitialAdminAccountId:        aliceID,
		ParticipatingOrganizationIds: []string{orgB, orgC},
	})
	if err != nil {
		t.Fatalf("create inter-org workgroup: %v", err)
	}
	created, ok := createRes.(*api.CreateWorkgroupResponse)
	if !ok {
		t.Fatalf("create inter-org workgroup unexpected response: %T", createRes)
	}
	if created.Workgroup.State != api.WorkgroupStatePending {
		t.Fatalf("expected pending state, got %s", created.Workgroup.State)
	}
	if len(created.Invitations) != 2 {
		t.Fatalf("expected 2 invitations, got %d", len(created.Invitations))
	}

	wgID := created.Workgroup.ID

	bobClient := env.accountClient(t, bobToken)
	bobGet, err := bobClient.GetWorkgroup(env.ctx, api.GetWorkgroupParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("bob get pending workgroup: %v", err)
	}
	if _, ok := bobGet.(*api.GetWorkgroupNotFound); !ok {
		t.Fatalf("expected bob to receive 404 for pending workgroup, got %T", bobGet)
	}

	acceptRes, err := admin.AcceptWorkgroupInvitation(env.ctx, &api.AcceptWorkgroupInvitationRequest{InitialAdminAccountId: bobID}, api.AcceptWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: orgB})
	if err != nil {
		t.Fatalf("accept org-b invitation: %v", err)
	}
	ack, ok := acceptRes.(*api.AcknowledgeWorkgroupInvitationResponse)
	if !ok {
		t.Fatalf("accept unexpected response: %T", acceptRes)
	}
	if ack.Invitation.State != api.WorkgroupInvitationStateAccepted {
		t.Fatalf("expected accepted invitation, got %s", ack.Invitation.State)
	}
	if ack.Workgroup.State != api.WorkgroupStatePending {
		t.Fatalf("expected workgroup still pending after first accept, got %s", ack.Workgroup.State)
	}

	acceptC, err := admin.AcceptWorkgroupInvitation(env.ctx, &api.AcceptWorkgroupInvitationRequest{InitialAdminAccountId: carolID}, api.AcceptWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: orgC})
	if err != nil {
		t.Fatalf("accept org-c invitation: %v", err)
	}
	ackC, ok := acceptC.(*api.AcknowledgeWorkgroupInvitationResponse)
	if !ok {
		t.Fatalf("accept org-c unexpected response: %T", acceptC)
	}
	if ackC.Workgroup.State != api.WorkgroupStateActive {
		t.Fatalf("expected workgroup active after both accepts, got %s", ackC.Workgroup.State)
	}

	bobAfterAccept, err := bobClient.GetWorkgroup(env.ctx, api.GetWorkgroupParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("bob get after accept: %v", err)
	}
	if _, ok := bobAfterAccept.(*api.Workgroup); !ok {
		t.Fatalf("expected bob to see workgroup after accept, got %T", bobAfterAccept)
	}

	dupAccept, err := admin.AcceptWorkgroupInvitation(env.ctx, &api.AcceptWorkgroupInvitationRequest{InitialAdminAccountId: bobID}, api.AcceptWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: orgB})
	if err != nil {
		t.Fatalf("dup accept: %v", err)
	}
	if _, ok := dupAccept.(*api.AcceptWorkgroupInvitationConflict); !ok {
		t.Fatalf("expected conflict on duplicate accept, got %T", dupAccept)
	}

	_ = aliceToken
}

func TestWorkgroupInterOrgDeclineLocksWorkgroup(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, _ := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	orgB, _, _ := env.createOrgWithAccount(t, "org-b", "bob@example.com")
	orgC, carolID, _ := env.createOrgWithAccount(t, "org-c", "carol@example.com")

	admin := env.adminClient(t)
	createRes, err := admin.CreateWorkgroup(env.ctx, &api.CreateWorkgroupRequest{
		Name:                         "shared",
		Scope:                        api.CreateWorkgroupRequestScopeInterOrg,
		OwnerOrganizationId:          orgA,
		InitialAdminAccountId:        aliceID,
		ParticipatingOrganizationIds: []string{orgB, orgC},
	})
	if err != nil {
		t.Fatalf("create inter-org workgroup: %v", err)
	}
	wgID := createRes.(*api.CreateWorkgroupResponse).Workgroup.ID

	declineRes, err := admin.DeclineWorkgroupInvitation(env.ctx, &api.DeclineWorkgroupInvitationRequest{}, api.DeclineWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: orgB})
	if err != nil {
		t.Fatalf("decline org-b invitation: %v", err)
	}
	ack, ok := declineRes.(*api.AcknowledgeWorkgroupInvitationResponse)
	if !ok {
		t.Fatalf("decline unexpected response: %T", declineRes)
	}
	if ack.Workgroup.State != api.WorkgroupStateDeclined {
		t.Fatalf("expected declined workgroup, got %s", ack.Workgroup.State)
	}

	subsequent, err := admin.AcceptWorkgroupInvitation(env.ctx, &api.AcceptWorkgroupInvitationRequest{InitialAdminAccountId: carolID}, api.AcceptWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: orgC})
	if err != nil {
		t.Fatalf("subsequent accept after decline: %v", err)
	}
	if _, ok := subsequent.(*api.AcceptWorkgroupInvitationConflict); !ok {
		t.Fatalf("expected workgroup_declined conflict, got %T", subsequent)
	}
}

func (e *workgroupTestEnv) seedIntraOrgWorkgroup(t *testing.T, orgID, name, adminAcctID string) string {
	t.Helper()
	admin := e.adminClient(t)
	res, err := admin.CreateWorkgroup(e.ctx, &api.CreateWorkgroupRequest{
		Name:                  name,
		Scope:                 api.CreateWorkgroupRequestScopeIntraOrg,
		OwnerOrganizationId:   orgID,
		InitialAdminAccountId: adminAcctID,
	})
	if err != nil {
		t.Fatalf("create workgroup %q: %v", name, err)
	}
	return res.(*api.CreateWorkgroupResponse).Workgroup.ID
}

func TestAdvertisementPublishVisibility(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	bobID, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "channel", aliceID)

	aliceClient := env.accountClient(t, aliceToken)
	pubRes, err := aliceClient.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "alice-summarizer",
		Capabilities:        []api.AdvertisementCapability{{Name: "summarize"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	ad, ok := pubRes.(*api.Advertisement)
	if !ok {
		t.Fatalf("publish unexpected response: %T", pubRes)
	}

	bobClient := env.accountClient(t, bobToken)
	bobGet, err := bobClient.GetAdvertisement(env.ctx, api.GetAdvertisementParams{AdvertisementId: ad.ID})
	if err != nil {
		t.Fatalf("bob get: %v", err)
	}
	if _, ok := bobGet.(*api.GetAdvertisementNotFound); !ok {
		t.Fatalf("expected bob to get 404 (no shared workgroup), got %T", bobGet)
	}

	addMember, err := aliceClient.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("add bob to workgroup: %v", err)
	}
	if _, ok := addMember.(*api.WorkgroupMembership); !ok {
		t.Fatalf("add member unexpected response: %T", addMember)
	}

	bobGet2, err := bobClient.GetAdvertisement(env.ctx, api.GetAdvertisementParams{AdvertisementId: ad.ID})
	if err != nil {
		t.Fatalf("bob get after add: %v", err)
	}
	if _, ok := bobGet2.(*api.Advertisement); !ok {
		t.Fatalf("expected bob to see ad after joining workgroup, got %T", bobGet2)
	}

	_ = bobID
}

func TestAdvertisementPublishRejectedNotMember(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, _ := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "alice-only", aliceID)

	bobClient := env.accountClient(t, bobToken)
	res, err := bobClient.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "bob-tries",
		Capabilities:        []api.AdvertisementCapability{{Name: "x"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, ok := res.(*api.PublishAdvertisementForbidden); !ok {
		t.Fatalf("expected 403 not_a_workgroup_member, got %T", res)
	}
}

func TestAdvertisementPublishUnknownWorkgroup(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	aliceClient := env.accountClient(t, aliceToken)

	res, err := aliceClient.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "ghost",
		Capabilities:        []api.AdvertisementCapability{{Name: "x"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{"wg_zzzzzzzzzzzz"},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, ok := res.(*api.PublishAdvertisementBadRequest); !ok {
		t.Fatalf("expected 400 unknown_workgroup, got %T", res)
	}
}

func TestAdvertisementNameUniqueness(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "ch", aliceID)

	aliceClient := env.accountClient(t, aliceToken)
	pubReq := &api.PublishAdvertisementRequest{
		Name:                "DupName",
		Capabilities:        []api.AdvertisementCapability{{Name: "x"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
	}
	if _, err := aliceClient.PublishAdvertisement(env.ctx, pubReq); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	dupRes, err := aliceClient.PublishAdvertisement(env.ctx, pubReq)
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if _, ok := dupRes.(*api.PublishAdvertisementConflict); !ok {
		t.Fatalf("expected 409 name_in_use, got %T", dupRes)
	}
}

func TestAdvertisementUpdateAndRetract(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "ch", aliceID)

	aliceClient := env.accountClient(t, aliceToken)
	bobClient := env.accountClient(t, bobToken)

	pubRes, err := aliceClient.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "to-update",
		Capabilities:        []api.AdvertisementCapability{{Name: "v1"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	advID := pubRes.(*api.Advertisement).ID

	updReq := &api.UpdateAdvertisementRequest{Capabilities: []api.AdvertisementCapability{{Name: "v2"}}}
	updRes, err := bobClient.UpdateAdvertisement(env.ctx, updReq, api.UpdateAdvertisementParams{AdvertisementId: advID})
	if err != nil {
		t.Fatalf("bob update: %v", err)
	}
	if _, ok := updRes.(*api.UpdateAdvertisementNotFound); !ok {
		t.Fatalf("expected bob to receive 404 on update (non-owner), got %T", updRes)
	}

	updRes2, err := aliceClient.UpdateAdvertisement(env.ctx, updReq, api.UpdateAdvertisementParams{AdvertisementId: advID})
	if err != nil {
		t.Fatalf("alice update: %v", err)
	}
	if _, ok := updRes2.(*api.Advertisement); !ok {
		t.Fatalf("expected updated advertisement, got %T", updRes2)
	}

	retract, err := aliceClient.RetractAdvertisement(env.ctx, api.RetractAdvertisementParams{AdvertisementId: advID})
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	if _, ok := retract.(*api.RetractAdvertisementNoContent); !ok {
		t.Fatalf("expected 204, got %T", retract)
	}

	// idempotent re-retract returns 204
	retract2, err := aliceClient.RetractAdvertisement(env.ctx, api.RetractAdvertisementParams{AdvertisementId: advID})
	if err != nil {
		t.Fatalf("re-retract: %v", err)
	}
	if _, ok := retract2.(*api.RetractAdvertisementNoContent); !ok {
		t.Fatalf("expected idempotent 204, got %T", retract2)
	}
}

func TestAdvertisementUpdateRetractedReturns409(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "ch", aliceID)
	aliceClient := env.accountClient(t, aliceToken)

	pub, _ := aliceClient.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "ad",
		Capabilities:        []api.AdvertisementCapability{{Name: "x"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
	})
	advID := pub.(*api.Advertisement).ID
	if _, err := aliceClient.RetractAdvertisement(env.ctx, api.RetractAdvertisementParams{AdvertisementId: advID}); err != nil {
		t.Fatalf("retract: %v", err)
	}

	updReq := &api.UpdateAdvertisementRequest{Capabilities: []api.AdvertisementCapability{{Name: "y"}}}
	updRes, err := aliceClient.UpdateAdvertisement(env.ctx, updReq, api.UpdateAdvertisementParams{AdvertisementId: advID})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := updRes.(*api.UpdateAdvertisementConflict); !ok {
		t.Fatalf("expected 409 advertisement_retracted, got %T", updRes)
	}
}

func TestCatalogSearchVisibilityAndFilters(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "shared", aliceID)
	otherWG := env.seedIntraOrgWorkgroup(t, orgA, "alice-only", aliceID)

	aliceClient := env.accountClient(t, aliceToken)
	bobClient := env.accountClient(t, bobToken)

	// add bob to "shared" only
	if _, err := aliceClient.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}

	publish := func(name, capName string, scopes []string) {
		t.Helper()
		if _, err := aliceClient.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
			Name:                name,
			Capabilities:        []api.AdvertisementCapability{{Name: capName}},
			InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
			WorkgroupScopes:     scopes,
		}); err != nil {
			t.Fatalf("publish %s: %v", name, err)
		}
	}
	publish("shared-summarizer", "summarize", []string{wgID})
	publish("shared-translator", "translate", []string{wgID})
	publish("alice-only-tool", "internal", []string{otherWG})

	// bob: shared has 2 ads visible; alice-only is invisible
	bobSearch, err := bobClient.SearchCatalog(env.ctx, api.SearchCatalogParams{})
	if err != nil {
		t.Fatalf("bob search: %v", err)
	}
	bobResp, ok := bobSearch.(*api.CatalogSearchResponse)
	if !ok {
		t.Fatalf("bob search unexpected response: %T", bobSearch)
	}
	if len(bobResp.Items) != 2 {
		t.Fatalf("expected bob to see 2 ads, got %d", len(bobResp.Items))
	}

	// capability filter: substring "summar" → 1 result
	capSearch, _ := bobClient.SearchCatalog(env.ctx, api.SearchCatalogParams{Capability: api.NewOptString("summar")})
	capResp := capSearch.(*api.CatalogSearchResponse)
	if len(capResp.Items) != 1 || capResp.Items[0].Name != "shared-summarizer" {
		t.Fatalf("expected single summarizer match, got %d items", len(capResp.Items))
	}
}

func TestCatalogSearchPagination(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "ch", aliceID)
	aliceClient := env.accountClient(t, aliceToken)

	for i := 0; i < 3; i++ {
		if _, err := aliceClient.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
			Name:                fmt.Sprintf("paged-%d", i),
			Capabilities:        []api.AdvertisementCapability{{Name: "x"}},
			InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
			WorkgroupScopes:     []string{wgID},
		}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
		time.Sleep(time.Millisecond * 5)
	}

	page1Res, err := aliceClient.SearchCatalog(env.ctx, api.SearchCatalogParams{Limit: api.NewOptInt(2)})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	page1 := page1Res.(*api.CatalogSearchResponse)
	if len(page1.Items) != 2 || !page1.NextCursor.Set {
		t.Fatalf("expected 2 items + cursor, got items=%d cursor.Set=%v", len(page1.Items), page1.NextCursor.Set)
	}

	page2Res, err := aliceClient.SearchCatalog(env.ctx, api.SearchCatalogParams{Limit: api.NewOptInt(2), Cursor: api.NewOptString(page1.NextCursor.Value)})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	page2 := page2Res.(*api.CatalogSearchResponse)
	if len(page2.Items) != 1 || page2.NextCursor.Set {
		t.Fatalf("expected 1 item + no cursor, got items=%d cursor.Set=%v", len(page2.Items), page2.NextCursor.Set)
	}
}
