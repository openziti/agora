package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
)

// uniqueEnvironmentIdentities overrides the lifecycle factory so each
// EnableEnvironment call provisions a distinct ziti identity id. The default
// test factory returns a fixed id, which collides on the unique constraint
// when an account enrolls more than one environment.
func uniqueEnvironmentIdentities(env *workgroupTestEnv) {
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
}

// uniqueDialTunnelLifecycle derives the dial policy id from the (unique)
// attachment id so multiple attachments do not collide on the
// dial_policy_id unique constraint. The shared fake returns a constant id.
type uniqueDialTunnelLifecycle struct {
	fakeTunnelLifecycle
}

func (f *uniqueDialTunnelLifecycle) CreateAttachmentDialPolicy(_ context.Context, spec automation.TunnelAccessSpec) (string, error) {
	f.attachmentCalls = append(f.attachmentCalls, spec)
	return spec.AttachmentID + "-dial", nil
}

// connect is account-scoped: any environment enrolled to the owning account
// may connect to the account's own tunnel, even when a different environment
// served it. serve remains environment-bound.
func TestConnectTunnelCrossEnvironmentSameAccount(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	uniqueEnvironmentIdentities(env)

	_, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	alice := env.accountClient(t, aliceToken)

	serveEnvID := env.enableEnvironment(t, aliceToken)
	connectEnvID := env.enableEnvironment(t, aliceToken)
	if serveEnvID == connectEnvID {
		t.Fatalf("expected two distinct environments, got %q twice", serveEnvID)
	}

	createRes, err := alice.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
		EnvironmentId: serveEnvID,
		Name:          "llm-gateway",
		Mode:          api.TunnelModeTCP,
		BackendTarget: "127.0.0.1:8443",
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}
	tunnel, ok := createRes.(*api.Tunnel)
	if !ok {
		t.Fatalf("unexpected create tunnel response: %T", createRes)
	}

	// connect from a different environment of the same account.
	crossRes, err := alice.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{
		Name:          tunnel.Name,
		EnvironmentId: connectEnvID,
		ListenAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("connect tunnel from second environment: %v", err)
	}
	crossConnected, ok := crossRes.(*api.ConnectTunnelResponse)
	if !ok {
		t.Fatalf("expected cross-environment connect to succeed, got %T", crossRes)
	}
	if crossConnected.Attachment.EnvironmentId != connectEnvID {
		t.Fatalf("expected attachment bound to connect environment %q, got %q", connectEnvID, crossConnected.Attachment.EnvironmentId)
	}

	// regression guard: connect from the serving environment still works.
	sameRes, err := alice.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{
		Name:          tunnel.Name,
		EnvironmentId: serveEnvID,
		ListenAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("connect tunnel from serving environment: %v", err)
	}
	if sameConnected, ok := sameRes.(*api.ConnectTunnelResponse); !ok {
		t.Fatalf("expected same-environment connect to succeed, got %T", sameRes)
	} else if sameConnected.Attachment.EnvironmentId != serveEnvID {
		t.Fatalf("expected attachment bound to serving environment %q, got %q", serveEnvID, sameConnected.Attachment.EnvironmentId)
	}
}

// account boundary is preserved: a different account in the same organization
// cannot connect without an explicit grant, and can once granted.
func TestConnectTunnelCrossAccountRequiresGrant(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()
	uniqueEnvironmentIdentities(env)

	orgA, _, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	_, bobToken := env.addAccountToOrg(t, orgA, "bob@example.com")

	alice := env.accountClient(t, aliceToken)
	bob := env.accountClient(t, bobToken)

	aliceEnvID := env.enableEnvironment(t, aliceToken)
	bobEnvID := env.enableEnvironment(t, bobToken)

	createRes, err := alice.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
		EnvironmentId: aliceEnvID,
		Name:          "private-gateway",
		Mode:          api.TunnelModeTCP,
		BackendTarget: "127.0.0.1:8443",
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}
	tunnel := createRes.(*api.Tunnel)

	// without a grant, bob cannot see or connect to alice's tunnel.
	denied, err := bob.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{
		Name:          tunnel.Name,
		EnvironmentId: bobEnvID,
		ListenAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("connect tunnel (ungranted): %v", err)
	}
	if _, ok := denied.(*api.ConnectTunnelNotFound); !ok {
		t.Fatalf("expected ungranted cross-account connect to be rejected, got %T", denied)
	}

	// grant bob access, then the same connect succeeds.
	if _, err := alice.AddTunnelGrant(env.ctx, &api.AddTunnelGrantRequest{Email: "bob@example.com"}, api.AddTunnelGrantParams{TunnelId: tunnel.ID}); err != nil {
		t.Fatalf("add tunnel grant: %v", err)
	}
	granted, err := bob.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{
		Name:          tunnel.Name,
		EnvironmentId: bobEnvID,
		ListenAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("connect tunnel (granted): %v", err)
	}
	if _, ok := granted.(*api.ConnectTunnelResponse); !ok {
		t.Fatalf("expected granted cross-account connect to succeed, got %T", granted)
	}
}
