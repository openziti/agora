package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

// uniqueEnvironmentIdentitiesHeld mirrors uniqueEnvironmentIdentities but returns the tunnel lifecycle
// fake so tests can assert terminator eviction. Each enabled environment gets a distinct ziti identity
// (the environments table requires unique ziti_identity_id).
func uniqueEnvironmentIdentitiesHeld(env *workgroupTestEnv) *uniqueDialTunnelLifecycle {
	var seq int
	tunnelLC := &uniqueDialTunnelLifecycle{}
	env.service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		seq++
		return &fakeEnvironmentLifecycle{
			enableResult: &automation.ProvisionedEnvironment{
				IdentityID:     fmt.Sprintf("ziti-env-%d", seq),
				EnrollmentJSON: []byte(`{}`),
				PolicyID:       fmt.Sprintf("erp-%d", seq),
			},
		}, tunnelLC, nil
	}
	return tunnelLC
}

func createProxyTunnel(t *testing.T, env *workgroupTestEnv, client *api.Client, name string) *api.Tunnel {
	t.Helper()
	res, err := client.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
		Name:          name,
		Mode:          api.TunnelModeTCP,
		BackendTarget: api.NewOptString("127.0.0.1:8443"),
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}
	tunnel, ok := res.(*api.Tunnel)
	if !ok {
		t.Fatalf("unexpected create tunnel response: %T", res)
	}
	if tunnel.EnvironmentId.Set {
		t.Fatalf("expected standalone tunnel to have no environment, got %q", tunnel.EnvironmentId.Value)
	}
	return tunnel
}

// a standalone tunnel is account-owned, so any of the account's environments may host it -- not just
// the one that created it (which, for a standalone tunnel, is none).
func TestServeFromSecondEnvironmentOfOwningAccount(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	uniqueEnvironmentIdentitiesHeld(env)

	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)
	env.enableEnvironment(t, aliceToken)
	secondEnvID := env.enableEnvironment(t, aliceToken)

	tunnel := createProxyTunnel(t, env, alice, "llm-gateway")

	serveRes, err := alice.StartTunnelServe(env.ctx, &api.StartTunnelServeRequest{
		EnvironmentId: secondEnvID,
	}, api.StartTunnelServeParams{TunnelId: tunnel.ID})
	if err != nil {
		t.Fatalf("start tunnel serve from second environment: %v", err)
	}
	if _, ok := serveRes.(*api.TunnelServe); !ok {
		t.Fatalf("expected serve from a second owning-account environment to succeed, got %T", serveRes)
	}
}

// hosting is owning-account-only: a different account's environment may not host the tunnel. grants
// govern reach, not host, so even a granted account cannot serve another account's tunnel.
func TestServeRejectedFromOtherAccountEnvironment(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	uniqueEnvironmentIdentitiesHeld(env)

	orgID, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgID, "bob@example.com")

	alice := env.accountClient(t, aliceToken)
	env.enableEnvironment(t, aliceToken)
	tunnel := createProxyTunnel(t, env, alice, "alice-gateway")

	// bob is a different account in the same organization; he cannot host alice's account-owned tunnel.
	bob := env.accountClient(t, bobToken)
	bobEnvID := env.enableEnvironment(t, bobToken)
	serveRes, err := bob.StartTunnelServe(env.ctx, &api.StartTunnelServeRequest{
		EnvironmentId: bobEnvID,
	}, api.StartTunnelServeParams{TunnelId: tunnel.ID})
	if err != nil {
		t.Fatalf("start tunnel serve request: %v", err)
	}
	if _, ok := serveRes.(*api.TunnelServe); ok {
		t.Fatalf("expected serve from another account's environment to be rejected, but it succeeded")
	}
	if _, ok := serveRes.(*api.StartTunnelServeNotFound); !ok {
		t.Fatalf("expected another account's serve attempt to be rejected as not-found, got %T", serveRes)
	}
}

// takeover on a proxy tunnel evicts the host's fabric terminators and clears the active serve record.
func TestTakeoverProxyTunnelEvictsHostAndClearsServe(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	tunnelLC := uniqueEnvironmentIdentitiesHeld(env)

	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)
	serveEnvID := env.enableEnvironment(t, aliceToken)
	tunnel := createProxyTunnel(t, env, alice, "llm-gateway")

	if _, err := alice.StartTunnelServe(env.ctx, &api.StartTunnelServeRequest{
		EnvironmentId: serveEnvID,
	}, api.StartTunnelServeParams{TunnelId: tunnel.ID}); err != nil {
		t.Fatalf("start tunnel serve: %v", err)
	}
	if _, err := env.store.TunnelServes.GetActiveByTunnel(env.ctx, env.store.DB(), tunnel.ID, orgIDOf(t, env, tunnel.ID)); err != nil {
		t.Fatalf("expected an active serve record before takeover: %v", err)
	}

	takeoverRes, err := alice.TakeoverTunnel(env.ctx, api.TakeoverTunnelParams{TunnelId: tunnel.ID})
	if err != nil {
		t.Fatalf("takeover tunnel: %v", err)
	}
	if _, ok := takeoverRes.(*api.TakeoverTunnelNoContent); !ok {
		t.Fatalf("expected takeover to succeed, got %T", takeoverRes)
	}

	if !containsString(tunnelLC.evictServiceCalls, tunnel.ZitiServiceId.Value) {
		t.Fatalf("expected takeover to evict terminators for service %q, got %#v", tunnel.ZitiServiceId.Value, tunnelLC.evictServiceCalls)
	}
	if _, err := env.store.TunnelServes.GetActiveByTunnel(env.ctx, env.store.DB(), tunnel.ID, orgIDOf(t, env, tunnel.ID)); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("expected the active serve record to be cleared by takeover, got %v", err)
	}
}

// takeover on a direct tunnel evicts its fabric terminators; a direct tunnel keeps no serve record, so
// terminator eviction is the whole job.
func TestTakeoverDirectTunnelEvictsTerminators(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	tunnelLC := uniqueEnvironmentIdentitiesHeld(env)

	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)
	env.enableEnvironment(t, aliceToken)

	createRes, err := alice.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
		Name: "direct-gateway",
		Mode: api.TunnelModeHTTP,
	})
	if err != nil {
		t.Fatalf("create direct tunnel: %v", err)
	}
	tunnel, ok := createRes.(*api.Tunnel)
	if !ok {
		t.Fatalf("unexpected create tunnel response: %T", createRes)
	}

	takeoverRes, err := alice.TakeoverTunnel(env.ctx, api.TakeoverTunnelParams{TunnelId: tunnel.ID})
	if err != nil {
		t.Fatalf("takeover direct tunnel: %v", err)
	}
	if _, ok := takeoverRes.(*api.TakeoverTunnelNoContent); !ok {
		t.Fatalf("expected direct takeover to succeed, got %T", takeoverRes)
	}
	if !containsString(tunnelLC.evictServiceCalls, tunnel.ZitiServiceId.Value) {
		t.Fatalf("expected takeover to evict terminators for service %q, got %#v", tunnel.ZitiServiceId.Value, tunnelLC.evictServiceCalls)
	}
}

// takeover applies only to account-owned standalone tunnels; a session tunnel (environment_id set) is
// rejected before any fabric or DB mutation (finding R2-C1).
func TestTakeoverRejectsSessionTunnel(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	tunnelLC := uniqueEnvironmentIdentitiesHeld(env)

	orgID, accountID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)
	envID := env.enableEnvironment(t, aliceToken)

	// a session tunnel keeps environment_id set; insert one directly to exercise the takeover guard.
	serviceID := "svc-session-1"
	bindPolicyID := "bind-session-1"
	session, err := env.store.Tunnels.Create(env.ctx, env.store.DB(), persistence.Tunnel{
		OrganizationID: orgID,
		AccountID:      accountID,
		EnvironmentID:  &envID,
		Name:           "session-tunnel",
		Mode:           persistence.TunnelModeTCP,
		Kind:           persistence.TunnelKindProxy,
		BackendTarget:  stringPtr("127.0.0.1:8443"),
		ZitiServiceID:  &serviceID,
		BindPolicyID:   &bindPolicyID,
		State:          persistence.TunnelStateActive,
	})
	if err != nil {
		t.Fatalf("insert session tunnel: %v", err)
	}

	takeoverRes, err := alice.TakeoverTunnel(env.ctx, api.TakeoverTunnelParams{TunnelId: session.ID})
	if err != nil {
		t.Fatalf("takeover session tunnel request: %v", err)
	}
	if _, ok := takeoverRes.(*api.TakeoverTunnelConflict); !ok {
		t.Fatalf("expected takeover to reject a session tunnel, got %T", takeoverRes)
	}
	if len(tunnelLC.evictServiceCalls) != 0 {
		t.Fatalf("expected no terminator eviction for a rejected session tunnel, got %#v", tunnelLC.evictServiceCalls)
	}
}

// account deletion is refused while the account still owns a standalone tunnel, and succeeds once the
// tunnel is removed (finding R2-C2).
func TestDeleteAccountRefusedWhileStandaloneTunnelExists(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	uniqueEnvironmentIdentitiesHeld(env)

	orgID, accountID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)
	env.enableEnvironment(t, aliceToken)
	tunnel := createProxyTunnel(t, env, alice, "llm-gateway")

	admin := env.adminClient(t)
	refusedRes, err := admin.DeleteAccount(env.ctx, api.DeleteAccountParams{OrganizationId: orgID, AccountId: accountID})
	if err != nil {
		t.Fatalf("delete account request: %v", err)
	}
	if _, ok := refusedRes.(*api.DeleteAccountConflict); !ok {
		t.Fatalf("expected account delete to be refused while a standalone tunnel exists, got %T", refusedRes)
	}

	if _, err := alice.DeleteTunnel(env.ctx, api.DeleteTunnelParams{TunnelId: tunnel.ID}); err != nil {
		t.Fatalf("delete tunnel: %v", err)
	}

	okRes, err := admin.DeleteAccount(env.ctx, api.DeleteAccountParams{OrganizationId: orgID, AccountId: accountID})
	if err != nil {
		t.Fatalf("delete account after tunnel removal: %v", err)
	}
	if _, ok := okRes.(*api.DeleteAccountNoContent); !ok {
		t.Fatalf("expected account delete to succeed after the standalone tunnel was removed, got %T", okRes)
	}
}

func orgIDOf(t *testing.T, env *workgroupTestEnv, tunnelID string) string {
	t.Helper()
	tunnel, err := env.store.Tunnels.GetByID(env.ctx, env.store.DB(), tunnelID)
	if err != nil {
		t.Fatalf("get tunnel for org id: %v", err)
	}
	return tunnel.OrganizationID
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
