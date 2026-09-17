package controller

import (
	"context"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

type proxyAttachmentTestFixture struct {
	env           *workgroupTestEnv
	client        *api.Client
	lifecycle     *fakeTunnelLifecycle
	environmentID string
	tunnelID      string
	attachmentID  string
}

func newProxyAttachmentTestFixture(t *testing.T) *proxyAttachmentTestFixture {
	t.Helper()

	env, cleanup := newWorkgroupTestEnv(t)
	t.Cleanup(cleanup)

	lifecycle := &fakeTunnelLifecycle{attachmentResult: "dial-policy-original"}
	env.service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		return &fakeEnvironmentLifecycle{
			enableResult: &automation.ProvisionedEnvironment{
				IdentityID:     "ziti-env-reaper",
				EnrollmentJSON: []byte(`{}`),
				PolicyID:       "environment-policy-reaper",
			},
		}, lifecycle, nil
	}

	_, _, accountToken := env.createOrgWithAccount(t, "reaper-org", "reaper@example.com")
	client := env.accountClient(t, accountToken)
	environmentID := env.enableEnvironment(t, accountToken)

	createRes, err := client.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
		EnvironmentId: api.NewOptString(environmentID),
		Name:          "reaper-tunnel",
		Mode:          api.TunnelModeTCP,
		BackendTarget: api.NewOptString("127.0.0.1:8080"),
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}
	tunnel, ok := createRes.(*api.Tunnel)
	if !ok {
		t.Fatalf("unexpected create tunnel response: %T", createRes)
	}

	connectRes, err := client.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{
		EnvironmentId: environmentID,
		Name:          tunnel.Name,
		ListenAddress: api.NewOptString("127.0.0.1:18080"),
	})
	if err != nil {
		t.Fatalf("connect tunnel: %v", err)
	}
	connected, ok := connectRes.(*api.ConnectTunnelResponse)
	if !ok {
		t.Fatalf("unexpected connect tunnel response: %T", connectRes)
	}

	return &proxyAttachmentTestFixture{
		env:           env,
		client:        client,
		lifecycle:     lifecycle,
		environmentID: environmentID,
		tunnelID:      tunnel.ID,
		attachmentID:  connected.Attachment.ID,
	}
}

func TestTunnelAttachmentReaperRetainsDialPolicy(t *testing.T) {
	fixture := newProxyAttachmentTestFixture(t)
	if got := len(auditEventsByType(t, fixture.env, persistence.AuditEventTunnelAttached)); got != 1 {
		t.Fatalf("expected one creation-time tunnel.attached event, got %d", got)
	}
	now := time.Now().UTC()
	expiredAt := now.Add(-tunnelAttachmentLeaseTTL - time.Second)
	if _, err := fixture.env.store.DB().ExecContext(
		fixture.env.ctx,
		`update tunnel_attachments set last_heartbeat_at = $2 where id = $1`,
		fixture.attachmentID,
		expiredAt,
	); err != nil {
		t.Fatalf("expire attachment: %v", err)
	}

	if err := fixture.env.service.ReapStaleTunnelAttachments(fixture.env.ctx, now); err != nil {
		t.Fatalf("reap stale attachment: %v", err)
	}
	if len(fixture.lifecycle.deprovisionCalls) != 0 {
		t.Fatalf("stale reaping must retain the dial policy, got deprovision calls %#v", fixture.lifecycle.deprovisionCalls)
	}

	attachment, err := fixture.env.store.TunnelAttachments.GetByID(fixture.env.ctx, fixture.env.store.DB(), fixture.attachmentID)
	if err != nil {
		t.Fatalf("get reaped attachment: %v", err)
	}
	if attachment.State != persistence.TunnelAttachmentStateStale {
		t.Fatalf("expected stale attachment, got %q", attachment.State)
	}
	if attachment.DisconnectedAt != nil {
		t.Fatalf("lease expiry must not record a disconnect time, got %s", attachment.DisconnectedAt)
	}
	if attachment.DialPolicyID == nil || *attachment.DialPolicyID != "dial-policy-original" {
		t.Fatalf("expected retained dial policy, got %#v", attachment.DialPolicyID)
	}
	if got := len(auditEventsByType(t, fixture.env, persistence.AuditEventTunnelDetached)); got != 0 {
		t.Fatalf("lease expiry must not emit tunnel.detached, got %d events", got)
	}

	heartbeatRes, err := fixture.client.HeartbeatTunnelAttachment(fixture.env.ctx, api.HeartbeatTunnelAttachmentParams{AttachmentId: fixture.attachmentID})
	if err != nil {
		t.Fatalf("heartbeat stale attachment: %v", err)
	}
	if _, ok := heartbeatRes.(*api.HeartbeatTunnelAttachmentNoContent); !ok {
		t.Fatalf("unexpected heartbeat response: %T", heartbeatRes)
	}
	if len(fixture.lifecycle.ensureAttachmentCalls) != 1 {
		t.Fatalf("expected one dial policy reconciliation, got %d", len(fixture.lifecycle.ensureAttachmentCalls))
	}

	restored, err := fixture.env.store.TunnelAttachments.GetByID(fixture.env.ctx, fixture.env.store.DB(), fixture.attachmentID)
	if err != nil {
		t.Fatalf("get restored attachment: %v", err)
	}
	if restored.State != persistence.TunnelAttachmentStateActive {
		t.Fatalf("expected active attachment after heartbeat, got %q", restored.State)
	}
	if restored.DisconnectedAt != nil {
		t.Fatalf("expected disconnected timestamp to remain unset, got %s", restored.DisconnectedAt)
	}
	if restored.DialPolicyID == nil || *restored.DialPolicyID != "dial-policy-original" {
		t.Fatalf("expected existing dial policy after heartbeat, got %#v", restored.DialPolicyID)
	}
	if got := len(auditEventsByType(t, fixture.env, persistence.AuditEventTunnelAttached)); got != 1 {
		t.Fatalf("lease recovery must not emit another tunnel.attached, got %d events", got)
	}
}

func TestManagedProxyReconnectReusesActiveAndStaleAttachment(t *testing.T) {
	fixture := newProxyAttachmentTestFixture(t)
	reconnect := func() *api.ConnectTunnelResponse {
		res, err := fixture.client.ConnectTunnel(fixture.env.ctx, &api.ConnectTunnelRequest{
			EnvironmentId: fixture.environmentID,
			Name:          fixture.tunnelID,
			ListenAddress: api.NewOptString("127.0.0.1:18080"),
			AttachmentId:  api.NewOptString(fixture.attachmentID),
		})
		if err != nil {
			t.Fatalf("reconnect managed proxy: %v", err)
		}
		connected, ok := res.(*api.ConnectTunnelResponse)
		if !ok {
			t.Fatalf("unexpected reconnect response: %T", res)
		}
		return connected
	}

	activeReconnect := reconnect()
	if activeReconnect.Attachment.ID != fixture.attachmentID {
		t.Fatalf("expected active reconnect to reuse attachment %q, got %q", fixture.attachmentID, activeReconnect.Attachment.ID)
	}
	if len(fixture.lifecycle.attachmentCalls) != 1 || len(fixture.lifecycle.ensureAttachmentCalls) != 1 {
		t.Fatalf("active reconnect must ensure the existing policy without creating another, create=%d ensure=%d", len(fixture.lifecycle.attachmentCalls), len(fixture.lifecycle.ensureAttachmentCalls))
	}

	now := time.Now().UTC()
	if _, err := fixture.env.store.DB().ExecContext(
		fixture.env.ctx,
		`update tunnel_attachments set last_heartbeat_at = $2 where id = $1`,
		fixture.attachmentID,
		now.Add(-tunnelAttachmentLeaseTTL-time.Second),
	); err != nil {
		t.Fatalf("expire attachment: %v", err)
	}
	if err := fixture.env.service.ReapStaleTunnelAttachments(fixture.env.ctx, now); err != nil {
		t.Fatalf("reap stale attachment: %v", err)
	}

	staleReconnect := reconnect()
	if staleReconnect.Attachment.ID != fixture.attachmentID {
		t.Fatalf("expected stale reconnect to reuse attachment %q, got %q", fixture.attachmentID, staleReconnect.Attachment.ID)
	}
	if len(fixture.lifecycle.attachmentCalls) != 1 || len(fixture.lifecycle.ensureAttachmentCalls) != 2 {
		t.Fatalf("stale reconnect must ensure the existing policy without creating another, create=%d ensure=%d", len(fixture.lifecycle.attachmentCalls), len(fixture.lifecycle.ensureAttachmentCalls))
	}

	var attachmentCount int
	if err := fixture.env.store.DB().GetContext(
		fixture.env.ctx,
		&attachmentCount,
		`select count(*) from tunnel_attachments where environment_id = $1 and tunnel_id = $2 and kind = 'proxy' and listen_address = $3 and not deleted`,
		fixture.environmentID,
		fixture.tunnelID,
		"127.0.0.1:18080",
	); err != nil {
		t.Fatalf("count matching proxy attachments: %v", err)
	}
	if attachmentCount != 1 {
		t.Fatalf("expected one durable proxy attachment after reconnects, got %d", attachmentCount)
	}
	if got := len(auditEventsByType(t, fixture.env, persistence.AuditEventTunnelAttached)); got != 1 {
		t.Fatalf("proxy reconnect must not emit another tunnel.attached, got %d events", got)
	}
	if got := len(auditEventsByType(t, fixture.env, persistence.AuditEventTunnelDetached)); got != 0 {
		t.Fatalf("proxy reconnect lease transitions must not emit tunnel.detached, got %d events", got)
	}
}

func TestManagedProxyConnectWithoutOwnershipProofCreatesDistinctAttachment(t *testing.T) {
	fixture := newProxyAttachmentTestFixture(t)
	fixture.lifecycle.attachmentResult = "dial-policy-second-runtime"

	res, err := fixture.client.ConnectTunnel(fixture.env.ctx, &api.ConnectTunnelRequest{
		EnvironmentId: fixture.environmentID,
		Name:          fixture.tunnelID,
		ListenAddress: api.NewOptString("127.0.0.1:18080"),
	})
	if err != nil {
		t.Fatalf("connect second managed proxy: %v", err)
	}
	connected, ok := res.(*api.ConnectTunnelResponse)
	if !ok {
		t.Fatalf("unexpected connect response: %T", res)
	}
	if connected.Attachment.ID == fixture.attachmentID {
		t.Fatalf("connect without prior attachment ID must not adopt attachment %q", fixture.attachmentID)
	}
	if len(fixture.lifecycle.attachmentCalls) != 2 || len(fixture.lifecycle.ensureAttachmentCalls) != 0 {
		t.Fatalf("connect without ownership proof must create a distinct attachment, create=%d ensure=%d", len(fixture.lifecycle.attachmentCalls), len(fixture.lifecycle.ensureAttachmentCalls))
	}
	if got := len(auditEventsByType(t, fixture.env, persistence.AuditEventTunnelAttached)); got != 2 {
		t.Fatalf("distinct proxy attachment must emit its own tunnel.attached event, got %d events", got)
	}
}

func TestTunnelAttachmentHeartbeatRepairsMissingDialPolicy(t *testing.T) {
	fixture := newProxyAttachmentTestFixture(t)
	disconnectedAt := time.Now().UTC().Add(-time.Minute)
	if _, err := fixture.env.store.DB().ExecContext(
		fixture.env.ctx,
		`update tunnel_attachments set state = 'active', dial_policy_id = 'dial-policy-missing', disconnected_at = $2 where id = $1`,
		fixture.attachmentID,
		disconnectedAt,
	); err != nil {
		t.Fatalf("create active attachment with missing policy: %v", err)
	}
	fixture.lifecycle.ensureAttachmentResult = "dial-policy-repaired"
	fixture.lifecycle.ensureAttachmentCreated = true

	heartbeatRes, err := fixture.client.HeartbeatTunnelAttachment(fixture.env.ctx, api.HeartbeatTunnelAttachmentParams{AttachmentId: fixture.attachmentID})
	if err != nil {
		t.Fatalf("heartbeat attachment with missing policy: %v", err)
	}
	if _, ok := heartbeatRes.(*api.HeartbeatTunnelAttachmentNoContent); !ok {
		t.Fatalf("unexpected heartbeat response: %T", heartbeatRes)
	}
	if len(fixture.lifecycle.ensureAttachmentCalls) != 1 {
		t.Fatalf("expected one dial policy reconciliation, got %d", len(fixture.lifecycle.ensureAttachmentCalls))
	}
	ensureSpec := fixture.lifecycle.ensureAttachmentCalls[0]
	if ensureSpec.AttachmentID != fixture.attachmentID || ensureSpec.EnvironmentID != fixture.environmentID || ensureSpec.TunnelID != fixture.tunnelID {
		t.Fatalf("unexpected dial policy reconciliation spec: %#v", ensureSpec)
	}

	restored, err := fixture.env.store.TunnelAttachments.GetByID(fixture.env.ctx, fixture.env.store.DB(), fixture.attachmentID)
	if err != nil {
		t.Fatalf("get repaired attachment: %v", err)
	}
	if restored.State != persistence.TunnelAttachmentStateActive || restored.DisconnectedAt != nil {
		t.Fatalf("expected repaired active attachment, got state=%q disconnected_at=%v", restored.State, restored.DisconnectedAt)
	}
	if restored.DialPolicyID == nil || *restored.DialPolicyID != "dial-policy-repaired" {
		t.Fatalf("expected repaired dial policy, got %#v", restored.DialPolicyID)
	}
}

func TestCrossOrganizationTunnelAttachmentHeartbeatRecoversStale(t *testing.T) {
	env, cleanup := newWorkgroupTestEnv(t)
	t.Cleanup(cleanup)

	lifecycle := uniqueEnvironmentIdentitiesHeld(env)

	_, _, providerToken := env.createOrgWithAccount(t, "reaper-provider-org", "reaper-provider@example.com")
	consumerOrganizationID, consumerAccountID, consumerToken := env.createOrgWithAccount(t, "reaper-consumer-org", "reaper-consumer@example.com")
	provider := env.accountClient(t, providerToken)
	consumer := env.accountClient(t, consumerToken)
	providerEnvironmentID := env.enableEnvironment(t, providerToken)
	consumerEnvironmentID := env.enableEnvironment(t, consumerToken)

	createRes, err := provider.CreateTunnel(env.ctx, &api.CreateTunnelRequest{
		EnvironmentId: api.NewOptString(providerEnvironmentID),
		Name:          "cross-org-reaper-tunnel",
		Mode:          api.TunnelModeTCP,
		BackendTarget: api.NewOptString("127.0.0.1:8080"),
	})
	if err != nil {
		t.Fatalf("create provider tunnel: %v", err)
	}
	tunnel, ok := createRes.(*api.Tunnel)
	if !ok {
		t.Fatalf("unexpected create tunnel response: %T", createRes)
	}
	if _, err := env.store.TunnelGrants.Create(env.ctx, env.store.DB(), persistence.TunnelAccountGrant{
		TunnelID:       tunnel.ID,
		AccountID:      consumerAccountID,
		OrganizationID: consumerOrganizationID,
	}); err != nil {
		t.Fatalf("grant cross-organization tunnel access: %v", err)
	}

	connectRes, err := consumer.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{
		EnvironmentId: consumerEnvironmentID,
		Name:          tunnel.ID,
		ListenAddress: api.NewOptString("127.0.0.1:18081"),
	})
	if err != nil {
		t.Fatalf("connect cross-organization tunnel: %v", err)
	}
	connected, ok := connectRes.(*api.ConnectTunnelResponse)
	if !ok {
		t.Fatalf("unexpected connect tunnel response: %T", connectRes)
	}

	now := time.Now().UTC()
	if _, err := env.store.DB().ExecContext(
		env.ctx,
		`update tunnel_attachments set last_heartbeat_at = $2 where id = $1`,
		connected.Attachment.ID,
		now.Add(-tunnelAttachmentLeaseTTL-time.Second),
	); err != nil {
		t.Fatalf("expire cross-organization attachment: %v", err)
	}
	if err := env.service.ReapStaleTunnelAttachments(env.ctx, now); err != nil {
		t.Fatalf("reap cross-organization attachment: %v", err)
	}

	heartbeatRes, err := consumer.HeartbeatTunnelAttachment(env.ctx, api.HeartbeatTunnelAttachmentParams{AttachmentId: connected.Attachment.ID})
	if err != nil {
		t.Fatalf("heartbeat cross-organization attachment: %v", err)
	}
	if _, ok := heartbeatRes.(*api.HeartbeatTunnelAttachmentNoContent); !ok {
		t.Fatalf("unexpected heartbeat response: %T", heartbeatRes)
	}
	if len(lifecycle.ensureAttachmentCalls) != 1 {
		t.Fatalf("expected one cross-organization dial policy reconciliation, got %d", len(lifecycle.ensureAttachmentCalls))
	}
	ensureSpec := lifecycle.ensureAttachmentCalls[0]
	if ensureSpec.EnvironmentID != consumerEnvironmentID || ensureSpec.TunnelID != tunnel.ID {
		t.Fatalf("unexpected cross-organization reconciliation spec: %#v", ensureSpec)
	}

	restored, err := env.store.TunnelAttachments.GetByID(env.ctx, env.store.DB(), connected.Attachment.ID)
	if err != nil {
		t.Fatalf("get restored cross-organization attachment: %v", err)
	}
	if restored.State != persistence.TunnelAttachmentStateActive || restored.DisconnectedAt != nil {
		t.Fatalf("expected restored cross-organization attachment, got state=%q disconnected_at=%v", restored.State, restored.DisconnectedAt)
	}
}

func TestTunnelAttachmentReaperDoesNotApplyStaleSnapshot(t *testing.T) {
	fixture := newProxyAttachmentTestFixture(t)
	now := time.Now().UTC()
	expiredBefore := now.Add(-tunnelAttachmentLeaseTTL)
	if _, err := fixture.env.store.DB().ExecContext(
		fixture.env.ctx,
		`update tunnel_attachments set last_heartbeat_at = $2 where id = $1`,
		fixture.attachmentID,
		expiredBefore.Add(-time.Second),
	); err != nil {
		t.Fatalf("expire attachment: %v", err)
	}

	expired, err := fixture.env.store.TunnelAttachments.ListExpiredActive(fixture.env.ctx, fixture.env.store.DB(), expiredBefore)
	if err != nil {
		t.Fatalf("list expired attachments: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != fixture.attachmentID {
		t.Fatalf("expected expired attachment snapshot, got %#v", expired)
	}
	if err := fixture.env.store.TunnelAttachments.Heartbeat(
		fixture.env.ctx,
		fixture.env.store.DB(),
		fixture.attachmentID,
		expired[0].OrganizationID,
		expired[0].AccountID,
		now,
	); err != nil {
		t.Fatalf("refresh attachment after snapshot: %v", err)
	}

	reaped, err := fixture.env.service.reapStaleTunnelAttachment(fixture.env.ctx, expired[0], expiredBefore)
	if err != nil {
		t.Fatalf("reap refreshed attachment from stale snapshot: %v", err)
	}
	if reaped {
		t.Fatal("expected refreshed attachment not to be reaped")
	}
	current, err := fixture.env.store.TunnelAttachments.GetByID(fixture.env.ctx, fixture.env.store.DB(), fixture.attachmentID)
	if err != nil {
		t.Fatalf("get refreshed attachment: %v", err)
	}
	if current.State != persistence.TunnelAttachmentStateActive || current.DisconnectedAt != nil {
		t.Fatalf("expected refreshed attachment to remain active, got state=%q disconnected_at=%v", current.State, current.DisconnectedAt)
	}
}

func TestTunnelAttachmentReconciliationCompensatesOnlyUnreferencedPolicy(t *testing.T) {
	fixture := newProxyAttachmentTestFixture(t)
	attachment, err := fixture.env.store.TunnelAttachments.GetByID(fixture.env.ctx, fixture.env.store.DB(), fixture.attachmentID)
	if err != nil {
		t.Fatalf("get attachment: %v", err)
	}

	committedPolicyID := "dial-policy-adopted"
	if err := fixture.env.store.TunnelAttachments.HeartbeatWithDialPolicy(
		fixture.env.ctx,
		fixture.env.store.DB(),
		attachment.ID,
		attachment.OrganizationID,
		attachment.AccountID,
		committedPolicyID,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf("adopt replacement policy: %v", err)
	}
	if err := fixture.env.service.compensateCreatedAttachmentDialPolicy(fixture.env.ctx, fixture.lifecycle, attachment, committedPolicyID); err != nil {
		t.Fatalf("compensate adopted policy: %v", err)
	}
	if len(fixture.lifecycle.deprovisionCalls) != 0 {
		t.Fatalf("compensation must preserve a policy adopted by another heartbeat, got %#v", fixture.lifecycle.deprovisionCalls)
	}

	orphanPolicyID := "dial-policy-orphaned"
	canceledCtx, cancel := context.WithCancel(fixture.env.ctx)
	cancel()
	if err := fixture.env.service.compensateCreatedAttachmentDialPolicy(canceledCtx, fixture.lifecycle, attachment, orphanPolicyID); err != nil {
		t.Fatalf("compensate orphaned policy: %v", err)
	}
	if len(fixture.lifecycle.deprovisionCalls) != 1 || fixture.lifecycle.deprovisionCalls[0].DialPolicyID != orphanPolicyID {
		t.Fatalf("expected only the unreferenced policy to be deprovisioned, got %#v", fixture.lifecycle.deprovisionCalls)
	}
}

func TestExplicitDeleteOfStaleTunnelAttachmentEmitsDetachedAudit(t *testing.T) {
	fixture := newProxyAttachmentTestFixture(t)
	now := time.Now().UTC()
	if _, err := fixture.env.store.DB().ExecContext(
		fixture.env.ctx,
		`update tunnel_attachments set last_heartbeat_at = $2 where id = $1`,
		fixture.attachmentID,
		now.Add(-tunnelAttachmentLeaseTTL-time.Second),
	); err != nil {
		t.Fatalf("expire attachment: %v", err)
	}
	if err := fixture.env.service.ReapStaleTunnelAttachments(fixture.env.ctx, now); err != nil {
		t.Fatalf("reap stale attachment: %v", err)
	}
	if got := len(auditEventsByType(t, fixture.env, persistence.AuditEventTunnelDetached)); got != 0 {
		t.Fatalf("lease expiry must not emit tunnel.detached, got %d events", got)
	}

	deleteRes, err := fixture.client.DeleteTunnelAttachment(fixture.env.ctx, api.DeleteTunnelAttachmentParams{AttachmentId: fixture.attachmentID})
	if err != nil {
		t.Fatalf("delete stale attachment: %v", err)
	}
	if _, ok := deleteRes.(*api.DeleteTunnelAttachmentNoContent); !ok {
		t.Fatalf("unexpected delete response: %T", deleteRes)
	}
	detached := auditEventsByType(t, fixture.env, persistence.AuditEventTunnelDetached)
	if len(detached) != 1 {
		t.Fatalf("explicit deletion of stale attachment must emit one tunnel.detached event, got %d", len(detached))
	}
	if got := detached[0].Data["final_state"]; got != string(persistence.TunnelAttachmentStateDisconnected) {
		t.Fatalf("expected final_state disconnected, got %#v", got)
	}
	if len(fixture.lifecycle.deprovisionCalls) != 1 {
		t.Fatalf("expected explicit deletion to deprovision retained policy once, got %#v", fixture.lifecycle.deprovisionCalls)
	}
}

func TestDisconnectedTunnelAttachmentHeartbeatDoesNotReactivate(t *testing.T) {
	fixture := newProxyAttachmentTestFixture(t)

	deleteRes, err := fixture.client.DeleteTunnelAttachment(fixture.env.ctx, api.DeleteTunnelAttachmentParams{AttachmentId: fixture.attachmentID})
	if err != nil {
		t.Fatalf("delete attachment: %v", err)
	}
	if _, ok := deleteRes.(*api.DeleteTunnelAttachmentNoContent); !ok {
		t.Fatalf("unexpected delete response: %T", deleteRes)
	}

	heartbeatRes, err := fixture.client.HeartbeatTunnelAttachment(fixture.env.ctx, api.HeartbeatTunnelAttachmentParams{AttachmentId: fixture.attachmentID})
	if err != nil {
		t.Fatalf("heartbeat disconnected attachment: %v", err)
	}
	if _, ok := heartbeatRes.(*api.HeartbeatTunnelAttachmentNotFound); !ok {
		t.Fatalf("expected disconnected attachment heartbeat to return not found, got %T", heartbeatRes)
	}
	if len(fixture.lifecycle.ensureAttachmentCalls) != 0 {
		t.Fatalf("disconnected attachment must not recreate dial policy, got %d ensure calls", len(fixture.lifecycle.ensureAttachmentCalls))
	}

	disconnected, err := fixture.env.store.TunnelAttachments.GetByID(fixture.env.ctx, fixture.env.store.DB(), fixture.attachmentID)
	if err != nil {
		t.Fatalf("get disconnected attachment: %v", err)
	}
	if disconnected.State != persistence.TunnelAttachmentStateDisconnected {
		t.Fatalf("expected disconnected attachment to remain disconnected, got %q", disconnected.State)
	}
}
