package controller

import (
	"context"
	"fmt"
	"net/http"
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
		BackendTarget: api.NewOptString("127.0.0.1:8443"),
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
	for _, item := range bobResp.Items {
		if item.OrganizationName != "org-a" {
			t.Fatalf("expected catalog organizationName %q, got %q", "org-a", item.OrganizationName)
		}
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

// --- session tests ---

func seedSharedWorkgroupAd(t *testing.T, env *workgroupTestEnv, orgA string, aliceID string, wgName, adName string) (wgID string, advertisementID string) {
	t.Helper()
	wgID = env.seedIntraOrgWorkgroup(t, orgA, wgName, aliceID)
	aliceClient := env.accountClient(t, env.mustTokenForAccount(t, aliceID))
	pubRes, err := aliceClient.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                adName,
		Capabilities:        []api.AdvertisementCapability{{Name: "quote"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
		TunnelMode:          api.NewOptAdvertisementTunnelMode(api.AdvertisementTunnelModeTCP),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	ad, ok := pubRes.(*api.Advertisement)
	if !ok {
		t.Fatalf("publish unexpected response: %T", pubRes)
	}
	return wgID, ad.ID
}

func (e *workgroupTestEnv) mustTokenForAccount(t *testing.T, accountID string) string {
	t.Helper()
	acct, err := e.store.Accounts.GetByID(e.ctx, e.store.DB(), accountID)
	if err != nil {
		t.Fatalf("get account by id: %v", err)
	}
	return acct.AccountToken
}

func (e *workgroupTestEnv) enableEnvironment(t *testing.T, token string) string {
	t.Helper()
	c := e.accountClient(t, token)
	res, err := c.EnableEnvironment(e.ctx, &api.EnableEnvironmentRequest{})
	if err != nil {
		t.Fatalf("enable env: %v", err)
	}
	resp, ok := res.(*api.EnableEnvironmentResponse)
	if !ok {
		t.Fatalf("enable env unexpected: %T", res)
	}
	return resp.Environment.ID
}

func TestDashboardSummaryEndpoint(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgAlpha, alphaID, alphaToken := env.createOrgWithAccount(t, "Alpha Co", "alpha@example.com")
	orgBeta, betaID, betaToken := env.createOrgWithAccount(t, "Beta Co", "beta@example.com")

	wgAlpha := env.seedIntraOrgWorkgroup(t, orgAlpha, "alpha-shared", alphaID)
	wgBeta := env.seedIntraOrgWorkgroup(t, orgBeta, "beta-shared", betaID)
	alphaAdID := env.publishDashboardAdvertisement(t, alphaToken, "alpha-ad", wgAlpha)
	env.publishDashboardAdvertisement(t, betaToken, "beta-ad-one", wgBeta)
	env.publishDashboardAdvertisement(t, betaToken, "beta-ad-two", wgBeta)

	now := time.Now().UTC()
	seedDashboardEnvironment(t, env, orgAlpha, alphaID, "alpha-host", persistence.EnvironmentStateEnabled, &now)
	seedDashboardEnvironment(t, env, orgBeta, betaID, "beta-host-one", persistence.EnvironmentStateEnabled, &now)
	seedDashboardEnvironment(t, env, orgBeta, betaID, "beta-host-two", persistence.EnvironmentStateEnabled, &now)
	seedDashboardSession(t, env, orgAlpha, alphaID, wgAlpha, alphaAdID, persistence.SessionStateActive, now.Add(-time.Hour))
	recordDashboardEnvelopeEvent(t, env, orgAlpha, alphaID, wgAlpha, 7, now.Add(-time.Hour))
	recordDashboardSessionProposedEvent(t, env, orgAlpha, alphaID, wgAlpha, now.Add(-time.Hour))

	assertDashboardUnauthorized(t, env, "/dashboard/summary")

	alpha := env.accountClient(t, alphaToken)
	alphaRes, err := alpha.GetDashboardSummary(env.ctx)
	if err != nil {
		t.Fatalf("get alpha summary: %v", err)
	}
	alphaSummary, ok := alphaRes.(*api.DashboardSummaryResponse)
	if !ok {
		t.Fatalf("alpha summary unexpected response: %T", alphaRes)
	}
	if alphaSummary.Account.OrganizationName != "Alpha Co" {
		t.Fatalf("expected Alpha Co organization name, got %q", alphaSummary.Account.OrganizationName)
	}
	if alphaSummary.Account.AccountId != alphaID || alphaSummary.Account.OrganizationId != orgAlpha {
		t.Fatalf("summary account mismatch: %#v", alphaSummary.Account)
	}
	if alphaSummary.Stats.ActiveSessions != 1 || alphaSummary.Stats.ActiveSessionsDelta7d != 1 {
		t.Fatalf("expected alpha active sessions 1 delta 1, got %+v", alphaSummary.Stats)
	}
	if alphaSummary.Stats.EnvelopesToday != 7 || alphaSummary.Stats.EnvelopesYesterday != 0 {
		t.Fatalf("expected alpha envelopes 7/0, got %+v", alphaSummary.Stats)
	}
	if alphaSummary.Ribbon.AdvertisementCount != 1 || alphaSummary.Ribbon.EnvironmentCount != 1 || alphaSummary.Ribbon.SessionsToday != 1 {
		t.Fatalf("expected alpha scoped ribbon counts, got %+v", alphaSummary.Ribbon)
	}

	beta := env.accountClient(t, betaToken)
	betaRes, err := beta.GetDashboardSummary(env.ctx)
	if err != nil {
		t.Fatalf("get beta summary: %v", err)
	}
	betaSummary, ok := betaRes.(*api.DashboardSummaryResponse)
	if !ok {
		t.Fatalf("beta summary unexpected response: %T", betaRes)
	}
	if betaSummary.Account.OrganizationName != "Beta Co" {
		t.Fatalf("expected Beta Co organization name, got %q", betaSummary.Account.OrganizationName)
	}
	if betaSummary.Stats.ActiveSessions != 0 || betaSummary.Stats.EnvelopesToday != 0 {
		t.Fatalf("expected beta to exclude alpha stats, got %+v", betaSummary.Stats)
	}
	if betaSummary.Ribbon.AdvertisementCount != 2 || betaSummary.Ribbon.EnvironmentCount != 2 || betaSummary.Ribbon.SessionsToday != 0 {
		t.Fatalf("expected beta scoped ribbon counts, got %+v", betaSummary.Ribbon)
	}
}

func TestDashboardActivityEndpoint(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgAlpha, alphaID, alphaToken := env.createOrgWithAccount(t, "Alpha Co", "alpha-activity@example.com")
	orgBeta, betaID, _ := env.createOrgWithAccount(t, "Beta Co", "beta-activity@example.com")
	wgAlpha := env.seedIntraOrgWorkgroup(t, orgAlpha, "alpha-activity", alphaID)
	wgBeta := env.seedIntraOrgWorkgroup(t, orgBeta, "beta-activity", betaID)

	now := time.Now().UTC()
	recordDashboardEnvelopeEvent(t, env, orgAlpha, alphaID, wgAlpha, 50, now.Add(-time.Hour))
	recordDashboardSessionProposedEvent(t, env, orgAlpha, alphaID, wgAlpha, now.Add(-time.Hour))
	recordDashboardEnvelopeEvent(t, env, orgBeta, betaID, wgBeta, 30, now.Add(-time.Hour))
	recordDashboardSessionProposedEvent(t, env, orgBeta, betaID, wgBeta, now.Add(-time.Hour))

	assertDashboardUnauthorized(t, env, "/dashboard/activity")

	alpha := env.accountClient(t, alphaToken)
	res, err := alpha.GetDashboardActivity(env.ctx, api.GetDashboardActivityParams{
		Window: api.NewOptDashboardWindow(api.DashboardWindow24h),
		Bucket: api.NewOptDashboardBucket(api.DashboardBucket1h),
	})
	if err != nil {
		t.Fatalf("get dashboard activity: %v", err)
	}
	activity, ok := res.(*api.DashboardActivityResponse)
	if !ok {
		t.Fatalf("activity unexpected response: %T", res)
	}
	if got := sumDashboardBucketEnvelopes(activity.Buckets); got != 50 {
		t.Fatalf("expected 50 alpha envelopes, got %d", got)
	}
	if got := sumDashboardBucketSessions(activity.Buckets); got != 1 {
		t.Fatalf("expected 1 alpha session, got %d", got)
	}
	if len(activity.ByWorkgroup) != 1 {
		t.Fatalf("expected 1 alpha workgroup row, got %d", len(activity.ByWorkgroup))
	}
	if activity.ByWorkgroup[0].WorkgroupId != wgAlpha || activity.ByWorkgroup[0].Envelopes != 50 {
		t.Fatalf("unexpected alpha workgroup breakdown: %+v", activity.ByWorkgroup)
	}

	bad, err := alpha.GetDashboardActivity(env.ctx, api.GetDashboardActivityParams{
		Window: api.NewOptDashboardWindow(api.DashboardWindow24h),
		Bucket: api.NewOptDashboardBucket(api.DashboardBucket1d),
	})
	if err != nil {
		t.Fatalf("get dashboard activity bad request: %v", err)
	}
	if _, ok := bad.(*api.GetDashboardActivityBadRequest); !ok {
		t.Fatalf("expected bad request for bucket >= window, got %T", bad)
	}
}

func TestDashboardEnvironmentsEndpoint(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgAlpha, alphaID, alphaToken := env.createOrgWithAccount(t, "Alpha Co", "alpha-env@example.com")
	orgBeta, betaID, _ := env.createOrgWithAccount(t, "Beta Co", "beta-env@example.com")
	now := time.Now().UTC()
	alphaEnvID := seedDashboardEnvironment(t, env, orgAlpha, alphaID, "alpha-host", persistence.EnvironmentStateEnabled, &now)
	seedDashboardEnvironment(t, env, orgBeta, betaID, "beta-host", persistence.EnvironmentStateEnabled, &now)

	assertDashboardUnauthorized(t, env, "/dashboard/environments")

	alpha := env.accountClient(t, alphaToken)
	res, err := alpha.GetDashboardEnvironments(env.ctx)
	if err != nil {
		t.Fatalf("get dashboard environments: %v", err)
	}
	environments, ok := res.(*api.DashboardEnvironmentsResponse)
	if !ok {
		t.Fatalf("environments unexpected response: %T", res)
	}
	if len(*environments) != 1 {
		t.Fatalf("expected 1 alpha environment, got %d", len(*environments))
	}
	got := (*environments)[0]
	if got.ID != alphaEnvID || got.Name != "alpha-host" || got.AccountId != alphaID {
		t.Fatalf("unexpected alpha environment row: %+v", got)
	}
	if got.Status != api.DashboardEnvironmentStatusOnline || !got.LastHeartbeatAt.Set {
		t.Fatalf("expected online status with heartbeat, got %+v", got)
	}
}

func TestWorkgroupsActivityEndpoint(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgID, accountAID, tokenA := env.createOrgWithAccount(t, "Activity Org", "a-workgroups@example.com")
	accountBID, tokenB := env.addAccountToOrg(t, orgID, "b-workgroups@example.com")
	accountOtherID, _ := env.addAccountToOrg(t, orgID, "other-workgroups@example.com")

	wgA1 := env.seedIntraOrgWorkgroup(t, orgID, "a-one", accountAID)
	wgA2 := env.seedIntraOrgWorkgroup(t, orgID, "a-zero", accountAID)
	wgB1 := env.seedIntraOrgWorkgroup(t, orgID, "b-one", accountBID)
	wgB2 := env.seedIntraOrgWorkgroup(t, orgID, "b-zero", accountBID)
	wgShared := env.seedIntraOrgWorkgroup(t, orgID, "shared", accountAID)
	wgOther := env.seedIntraOrgWorkgroup(t, orgID, "other", accountOtherID)
	seedDashboardMembership(t, env, orgID, wgShared, accountBID)

	now := time.Now().UTC()
	recordDashboardEnvelopeEvent(t, env, orgID, accountAID, wgA1, 50, now.Add(-time.Hour))
	recordDashboardEnvelopeEvent(t, env, orgID, accountBID, wgB1, 30, now.Add(-time.Hour))
	recordDashboardEnvelopeEvent(t, env, orgID, accountAID, wgShared, 100, now.Add(-time.Hour))
	recordDashboardEnvelopeEvent(t, env, orgID, accountOtherID, wgOther, 200, now.Add(-time.Hour))

	assertDashboardUnauthorized(t, env, "/dashboard/workgroups-activity")

	clientA := env.accountClient(t, tokenA)
	resA, err := clientA.GetWorkgroupsActivity(env.ctx, api.GetWorkgroupsActivityParams{Window: api.NewOptDashboardWindow(api.DashboardWindow24h)})
	if err != nil {
		t.Fatalf("get workgroups activity for A: %v", err)
	}
	aActivity, ok := resA.(*api.WorkgroupsActivityResponse)
	if !ok {
		t.Fatalf("workgroups activity A unexpected response: %T", resA)
	}
	assertDashboardWorkgroupRows(t, aActivity.ByWorkgroup, []api.DashboardWorkgroupActivity{
		{WorkgroupId: wgA1, WorkgroupName: "a-one", Envelopes: 50},
		{WorkgroupId: wgA2, WorkgroupName: "a-zero", Envelopes: 0},
		{WorkgroupId: wgShared, WorkgroupName: "shared", Envelopes: 100},
	})

	clientB := env.accountClient(t, tokenB)
	resB, err := clientB.GetWorkgroupsActivity(env.ctx, api.GetWorkgroupsActivityParams{Window: api.NewOptDashboardWindow(api.DashboardWindow24h)})
	if err != nil {
		t.Fatalf("get workgroups activity for B: %v", err)
	}
	bActivity, ok := resB.(*api.WorkgroupsActivityResponse)
	if !ok {
		t.Fatalf("workgroups activity B unexpected response: %T", resB)
	}
	assertDashboardWorkgroupRows(t, bActivity.ByWorkgroup, []api.DashboardWorkgroupActivity{
		{WorkgroupId: wgB1, WorkgroupName: "b-one", Envelopes: 30},
		{WorkgroupId: wgB2, WorkgroupName: "b-zero", Envelopes: 0},
		{WorkgroupId: wgShared, WorkgroupName: "shared", Envelopes: 100},
	})
}

func TestListAuditEventsEndpoint(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgAlpha, alphaID, alphaToken := env.createOrgWithAccount(t, "Alpha Co", "alpha-audit@example.com")
	orgBeta, betaID, _ := env.createOrgWithAccount(t, "Beta Co", "beta-audit@example.com")
	wgAlpha := env.seedIntraOrgWorkgroup(t, orgAlpha, "alpha-audit", alphaID)
	wgBeta := env.seedIntraOrgWorkgroup(t, orgBeta, "beta-audit", betaID)
	base := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

	alphaOld := recordAuditEventForTest(t, env, persistence.AuditEvent{
		OccurredAt:     base,
		EventType:      persistence.AuditEventAccountLogin,
		OrganizationID: orgAlpha,
		AccountID:      &alphaID,
		Data:           persistence.AuditEventData{"email": "alpha-audit@example.com"},
	})
	alphaMiddle := recordAuditEventForTest(t, env, persistence.AuditEvent{
		OccurredAt:     base.Add(time.Minute),
		EventType:      persistence.AuditEventEnvelopeFlowed,
		OrganizationID: orgAlpha,
		AccountID:      &alphaID,
		WorkgroupID:    &wgAlpha,
		Data:           persistence.AuditEventData{"count_delta": 9},
	})
	alphaLatest := recordAuditEventForTest(t, env, persistence.AuditEvent{
		OccurredAt:     base.Add(2 * time.Minute),
		EventType:      persistence.AuditEventSessionClosed,
		OrganizationID: orgAlpha,
		AccountID:      &alphaID,
		WorkgroupID:    &wgAlpha,
		Data:           persistence.AuditEventData{"close_reason": "consumer_close"},
	})
	recordAuditEventForTest(t, env, persistence.AuditEvent{
		OccurredAt:     base.Add(3 * time.Minute),
		EventType:      persistence.AuditEventEnvelopeFlowed,
		OrganizationID: orgBeta,
		AccountID:      &betaID,
		WorkgroupID:    &wgBeta,
		Data:           persistence.AuditEventData{"count_delta": 99},
	})

	assertDashboardUnauthorized(t, env, "/audit-events")

	alpha := env.accountClient(t, alphaToken)
	page1Res, err := alpha.ListAuditEvents(env.ctx, api.ListAuditEventsParams{Limit: api.NewOptInt(2)})
	if err != nil {
		t.Fatalf("list audit events page 1: %v", err)
	}
	page1 := page1Res.(*api.ListAuditEventsResponse)
	assertAuditResponseIDs(t, page1.Items, []int64{alphaLatest.ID, alphaMiddle.ID})
	if !page1.NextCursor.Set {
		t.Fatal("expected audit cursor")
	}
	for _, event := range page1.Items {
		if event.OrganizationId != orgAlpha {
			t.Fatalf("expected audit row scoped to %s, got %s", orgAlpha, event.OrganizationId)
		}
	}

	page2Res, err := alpha.ListAuditEvents(env.ctx, api.ListAuditEventsParams{
		Limit:  api.NewOptInt(2),
		Cursor: api.NewOptString(page1.NextCursor.Value),
	})
	if err != nil {
		t.Fatalf("list audit events page 2: %v", err)
	}
	page2 := page2Res.(*api.ListAuditEventsResponse)
	assertAuditResponseIDs(t, page2.Items, []int64{alphaOld.ID})
	if page2.NextCursor.Set {
		t.Fatalf("expected final audit page without cursor, got %q", page2.NextCursor.Value)
	}

	filteredRes, err := alpha.ListAuditEvents(env.ctx, api.ListAuditEventsParams{
		EventType:   []api.AuditEventType{api.AuditEventTypeEnvelopeFlowed},
		WorkgroupId: api.NewOptString(wgAlpha),
		AccountId:   api.NewOptString(alphaID),
	})
	if err != nil {
		t.Fatalf("list audit events filtered: %v", err)
	}
	filtered := filteredRes.(*api.ListAuditEventsResponse)
	assertAuditResponseIDs(t, filtered.Items, []int64{alphaMiddle.ID})

	from := base.Add(30 * time.Second)
	to := base.Add(90 * time.Second)
	windowRes, err := alpha.ListAuditEvents(env.ctx, api.ListAuditEventsParams{
		From: api.NewOptDateTime(from),
		To:   api.NewOptDateTime(to),
	})
	if err != nil {
		t.Fatalf("list audit events time window: %v", err)
	}
	windowRows := windowRes.(*api.ListAuditEventsResponse)
	assertAuditResponseIDs(t, windowRows.Items, []int64{alphaMiddle.ID})

	bad, err := alpha.ListAuditEvents(env.ctx, api.ListAuditEventsParams{
		From: api.NewOptDateTime(to),
		To:   api.NewOptDateTime(from),
	})
	if err != nil {
		t.Fatalf("list audit events bad window: %v", err)
	}
	if _, ok := bad.(*api.ListAuditEventsBadRequest); !ok {
		t.Fatalf("expected bad request for inverted time window, got %T", bad)
	}
}

func TestListAuditEventsAllowsSyntheticReferenceIDs(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgAlpha, alphaID, alphaToken := env.createOrgWithAccount(t, "Alpha Co", "alpha-audit-synthetic@example.com")
	base := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	sessionID := "sess_seed_000000000001"
	advertisementID := "ad_seed_equityfeed"

	inserted := recordAuditEventForTest(t, env, persistence.AuditEvent{
		OccurredAt:      base,
		EventType:       persistence.AuditEventSessionProposed,
		OrganizationID:  orgAlpha,
		AccountID:       &alphaID,
		SessionID:       &sessionID,
		AdvertisementID: &advertisementID,
		Data:            persistence.AuditEventData{"source": "synthetic_history"},
	})

	alpha := env.accountClient(t, alphaToken)
	res, err := alpha.ListAuditEvents(env.ctx, api.ListAuditEventsParams{
		From:  api.NewOptDateTime(base.Add(-time.Minute)),
		To:    api.NewOptDateTime(base.Add(time.Minute)),
		Limit: api.NewOptInt(10),
	})
	if err != nil {
		t.Fatalf("list audit events with synthetic reference ids: %v", err)
	}
	page := res.(*api.ListAuditEventsResponse)
	assertAuditResponseIDs(t, page.Items, []int64{inserted.ID})
	got := page.Items[0]
	if got.SessionId.Value != sessionID || got.AdvertisementId.Value != advertisementID {
		t.Fatalf("expected synthetic refs session=%q advertisement=%q, got session=%q advertisement=%q", sessionID, advertisementID, got.SessionId.Value, got.AdvertisementId.Value)
	}
}

func (e *workgroupTestEnv) publishDashboardAdvertisement(t *testing.T, token, name, workgroupID string) string {
	t.Helper()
	client := e.accountClient(t, token)
	res, err := client.PublishAdvertisement(e.ctx, &api.PublishAdvertisementRequest{
		Name:                name,
		Capabilities:        []api.AdvertisementCapability{{Name: "dashboard"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{workgroupID},
		TunnelMode:          api.NewOptAdvertisementTunnelMode(api.AdvertisementTunnelModeTCP),
	})
	if err != nil {
		t.Fatalf("publish dashboard advertisement %q: %v", name, err)
	}
	ad, ok := res.(*api.Advertisement)
	if !ok {
		t.Fatalf("publish dashboard advertisement %q unexpected response: %T", name, res)
	}
	return ad.ID
}

func assertDashboardUnauthorized(t *testing.T, env *workgroupTestEnv, path string) {
	t.Helper()
	resp, err := env.ts.Client().Get(env.baseURL + path)
	if err != nil {
		t.Fatalf("unauthorized dashboard request %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected %s to return 401 without account token, got %d", path, resp.StatusCode)
	}
}

func recordAuditEventForTest(t *testing.T, env *workgroupTestEnv, event persistence.AuditEvent) persistence.AuditEvent {
	t.Helper()
	if err := env.store.AuditEvents.Record(env.ctx, env.store.DB(), event); err != nil {
		t.Fatalf("record audit event: %v", err)
	}
	return latestAuditEventForControllerTest(t, env)
}

func latestAuditEventForControllerTest(t *testing.T, env *workgroupTestEnv) persistence.AuditEvent {
	t.Helper()
	query := fmt.Sprintf(`
select %s
from audit_events
order by id desc
limit 1`, auditEventSelectColumns)
	var event persistence.AuditEvent
	if err := env.store.DB().GetContext(env.ctx, &event, query); err != nil {
		t.Fatalf("get latest audit event: %v", err)
	}
	return event
}

func assertAuditResponseIDs(t *testing.T, events []api.AuditEvent, want []int64) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("expected %d audit events, got %d", len(want), len(events))
	}
	for i := range want {
		if events[i].ID != want[i] {
			t.Fatalf("event %d: expected id %d, got %d", i, want[i], events[i].ID)
		}
	}
}

func seedDashboardEnvironment(t *testing.T, env *workgroupTestEnv, orgID, accountID, host string, state persistence.EnvironmentState, lastSeenAt *time.Time) string {
	t.Helper()
	created, err := env.store.Environments.Create(env.ctx, env.store.DB(), persistence.Environment{
		OrganizationID: orgID,
		AccountID:      accountID,
		Host:           dashboardStringPtr(host),
		ZitiIdentityID: "ziti-" + persistence.NewResourceID(persistence.PrefixEnvironment),
		State:          state,
		LastSeenAt:     lastSeenAt,
	})
	if err != nil {
		t.Fatalf("seed dashboard environment %q: %v", host, err)
	}
	return created.ID
}

func seedDashboardSession(t *testing.T, env *workgroupTestEnv, orgID, accountID, workgroupID, advertisementID string, state persistence.SessionState, proposedAt time.Time) string {
	t.Helper()
	created, err := env.store.Sessions.Create(env.ctx, env.store.DB(), persistence.Session{
		AdvertisementID:        advertisementID,
		WorkgroupID:            workgroupID,
		ProviderAccountID:      accountID,
		ProviderOrganizationID: orgID,
		ConsumerAccountID:      accountID,
		ConsumerOrganizationID: orgID,
		TunnelMode:             persistence.TunnelModeTCP,
		State:                  state,
		ProposedAt:             proposedAt,
	})
	if err != nil {
		t.Fatalf("seed dashboard session: %v", err)
	}
	return created.ID
}

func seedDashboardMembership(t *testing.T, env *workgroupTestEnv, orgID, workgroupID, accountID string) {
	t.Helper()
	if _, err := env.store.WorkgroupMemberships.Create(env.ctx, env.store.DB(), persistence.WorkgroupMembership{
		WorkgroupID:    workgroupID,
		OrganizationID: orgID,
		AccountID:      accountID,
		Role:           persistence.WorkgroupMembershipRoleMember,
	}); err != nil {
		t.Fatalf("seed dashboard membership: %v", err)
	}
}

func recordDashboardEnvelopeEvent(t *testing.T, env *workgroupTestEnv, orgID, accountID, workgroupID string, count int, occurredAt time.Time) {
	t.Helper()
	sess := dashboardAuditSession(orgID, accountID, workgroupID)
	event := persistence.NewEnvelopeFlowedEvent(sess, orgID, accountID, count, count)
	event.OccurredAt = occurredAt
	if err := env.store.AuditEvents.Record(env.ctx, env.store.DB(), event); err != nil {
		t.Fatalf("record dashboard envelope event: %v", err)
	}
}

func recordDashboardSessionProposedEvent(t *testing.T, env *workgroupTestEnv, orgID, accountID, workgroupID string, occurredAt time.Time) {
	t.Helper()
	sess := dashboardAuditSession(orgID, accountID, workgroupID)
	event := persistence.NewSessionProposedEvent(sess, orgID, accountID)
	event.OccurredAt = occurredAt
	if err := env.store.AuditEvents.Record(env.ctx, env.store.DB(), event); err != nil {
		t.Fatalf("record dashboard session proposed event: %v", err)
	}
}

func dashboardAuditSession(orgID, accountID, workgroupID string) persistence.Session {
	return persistence.Session{
		ID:                     persistence.NewResourceID(persistence.PrefixSession),
		AdvertisementID:        persistence.NewResourceID(persistence.PrefixAdvertisement),
		WorkgroupID:            workgroupID,
		ProviderAccountID:      accountID,
		ProviderOrganizationID: orgID,
		ConsumerAccountID:      accountID,
		ConsumerOrganizationID: orgID,
		TunnelMode:             persistence.TunnelModeTCP,
	}
}

func dashboardStringPtr(v string) *string {
	return &v
}

func sumDashboardBucketEnvelopes(buckets []api.DashboardActivityBucket) int {
	total := 0
	for _, bucket := range buckets {
		total += bucket.Envelopes
	}
	return total
}

func sumDashboardBucketSessions(buckets []api.DashboardActivityBucket) int {
	total := 0
	for _, bucket := range buckets {
		total += bucket.Sessions
	}
	return total
}

func assertDashboardWorkgroupRows(t *testing.T, got, want []api.DashboardWorkgroupActivity) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d workgroup activity rows, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i].WorkgroupId != want[i].WorkgroupId || got[i].WorkgroupName != want[i].WorkgroupName || got[i].Envelopes != want[i].Envelopes {
			t.Fatalf("workgroup row %d mismatch: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestSessionProposeVisibility(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, _ := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "alice-feed")

	bob := env.accountClient(t, bobToken)
	res, err := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{
		AdvertisementId: adID, WorkgroupId: wgID,
	})
	if err != nil {
		t.Fatalf("propose (no membership): %v", err)
	}
	if _, ok := res.(*api.ProposeSessionNotFound); !ok {
		t.Fatalf("expected 404 for non-visible ad, got %T", res)
	}

	aliceToken := env.mustTokenForAccount(t, aliceID)
	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}

	res2, err := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{
		AdvertisementId: adID, WorkgroupId: wgID,
	})
	if err != nil {
		t.Fatalf("propose (after join): %v", err)
	}
	sess, ok := res2.(*api.Session)
	if !ok {
		t.Fatalf("expected Session, got %T", res2)
	}
	if sess.State != api.SessionStateProposed {
		t.Fatalf("expected proposed, got %s", sess.State)
	}
	if sess.TunnelMode != api.AdvertisementTunnelModeTCP {
		t.Fatalf("expected tcp, got %s", sess.TunnelMode)
	}
}

func TestSessionProposeWorkgroupNotInScope(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")
	otherWG := env.seedIntraOrgWorkgroup(t, orgA, "other", aliceID)
	_ = wgID

	alice := env.accountClient(t, aliceToken)
	res, err := alice.ProposeSession(env.ctx, &api.ProposeSessionRequest{
		AdvertisementId: adID, WorkgroupId: otherWG,
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, ok := res.(*api.ProposeSessionBadRequest); !ok {
		t.Fatalf("expected 400 workgroup_not_in_scope, got %T", res)
	}
}

func TestSessionAcceptRequiresProvider(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)

	proposeRes, err := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{
		AdvertisementId: adID, WorkgroupId: wgID,
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	sess := proposeRes.(*api.Session)

	// bob (consumer) tries to accept — should be 403 not_provider
	bobEnvID := env.enableEnvironment(t, bobToken)
	acceptBad, err := bob.AcceptSession(env.ctx, &api.AcceptSessionRequest{EnvironmentId: bobEnvID}, api.AcceptSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("accept (consumer): %v", err)
	}
	if _, ok := acceptBad.(*api.AcceptSessionForbidden); !ok {
		t.Fatalf("expected 403 not_provider, got %T", acceptBad)
	}
}

func TestSessionAcceptHappyPath(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)

	proposeRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	sess := proposeRes.(*api.Session)

	aliceEnvID := env.enableEnvironment(t, aliceToken)
	acceptRes, err := alice.AcceptSession(env.ctx, &api.AcceptSessionRequest{EnvironmentId: aliceEnvID}, api.AcceptSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	active, ok := acceptRes.(*api.Session)
	if !ok {
		t.Fatalf("expected Session, got %T", acceptRes)
	}
	if active.State != api.SessionStateActive {
		t.Fatalf("expected active, got %s", active.State)
	}
	if !active.TunnelId.Set {
		t.Fatalf("expected tunnel id populated")
	}
	if !active.AcceptedAt.Set {
		t.Fatalf("expected acceptedAt populated")
	}

	// second accept fails: already active
	acceptAgain, _ := alice.AcceptSession(env.ctx, &api.AcceptSessionRequest{EnvironmentId: aliceEnvID}, api.AcceptSessionParams{SessionId: sess.ID})
	if _, ok := acceptAgain.(*api.AcceptSessionConflict); !ok {
		t.Fatalf("expected 409 invalid_state on re-accept, got %T", acceptAgain)
	}
}

func TestSessionRejectFlow(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)
	proposeRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	sess := proposeRes.(*api.Session)

	rejectRes, err := alice.RejectSession(env.ctx, api.NewOptCloseSessionRequest(api.CloseSessionRequest{Detail: api.NewOptString("not taking new")}), api.RejectSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, ok := rejectRes.(*api.RejectSessionNoContent); !ok {
		t.Fatalf("expected 204, got %T", rejectRes)
	}

	// describe session, expect closed + rejected
	descRes, err := bob.GetSession(env.ctx, api.GetSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	describe, ok := descRes.(*api.Session)
	if !ok {
		t.Fatalf("expected Session, got %T", descRes)
	}
	if describe.State != api.SessionStateClosed {
		t.Fatalf("expected closed, got %s", describe.State)
	}
	if !describe.CloseReason.Set || describe.CloseReason.Value != api.SessionCloseReasonRejected {
		t.Fatalf("expected rejected, got %v", describe.CloseReason)
	}
	if !describe.CloseDetail.Set || describe.CloseDetail.Value != "not taking new" {
		t.Fatalf("expected close detail, got %v", describe.CloseDetail)
	}
}

func TestSessionCloseByConsumer(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)

	proposeRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	sess := proposeRes.(*api.Session)
	aliceEnvID := env.enableEnvironment(t, aliceToken)
	if _, err := alice.AcceptSession(env.ctx, &api.AcceptSessionRequest{EnvironmentId: aliceEnvID}, api.AcceptSessionParams{SessionId: sess.ID}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	closeRes, err := bob.CloseSession(env.ctx, api.OptCloseSessionRequest{}, api.CloseSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, ok := closeRes.(*api.CloseSessionNoContent); !ok {
		t.Fatalf("expected 204, got %T", closeRes)
	}

	// idempotent
	closeAgain, err := bob.CloseSession(env.ctx, api.OptCloseSessionRequest{}, api.CloseSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("close again: %v", err)
	}
	if _, ok := closeAgain.(*api.CloseSessionNoContent); !ok {
		t.Fatalf("expected 204 idempotent, got %T", closeAgain)
	}

	descRes, _ := bob.GetSession(env.ctx, api.GetSessionParams{SessionId: sess.ID})
	d := descRes.(*api.Session)
	if d.State != api.SessionStateClosed {
		t.Fatalf("expected closed, got %s", d.State)
	}
	if !d.CloseReason.Set || d.CloseReason.Value != api.SessionCloseReasonConsumerClose {
		t.Fatalf("expected consumer_close, got %v", d.CloseReason)
	}
}

func TestSessionCloseReasonValidation(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	bobID, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)

	newActiveSession := func() string {
		t.Helper()
		return seedSessionListSession(t, env, orgA, orgA, aliceID, bobID, wgID, adID, persistence.SessionStateActive, time.Now().UTC(), nil)
	}
	closeBody := func(reason api.SessionCloseReason, detail string) api.OptCloseSessionRequest {
		t.Helper()
		req := api.CloseSessionRequest{Reason: api.NewOptSessionCloseReason(reason)}
		if detail != "" {
			req.Detail.SetTo(detail)
		}
		return api.NewOptCloseSessionRequest(req)
	}
	assertClosed := func(sessionID string, reason api.SessionCloseReason, detail string) {
		t.Helper()
		descRes, err := bob.GetSession(env.ctx, api.GetSessionParams{SessionId: sessionID})
		if err != nil {
			t.Fatalf("describe %s: %v", sessionID, err)
		}
		d := descRes.(*api.Session)
		if d.State != api.SessionStateClosed {
			t.Fatalf("expected closed session %s, got %s", sessionID, d.State)
		}
		if !d.CloseReason.Set || d.CloseReason.Value != reason {
			t.Fatalf("expected close reason %s, got %v", reason, d.CloseReason)
		}
		if detail != "" && (!d.CloseDetail.Set || d.CloseDetail.Value != detail) {
			t.Fatalf("expected close detail %q, got %v", detail, d.CloseDetail)
		}
	}

	defaultProviderID := newActiveSession()
	defaultProviderRes, err := alice.CloseSession(env.ctx, api.OptCloseSessionRequest{}, api.CloseSessionParams{SessionId: defaultProviderID})
	if err != nil {
		t.Fatalf("default provider close: %v", err)
	}
	if _, ok := defaultProviderRes.(*api.CloseSessionNoContent); !ok {
		t.Fatalf("expected 204 default provider close, got %T", defaultProviderRes)
	}
	assertClosed(defaultProviderID, api.SessionCloseReasonProviderClose, "")

	providerID := newActiveSession()
	providerRes, err := alice.CloseSession(env.ctx, closeBody(api.SessionCloseReasonProviderClose, "provider maintenance"), api.CloseSessionParams{SessionId: providerID})
	if err != nil {
		t.Fatalf("provider reason close: %v", err)
	}
	if _, ok := providerRes.(*api.CloseSessionNoContent); !ok {
		t.Fatalf("expected 204 provider reason close, got %T", providerRes)
	}
	assertClosed(providerID, api.SessionCloseReasonProviderClose, "provider maintenance")

	consumerID := newActiveSession()
	consumerRes, err := bob.CloseSession(env.ctx, closeBody(api.SessionCloseReasonConsumerClose, "consumer done"), api.CloseSessionParams{SessionId: consumerID})
	if err != nil {
		t.Fatalf("consumer reason close: %v", err)
	}
	if _, ok := consumerRes.(*api.CloseSessionNoContent); !ok {
		t.Fatalf("expected 204 consumer reason close, got %T", consumerRes)
	}
	assertClosed(consumerID, api.SessionCloseReasonConsumerClose, "consumer done")

	violationID := newActiveSession()
	violationDetail := "outbound envelope size 4096 exceeds contract max 1024"
	violationRes, err := alice.CloseSession(env.ctx, closeBody(api.SessionCloseReasonContractViolation, violationDetail), api.CloseSessionParams{SessionId: violationID})
	if err != nil {
		t.Fatalf("contract violation close: %v", err)
	}
	if _, ok := violationRes.(*api.CloseSessionNoContent); !ok {
		t.Fatalf("expected 204 contract violation close, got %T", violationRes)
	}
	assertClosed(violationID, api.SessionCloseReasonContractViolation, violationDetail)

	rejectedID := newActiveSession()
	forbiddenProvider, err := bob.CloseSession(env.ctx, closeBody(api.SessionCloseReasonProviderClose, ""), api.CloseSessionParams{SessionId: rejectedID})
	if err != nil {
		t.Fatalf("consumer claims provider close: %v", err)
	}
	if res, ok := forbiddenProvider.(*api.CloseSessionForbidden); !ok || res.Code != "not_provider" {
		t.Fatalf("expected not_provider forbidden, got %T %+v", forbiddenProvider, forbiddenProvider)
	}
	forbiddenConsumer, err := alice.CloseSession(env.ctx, closeBody(api.SessionCloseReasonConsumerClose, ""), api.CloseSessionParams{SessionId: rejectedID})
	if err != nil {
		t.Fatalf("provider claims consumer close: %v", err)
	}
	if res, ok := forbiddenConsumer.(*api.CloseSessionForbidden); !ok || res.Code != "not_consumer" {
		t.Fatalf("expected not_consumer forbidden, got %T %+v", forbiddenConsumer, forbiddenConsumer)
	}
	forbiddenViolation, err := bob.CloseSession(env.ctx, closeBody(api.SessionCloseReasonContractViolation, ""), api.CloseSessionParams{SessionId: rejectedID})
	if err != nil {
		t.Fatalf("consumer claims contract violation: %v", err)
	}
	if res, ok := forbiddenViolation.(*api.CloseSessionForbidden); !ok || res.Code != "not_provider" {
		t.Fatalf("expected not_provider forbidden, got %T %+v", forbiddenViolation, forbiddenViolation)
	}
	unsupported, err := alice.CloseSession(env.ctx, closeBody(api.SessionCloseReasonAdminClose, ""), api.CloseSessionParams{SessionId: rejectedID})
	if err != nil {
		t.Fatalf("unsupported reason close: %v", err)
	}
	if res, ok := unsupported.(*api.CloseSessionBadRequest); !ok || res.Code != "invalid_request" {
		t.Fatalf("expected invalid_request bad request, got %T %+v", unsupported, unsupported)
	}
}

func TestSessionCloseByNonParticipant(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	_, eveToken := env.addAccountToOrg(t, orgA, "eve@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)
	proposeRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	sess := proposeRes.(*api.Session)

	eve := env.accountClient(t, eveToken)
	closeRes, err := eve.CloseSession(env.ctx, api.OptCloseSessionRequest{}, api.CloseSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, ok := closeRes.(*api.CloseSessionNotFound); !ok {
		t.Fatalf("expected 404 for non-participant, got %T", closeRes)
	}
}

func TestSessionListFiltersAndRoles(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)

	for i := 0; i < 3; i++ {
		if _, err := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID}); err != nil {
			t.Fatalf("propose %d: %v", i, err)
		}
	}

	// bob sees as consumer
	asConsumer, err := bob.ListSessions(env.ctx, api.ListSessionsParams{Role: api.NewOptListSessionsRole(api.ListSessionsRoleConsumer)})
	if err != nil {
		t.Fatalf("list consumer: %v", err)
	}
	cResp := asConsumer.(*api.ListSessionsResponse)
	if len(*cResp) != 3 {
		t.Fatalf("expected 3 consumer sessions, got %d", len(*cResp))
	}

	// alice sees as provider
	asProvider, err := alice.ListSessions(env.ctx, api.ListSessionsParams{Role: api.NewOptListSessionsRole(api.ListSessionsRoleProvider)})
	if err != nil {
		t.Fatalf("list provider: %v", err)
	}
	pResp := asProvider.(*api.ListSessionsResponse)
	if len(*pResp) != 3 {
		t.Fatalf("expected 3 provider sessions, got %d", len(*pResp))
	}
}

func TestSessionListSortLimitAndClosedFilter(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "session-sort-org", "alice-sort@example.com")
	bobID, _ := env.addAccountToOrg(t, orgA, "bob-sort@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "sort-workgroup", aliceID)
	seedDashboardMembership(t, env, orgA, wgID, bobID)
	adID := seedSessionListAdvertisement(t, env, orgA, aliceID, "sort-ad", wgID)

	base := time.Now().UTC().Truncate(time.Second)
	openID := seedSessionListSession(t, env, orgA, orgA, aliceID, bobID, wgID, adID, persistence.SessionStateProposed, base.Add(-10*time.Minute), nil)
	oldClosedID := seedSessionListSession(t, env, orgA, orgA, aliceID, bobID, wgID, adID, persistence.SessionStateClosed, base.Add(-9*time.Minute), controllerTimePtr(base.Add(-7*time.Minute)))
	newClosedID := seedSessionListSession(t, env, orgA, orgA, aliceID, bobID, wgID, adID, persistence.SessionStateClosed, base.Add(-8*time.Minute), controllerTimePtr(base.Add(-1*time.Minute)))
	midClosedID := seedSessionListSession(t, env, orgA, orgA, aliceID, bobID, wgID, adID, persistence.SessionStateClosed, base.Add(-7*time.Minute), controllerTimePtr(base.Add(-3*time.Minute)))

	alice := env.accountClient(t, aliceToken)
	defaultRes, err := alice.ListSessions(env.ctx, api.ListSessionsParams{Role: api.NewOptListSessionsRole(api.ListSessionsRoleProvider)})
	if err != nil {
		t.Fatalf("list default sessions: %v", err)
	}
	defaultRows := defaultRes.(*api.ListSessionsResponse)
	assertSessionIDOrder(t, *defaultRows, []string{midClosedID, newClosedID, oldClosedID, openID})

	recentRes, err := alice.ListSessions(env.ctx, api.ListSessionsParams{
		Role:  api.NewOptListSessionsRole(api.ListSessionsRoleProvider),
		Sort:  api.NewOptListSessionsSort(api.ListSessionsSortClosedAtDesc),
		Limit: api.NewOptInt(2),
	})
	if err != nil {
		t.Fatalf("list recent closed sessions: %v", err)
	}
	recentRows := recentRes.(*api.ListSessionsResponse)
	assertSessionIDOrder(t, *recentRows, []string{newClosedID, midClosedID})
	for _, row := range *recentRows {
		if !row.ClosedAt.Set {
			t.Fatalf("expected closed_at on recent closed row %+v", row)
		}
	}

	openWithClosedSort, err := alice.ListSessions(env.ctx, api.ListSessionsParams{
		State: []api.SessionState{
			api.SessionStateProposed,
			api.SessionStateAccepting,
			api.SessionStateActive,
			api.SessionStateClosing,
		},
		Role: api.NewOptListSessionsRole(api.ListSessionsRoleProvider),
		Sort: api.NewOptListSessionsSort(api.ListSessionsSortClosedAtDesc),
	})
	if err != nil {
		t.Fatalf("list open states with closed sort: %v", err)
	}
	if rows := openWithClosedSort.(*api.ListSessionsResponse); len(*rows) != 0 {
		t.Fatalf("expected closed sort to exclude open rows, got %d", len(*rows))
	}

	tooLarge, err := alice.ListSessions(env.ctx, api.ListSessionsParams{Limit: api.NewOptInt(201)})
	if err != nil {
		t.Fatalf("list with excessive limit: %v", err)
	}
	if _, ok := tooLarge.(*api.ListSessionsBadRequest); !ok {
		t.Fatalf("expected bad request for limit > 200, got %T", tooLarge)
	}
}

func TestSessionListDisplayFieldsAndEmailRedaction(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	providerOrgID, providerAccountID, providerToken := env.createOrgWithAccount(t, "Provider Org", "provider-display@example.com")
	sameOrgConsumerID, _ := env.addAccountToOrg(t, providerOrgID, "consumer-same-org@example.com")
	consumerOrgID, consumerAccountID, consumerToken := env.createOrgWithAccount(t, "Consumer Org", "consumer-cross-org@example.com")

	intraWG := env.seedIntraOrgWorkgroup(t, providerOrgID, "intra-display", providerAccountID)
	seedDashboardMembership(t, env, providerOrgID, intraWG, sameOrgConsumerID)
	intraAd := seedSessionListAdvertisement(t, env, providerOrgID, providerAccountID, "intra-ad", intraWG)

	interWG := seedSessionListWorkgroup(t, env, providerOrgID, "inter-display")
	seedDashboardMembership(t, env, providerOrgID, interWG, providerAccountID)
	seedDashboardMembership(t, env, consumerOrgID, interWG, consumerAccountID)
	interAd := seedSessionListAdvertisement(t, env, providerOrgID, providerAccountID, "inter-ad", interWG)

	base := time.Now().UTC().Truncate(time.Second)
	intraID := seedSessionListSession(t, env, providerOrgID, providerOrgID, providerAccountID, sameOrgConsumerID, intraWG, intraAd, persistence.SessionStateProposed, base.Add(-2*time.Minute), nil)
	interID := seedSessionListSession(t, env, providerOrgID, consumerOrgID, providerAccountID, consumerAccountID, interWG, interAd, persistence.SessionStateProposed, base.Add(-time.Minute), nil)

	provider := env.accountClient(t, providerToken)
	providerRes, err := provider.ListSessions(env.ctx, api.ListSessionsParams{Role: api.NewOptListSessionsRole(api.ListSessionsRoleProvider)})
	if err != nil {
		t.Fatalf("provider list sessions: %v", err)
	}
	providerRows := *providerRes.(*api.ListSessionsResponse)

	intra := findSessionRow(t, providerRows, intraID)
	assertSessionDisplayNames(t, intra, "intra-ad", "intra-display", "Provider Org", "Provider Org")
	if !intra.ProviderAccountEmail.Set || intra.ProviderAccountEmail.Value != "provider-display@example.com" {
		t.Fatalf("expected same-org provider email, got %+v", intra.ProviderAccountEmail)
	}
	if !intra.ConsumerAccountEmail.Set || intra.ConsumerAccountEmail.Value != "consumer-same-org@example.com" {
		t.Fatalf("expected same-org consumer email, got %+v", intra.ConsumerAccountEmail)
	}

	interAsProvider := findSessionRow(t, providerRows, interID)
	assertSessionDisplayNames(t, interAsProvider, "inter-ad", "inter-display", "Provider Org", "Consumer Org")
	if !interAsProvider.ProviderAccountEmail.Set || interAsProvider.ProviderAccountEmail.Value != "provider-display@example.com" {
		t.Fatalf("expected provider-side email, got %+v", interAsProvider.ProviderAccountEmail)
	}
	if interAsProvider.ConsumerAccountEmail.Set {
		t.Fatalf("expected cross-org consumer email to be omitted, got %+v", interAsProvider.ConsumerAccountEmail)
	}

	consumer := env.accountClient(t, consumerToken)
	consumerRes, err := consumer.ListSessions(env.ctx, api.ListSessionsParams{Role: api.NewOptListSessionsRole(api.ListSessionsRoleConsumer)})
	if err != nil {
		t.Fatalf("consumer list sessions: %v", err)
	}
	interAsConsumer := findSessionRow(t, *consumerRes.(*api.ListSessionsResponse), interID)
	assertSessionDisplayNames(t, interAsConsumer, "inter-ad", "inter-display", "Provider Org", "Consumer Org")
	if interAsConsumer.ProviderAccountEmail.Set {
		t.Fatalf("expected cross-org provider email to be omitted, got %+v", interAsConsumer.ProviderAccountEmail)
	}
	if !interAsConsumer.ConsumerAccountEmail.Set || interAsConsumer.ConsumerAccountEmail.Value != "consumer-cross-org@example.com" {
		t.Fatalf("expected consumer-side email, got %+v", interAsConsumer.ConsumerAccountEmail)
	}
}

func seedSessionListWorkgroup(t *testing.T, env *workgroupTestEnv, ownerOrgID, name string) string {
	t.Helper()
	wg, err := env.store.Workgroups.Create(env.ctx, env.store.DB(), persistence.Workgroup{
		OwnerOrganizationID: ownerOrgID,
		Name:                name,
		Scope:               persistence.WorkgroupScopeInterOrg,
		State:               persistence.WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("seed session list workgroup %q: %v", name, err)
	}
	return wg.ID
}

func seedSessionListAdvertisement(t *testing.T, env *workgroupTestEnv, orgID, accountID, name, workgroupID string) string {
	t.Helper()
	ad, err := env.store.Advertisements.Create(env.ctx, env.store.DB(), persistence.Advertisement{
		OrganizationID:      orgID,
		AccountID:           accountID,
		Name:                name,
		Capabilities:        persistence.CapabilitiesJSON{{Name: "sessions"}},
		InteractionPatterns: persistence.InteractionPatternsJSON{{Kind: persistence.InteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{workgroupID},
		TunnelMode:          persistence.TunnelModeTCP,
	})
	if err != nil {
		t.Fatalf("seed session list advertisement %q: %v", name, err)
	}
	return ad.ID
}

func seedSessionListSession(t *testing.T, env *workgroupTestEnv, providerOrgID, consumerOrgID, providerAccountID, consumerAccountID, workgroupID, advertisementID string, state persistence.SessionState, proposedAt time.Time, closedAt *time.Time) string {
	t.Helper()
	var reason *persistence.SessionCloseReason
	if state == persistence.SessionStateClosed {
		v := persistence.SessionCloseReasonConsumerClose
		reason = &v
	}
	sess, err := env.store.Sessions.Create(env.ctx, env.store.DB(), persistence.Session{
		AdvertisementID:        advertisementID,
		WorkgroupID:            workgroupID,
		ProviderAccountID:      providerAccountID,
		ProviderOrganizationID: providerOrgID,
		ConsumerAccountID:      consumerAccountID,
		ConsumerOrganizationID: consumerOrgID,
		TunnelMode:             persistence.TunnelModeTCP,
		State:                  state,
		CloseReason:            reason,
		ProposedAt:             proposedAt,
		ClosedAt:               closedAt,
	})
	if err != nil {
		t.Fatalf("seed session list session: %v", err)
	}
	return sess.ID
}

func controllerTimePtr(v time.Time) *time.Time {
	return &v
}

func assertSessionIDOrder(t *testing.T, rows api.ListSessionsResponse, want []string) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("expected %d session rows, got %d", len(want), len(rows))
	}
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.ID)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("session order mismatch: got %v want %v", got, want)
		}
	}
}

func findSessionRow(t *testing.T, rows api.ListSessionsResponse, sessionID string) api.Session {
	t.Helper()
	for _, row := range rows {
		if row.ID == sessionID {
			return row
		}
	}
	t.Fatalf("session row %q not found in %+v", sessionID, rows)
	return api.Session{}
}

func assertSessionDisplayNames(t *testing.T, row api.Session, advertisementName, workgroupName, providerOrgName, consumerOrgName string) {
	t.Helper()
	if row.AdvertisementName != advertisementName {
		t.Fatalf("expected advertisementName %q, got %q", advertisementName, row.AdvertisementName)
	}
	if row.WorkgroupName != workgroupName {
		t.Fatalf("expected workgroupName %q, got %q", workgroupName, row.WorkgroupName)
	}
	if row.ProviderOrganizationName != providerOrgName {
		t.Fatalf("expected providerOrganizationName %q, got %q", providerOrgName, row.ProviderOrganizationName)
	}
	if row.ConsumerOrganizationName != consumerOrgName {
		t.Fatalf("expected consumerOrganizationName %q, got %q", consumerOrgName, row.ConsumerOrganizationName)
	}
}

func TestSessionDescribeByNonParticipant(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	_, eveToken := env.addAccountToOrg(t, orgA, "eve@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)
	proposeRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	sess := proposeRes.(*api.Session)

	eve := env.accountClient(t, eveToken)
	res, err := eve.GetSession(env.ctx, api.GetSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if _, ok := res.(*api.GetSessionNotFound); !ok {
		t.Fatalf("expected 404 for non-participant describe, got %T", res)
	}
}

func TestSessionAdminClose(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)
	proposeRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	sess := proposeRes.(*api.Session)

	admin := env.adminClient(t)
	res, err := admin.AdminCloseSession(env.ctx, api.NewOptCloseSessionRequest(api.CloseSessionRequest{Detail: api.NewOptString("violation")}), api.AdminCloseSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("admin close: %v", err)
	}
	if _, ok := res.(*api.AdminCloseSessionNoContent); !ok {
		t.Fatalf("expected 204 admin close, got %T", res)
	}

	descRes, _ := bob.GetSession(env.ctx, api.GetSessionParams{SessionId: sess.ID})
	d := descRes.(*api.Session)
	if d.State != api.SessionStateClosed || !d.CloseReason.Set || d.CloseReason.Value != api.SessionCloseReasonAdminClose {
		t.Fatalf("unexpected close: state=%s reason=%v", d.State, d.CloseReason)
	}
}

func TestSessionSelfSessionAllowed(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	proposeRes, err := alice.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("propose self: %v", err)
	}
	sess, ok := proposeRes.(*api.Session)
	if !ok {
		t.Fatalf("expected Session, got %T", proposeRes)
	}
	if sess.ProviderAccountId != sess.ConsumerAccountId {
		t.Fatalf("expected self session, provider=%s consumer=%s", sess.ProviderAccountId, sess.ConsumerAccountId)
	}
	envID := env.enableEnvironment(t, aliceToken)
	acceptRes, err := alice.AcceptSession(env.ctx, &api.AcceptSessionRequest{EnvironmentId: envID}, api.AcceptSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("accept self: %v", err)
	}
	active, ok := acceptRes.(*api.Session)
	if !ok {
		t.Fatalf("expected active Session, got %T", acceptRes)
	}
	if active.State != api.SessionStateActive {
		t.Fatalf("expected active, got %s", active.State)
	}
}

// --- contract tests ---

func seedContractAndAdvertisement(t *testing.T, env *workgroupTestEnv, orgA, aliceID, wgID string) (contractID, adID string) {
	t.Helper()
	alice := env.accountClient(t, env.mustTokenForAccount(t, aliceID))
	cRes, err := alice.CreateContract(env.ctx, &api.CreateContractRequest{
		Name:               "demo-contract",
		MaxDurationSeconds: api.NewOptInt(60),
	})
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	c, ok := cRes.(*api.Contract)
	if !ok {
		t.Fatalf("create contract unexpected: %T", cRes)
	}
	adRes, err := alice.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "alice-feed",
		Capabilities:        []api.AdvertisementCapability{{Name: "quote"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
		ContractId:          api.NewOptString(c.ID),
	})
	if err != nil {
		t.Fatalf("publish ad with contract: %v", err)
	}
	ad, ok := adRes.(*api.Advertisement)
	if !ok {
		t.Fatalf("publish ad unexpected: %T", adRes)
	}
	if !ad.ContractId.Set || ad.ContractId.Value != c.ID {
		t.Fatalf("contractId not echoed on advertisement: %v", ad.ContractId)
	}
	_ = orgA
	return c.ID, ad.ID
}

func TestContractCreateAndList(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)

	if _, err := alice.CreateContract(env.ctx, &api.CreateContractRequest{Name: "a"}); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := alice.CreateContract(env.ctx, &api.CreateContractRequest{Name: "b"}); err != nil {
		t.Fatalf("create b: %v", err)
	}
	res, err := alice.ListContracts(env.ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	listing := res.(*api.ListContractsResponse)
	if len(*listing) != 2 {
		t.Fatalf("expected 2 contracts, got %d", len(*listing))
	}

	// name collision
	res2, err := alice.CreateContract(env.ctx, &api.CreateContractRequest{Name: "A"})
	if err != nil {
		t.Fatalf("create collision: %v", err)
	}
	if _, ok := res2.(*api.CreateContractConflict); !ok {
		t.Fatalf("expected 409 name_in_use, got %T", res2)
	}
}

func TestContractCreateUnknownWorkgroup(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)
	res, err := alice.CreateContract(env.ctx, &api.CreateContractRequest{
		Name:                         "ghost",
		RequiredWorkgroupMemberships: []string{"wg_zzzzzzzzzzzz"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, ok := res.(*api.CreateContractBadRequest); !ok {
		t.Fatalf("expected 400 unknown_workgroup, got %T", res)
	}
}

func TestContractUpdateAndDeleteInUse(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "shared", aliceID)
	contractID, adID := seedContractAndAdvertisement(t, env, orgA, aliceID, wgID)

	alice := env.accountClient(t, aliceToken)

	// delete while in use → 409
	delRes, err := alice.DeleteContract(env.ctx, api.DeleteContractParams{ContractId: contractID})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := delRes.(*api.DeleteContractConflict); !ok {
		t.Fatalf("expected 409 contract_in_use, got %T", delRes)
	}

	// update works
	updRes, err := alice.UpdateContract(env.ctx,
		&api.UpdateContractRequest{MaxDurationSeconds: api.NewOptInt(120)},
		api.UpdateContractParams{ContractId: contractID})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, ok := updRes.(*api.Contract)
	if !ok {
		t.Fatalf("update unexpected: %T", updRes)
	}
	if updated.MaxDurationSeconds != 120 {
		t.Fatalf("update max duration not applied: %d", updated.MaxDurationSeconds)
	}

	// retract the advertisement to unblock delete
	if _, err := alice.RetractAdvertisement(env.ctx, api.RetractAdvertisementParams{AdvertisementId: adID}); err != nil {
		t.Fatalf("retract ad: %v", err)
	}
	delRes2, err := alice.DeleteContract(env.ctx, api.DeleteContractParams{ContractId: contractID})
	if err != nil {
		t.Fatalf("delete after retract: %v", err)
	}
	if _, ok := delRes2.(*api.DeleteContractNoContent); !ok {
		t.Fatalf("expected 204 after retract, got %T", delRes2)
	}
}

func TestContractAdvertisementPublishUnknownContract(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "shared", aliceID)

	alice := env.accountClient(t, aliceToken)
	res, err := alice.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "mystery",
		Capabilities:        []api.AdvertisementCapability{{Name: "x"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
		ContractId:          api.NewOptString("con_zzzzzzzzzzzz"),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, ok := res.(*api.PublishAdvertisementBadRequest); !ok {
		t.Fatalf("expected 400 unknown_contract, got %T", res)
	}
}

func TestContractAdmissionMissingMembership(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")

	// advertisement workgroup — both alice and bob will be members
	sharedWG := env.seedIntraOrgWorkgroup(t, orgA, "shared", aliceID)
	// contract requires membership in this extra workgroup, which bob is NOT in
	extraWG := env.seedIntraOrgWorkgroup(t, orgA, "extra", aliceID)

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: sharedWG}); err != nil {
		t.Fatalf("add bob to shared: %v", err)
	}

	cRes, err := alice.CreateContract(env.ctx, &api.CreateContractRequest{
		Name:                         "strict",
		RequiredWorkgroupMemberships: []string{extraWG},
	})
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	c := cRes.(*api.Contract)

	adRes, err := alice.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "gated",
		Capabilities:        []api.AdvertisementCapability{{Name: "q"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{sharedWG},
		ContractId:          api.NewOptString(c.ID),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	ad := adRes.(*api.Advertisement)

	bob := env.accountClient(t, bobToken)
	propRes, err := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{
		AdvertisementId: ad.ID,
		WorkgroupId:     sharedWG,
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	sess := propRes.(*api.Session)
	aliceEnvID := env.enableEnvironment(t, aliceToken)
	acceptRes, err := alice.AcceptSession(env.ctx,
		&api.AcceptSessionRequest{EnvironmentId: aliceEnvID},
		api.AcceptSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, ok := acceptRes.(*api.AcceptSessionForbidden); !ok {
		t.Fatalf("expected 403 contract_violation_memberships, got %T", acceptRes)
	}

	// session transitioned to closed(contract_violation)
	descRes, _ := bob.GetSession(env.ctx, api.GetSessionParams{SessionId: sess.ID})
	d := descRes.(*api.Session)
	if d.State != api.SessionStateClosed {
		t.Fatalf("expected closed, got %s", d.State)
	}
	if !d.CloseReason.Set || d.CloseReason.Value != api.SessionCloseReasonContractViolation {
		t.Fatalf("expected contract_violation, got %v", d.CloseReason)
	}
}

func TestContractAdmissionPassAndSnapshot(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "shared", aliceID)

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}

	contractID, adID := seedContractAndAdvertisement(t, env, orgA, aliceID, wgID)
	_ = contractID
	bob := env.accountClient(t, bobToken)
	propRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	sess := propRes.(*api.Session)

	aliceEnvID := env.enableEnvironment(t, aliceToken)
	acceptRes, err := alice.AcceptSession(env.ctx,
		&api.AcceptSessionRequest{EnvironmentId: aliceEnvID},
		api.AcceptSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	active, ok := acceptRes.(*api.Session)
	if !ok {
		t.Fatalf("expected active session, got %T", acceptRes)
	}
	if !active.ContractSnapshot.Set {
		t.Fatalf("expected contractSnapshot populated on active session")
	}
	snap := active.ContractSnapshot.Value
	if snap.ContractId != contractID {
		t.Fatalf("snapshot contractId mismatch: %s vs %s", snap.ContractId, contractID)
	}
	if snap.MaxDurationSeconds != 60 {
		t.Fatalf("snapshot max duration mismatch: %d", snap.MaxDurationSeconds)
	}
	if snap.SnapshottedAt.IsZero() {
		t.Fatalf("snapshot timestamp not populated")
	}
}

func TestContractDurationReaper(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "shared", aliceID)

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	cRes, err := alice.CreateContract(env.ctx, &api.CreateContractRequest{
		Name:               "tight",
		MaxDurationSeconds: api.NewOptInt(1),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	c := cRes.(*api.Contract)
	adRes, err := alice.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "short-lived",
		Capabilities:        []api.AdvertisementCapability{{Name: "q"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
		ContractId:          api.NewOptString(c.ID),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	ad := adRes.(*api.Advertisement)

	bob := env.accountClient(t, bobToken)
	propRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: ad.ID, WorkgroupId: wgID})
	sess := propRes.(*api.Session)
	aliceEnvID := env.enableEnvironment(t, aliceToken)
	if _, err := alice.AcceptSession(env.ctx,
		&api.AcceptSessionRequest{EnvironmentId: aliceEnvID},
		api.AcceptSessionParams{SessionId: sess.ID}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	// wait past the cap, then invoke the reaper directly
	time.Sleep(1500 * time.Millisecond)
	if err := env.service.ReapExpiredSessions(env.ctx, time.Now().UTC()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	descRes, _ := bob.GetSession(env.ctx, api.GetSessionParams{SessionId: sess.ID})
	d := descRes.(*api.Session)
	if d.State != api.SessionStateClosed {
		t.Fatalf("expected closed after reap, got %s", d.State)
	}
	if !d.CloseReason.Set || d.CloseReason.Value != api.SessionCloseReasonContractViolation {
		t.Fatalf("expected contract_violation, got %v", d.CloseReason)
	}
}

func TestSessionEnvelopeCountReportAndVisibility(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID, adID := seedSharedWorkgroupAd(t, env, orgA, aliceID, "shared", "ad")

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	bob := env.accountClient(t, bobToken)

	propRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	sess := propRes.(*api.Session)

	aliceEnvID := env.enableEnvironment(t, aliceToken)
	if _, err := alice.AcceptSession(env.ctx,
		&api.AcceptSessionRequest{EnvironmentId: aliceEnvID, BackendAddress: api.NewOptString("127.0.0.1:9000")},
		api.AcceptSessionParams{SessionId: sess.ID}); err != nil {
		t.Fatalf("accept: %v", err)
	}

	// consumer cannot report (not provider)
	consReport, err := bob.ReportSessionEnvelopeCount(env.ctx,
		&api.ReportEnvelopeCountRequest{Count: 5, ObservedAt: time.Now().UTC()},
		api.ReportSessionEnvelopeCountParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("report as consumer: %v", err)
	}
	if _, ok := consReport.(*api.ReportSessionEnvelopeCountForbidden); !ok {
		t.Fatalf("expected 403 not_provider, got %T", consReport)
	}

	// provider reports 3
	rep3, err := alice.ReportSessionEnvelopeCount(env.ctx,
		&api.ReportEnvelopeCountRequest{Count: 3, ObservedAt: time.Now().UTC()},
		api.ReportSessionEnvelopeCountParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("report 3: %v", err)
	}
	if _, ok := rep3.(*api.ReportSessionEnvelopeCountNoContent); !ok {
		t.Fatalf("expected 204, got %T", rep3)
	}

	descRes, _ := bob.GetSession(env.ctx, api.GetSessionParams{SessionId: sess.ID})
	d := descRes.(*api.Session)
	if !d.EnvelopeCount.Set || d.EnvelopeCount.Value != 3 {
		t.Fatalf("expected envelopeCount=3, got %v", d.EnvelopeCount)
	}

	// stale lower report is ignored
	_, _ = alice.ReportSessionEnvelopeCount(env.ctx,
		&api.ReportEnvelopeCountRequest{Count: 1, ObservedAt: time.Now().UTC()},
		api.ReportSessionEnvelopeCountParams{SessionId: sess.ID})
	descRes2, _ := bob.GetSession(env.ctx, api.GetSessionParams{SessionId: sess.ID})
	if descRes2.(*api.Session).EnvelopeCount.Value != 3 {
		t.Fatalf("expected count to stay at 3 after stale lower report")
	}
}

func TestContractSnapshotCarriesMaxEnvelopeBytes(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")
	wgID := env.seedIntraOrgWorkgroup(t, orgA, "shared", aliceID)

	alice := env.accountClient(t, aliceToken)
	if _, err := alice.AddWorkgroupMember(env.ctx, &api.AddWorkgroupMemberRequest{AccountEmail: "bob@example.com"}, api.AddWorkgroupMemberParams{WorkgroupId: wgID}); err != nil {
		t.Fatalf("add bob: %v", err)
	}

	cRes, err := alice.CreateContract(env.ctx, &api.CreateContractRequest{
		Name:             "bytes-capped",
		MaxEnvelopeBytes: api.NewOptInt(1024),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	c := cRes.(*api.Contract)
	if c.MaxEnvelopeBytes != 1024 {
		t.Fatalf("create did not echo maxEnvelopeBytes: %d", c.MaxEnvelopeBytes)
	}

	adRes, err := alice.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "capped",
		Capabilities:        []api.AdvertisementCapability{{Name: "q"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
		ContractId:          api.NewOptString(c.ID),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	ad := adRes.(*api.Advertisement)

	bob := env.accountClient(t, bobToken)
	propRes, _ := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: ad.ID, WorkgroupId: wgID})
	sess := propRes.(*api.Session)

	aliceEnvID := env.enableEnvironment(t, aliceToken)
	acceptRes, err := alice.AcceptSession(env.ctx,
		&api.AcceptSessionRequest{EnvironmentId: aliceEnvID},
		api.AcceptSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	active, ok := acceptRes.(*api.Session)
	if !ok {
		t.Fatalf("expected active: %T", acceptRes)
	}
	if !active.ContractSnapshot.Set {
		t.Fatalf("expected snapshot populated")
	}
	snap := active.ContractSnapshot.Value
	if snap.MaxEnvelopeBytes != 1024 {
		t.Fatalf("snapshot mtu mismatch: %d", snap.MaxEnvelopeBytes)
	}
}
