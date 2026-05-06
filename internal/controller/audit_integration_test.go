package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

const auditEventSelectColumns = `id, occurred_at, event_type, organization_id, account_id, workgroup_id, session_id, advertisement_id, contract_id, envelope_id, data`

func TestAuditEventsControllerSessionAttachmentAndCatalogFlow(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	var envSeq int
	tunnelLC := &fakeTunnelLifecycle{}
	env.service.lifecycleFactory = func(context.Context) (environmentLifecycle, tunnelLifecycle, error) {
		envSeq++
		return &fakeEnvironmentLifecycle{
			enableResult: &automation.ProvisionedEnvironment{
				IdentityID:     fmt.Sprintf("ziti-env-audit-%d", envSeq),
				EnrollmentJSON: []byte(`{}`),
				PolicyID:       fmt.Sprintf("erp-audit-%d", envSeq),
			},
		}, tunnelLC, nil
	}

	orgA, aliceID, aliceToken := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	orgB, bobID, bobToken := env.createOrgWithAccount(t, "org-b", "bob@example.com")

	admin := env.adminClient(t)
	createWG, err := admin.CreateWorkgroup(env.ctx, &api.CreateWorkgroupRequest{
		Name:                         "shared",
		Scope:                        api.CreateWorkgroupRequestScopeInterOrg,
		OwnerOrganizationId:          orgA,
		InitialAdminAccountId:        aliceID,
		ParticipatingOrganizationIds: []string{orgB},
	})
	if err != nil {
		t.Fatalf("create inter-org workgroup: %v", err)
	}
	wgID := createWG.(*api.CreateWorkgroupResponse).Workgroup.ID

	if _, err := admin.AcceptWorkgroupInvitation(env.ctx, &api.AcceptWorkgroupInvitationRequest{InitialAdminAccountId: bobID}, api.AcceptWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: orgB}); err != nil {
		t.Fatalf("accept org-b invitation: %v", err)
	}

	alice := env.accountClient(t, aliceToken)
	bob := env.accountClient(t, bobToken)
	aliceEnvID := env.enableEnvironment(t, aliceToken)
	bobEnvID := env.enableEnvironment(t, bobToken)

	if hb, err := alice.HeartbeatEnvironment(env.ctx, api.HeartbeatEnvironmentParams{EnvironmentId: aliceEnvID}); err != nil {
		t.Fatalf("heartbeat environment: %v", err)
	} else if _, ok := hb.(*api.HeartbeatEnvironmentNoContent); !ok {
		t.Fatalf("expected heartbeat 204, got %T", hb)
	}

	pubRes, err := alice.PublishAdvertisement(env.ctx, &api.PublishAdvertisementRequest{
		Name:                "alice-feed",
		Capabilities:        []api.AdvertisementCapability{{Name: "quote"}},
		InteractionPatterns: []api.AdvertisementInteractionPattern{{Kind: api.AdvertisementInteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wgID},
		TunnelMode:          api.NewOptAdvertisementTunnelMode(api.AdvertisementTunnelModeTCP),
	})
	if err != nil {
		t.Fatalf("publish advertisement: %v", err)
	}
	ad := pubRes.(*api.Advertisement)

	assertOnePartyAuditEvent(t, auditEventsByAdvertisement(t, env, persistence.AuditEventAdvertisementPublished, ad.ID), orgA, aliceID)
	heartbeatEvents := auditEventsByType(t, env, persistence.AuditEventEnvironmentHeartbeat)
	assertOnePartyAuditEvent(t, heartbeatEvents, orgA, aliceID)
	if got := heartbeatEvents[0].Data["environment_id"]; got != aliceEnvID {
		t.Fatalf("expected heartbeat environment_id %q, got %#v", aliceEnvID, got)
	}

	propRes, err := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: ad.ID, WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("propose session: %v", err)
	}
	sess := propRes.(*api.Session)
	assertTwoPartyAuditRows(t, auditEventsBySession(t, env, persistence.AuditEventSessionProposed, sess.ID), orgA, aliceID, orgB, bobID)

	acceptRes, err := alice.AcceptSession(env.ctx, &api.AcceptSessionRequest{EnvironmentId: aliceEnvID}, api.AcceptSessionParams{SessionId: sess.ID})
	if err != nil {
		t.Fatalf("accept session: %v", err)
	}
	active := acceptRes.(*api.Session)
	assertTwoPartyAuditRows(t, auditEventsBySession(t, env, persistence.AuditEventSessionAccepted, sess.ID), orgA, aliceID, orgB, bobID)

	connectRes, err := bob.ConnectTunnel(env.ctx, &api.ConnectTunnelRequest{
		Name:          "session-" + sess.ID,
		EnvironmentId: bobEnvID,
		ListenAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("connect tunnel: %v", err)
	}
	connected := connectRes.(*api.ConnectTunnelResponse)
	attached := auditEventsBySession(t, env, persistence.AuditEventTunnelAttached, sess.ID)
	assertOnePartyAuditEvent(t, attached, orgB, bobID)
	if got := attached[0].Data["tunnel_id"]; got != active.TunnelId.Value {
		t.Fatalf("expected attached tunnel_id %q, got %#v", active.TunnelId.Value, got)
	}

	if delAttach, err := bob.DeleteTunnelAttachment(env.ctx, api.DeleteTunnelAttachmentParams{AttachmentId: connected.Attachment.ID}); err != nil {
		t.Fatalf("delete tunnel attachment: %v", err)
	} else if _, ok := delAttach.(*api.DeleteTunnelAttachmentNoContent); !ok {
		t.Fatalf("expected delete attachment 204, got %T", delAttach)
	}
	detached := auditEventsBySession(t, env, persistence.AuditEventTunnelDetached, sess.ID)
	assertOnePartyAuditEvent(t, detached, orgB, bobID)
	if got := detached[0].Data["final_state"]; got != string(persistence.TunnelAttachmentStateDisconnected) {
		t.Fatalf("expected final_state disconnected, got %#v", got)
	}

	if rep, err := alice.ReportSessionEnvelopeCount(env.ctx, &api.ReportEnvelopeCountRequest{Count: 5, ObservedAt: time.Now().UTC()}, api.ReportSessionEnvelopeCountParams{SessionId: sess.ID}); err != nil {
		t.Fatalf("report envelope count 5: %v", err)
	} else if _, ok := rep.(*api.ReportSessionEnvelopeCountNoContent); !ok {
		t.Fatalf("expected report 204, got %T", rep)
	}
	if rep, err := alice.ReportSessionEnvelopeCount(env.ctx, &api.ReportEnvelopeCountRequest{Count: 3, ObservedAt: time.Now().UTC()}, api.ReportSessionEnvelopeCountParams{SessionId: sess.ID}); err != nil {
		t.Fatalf("report envelope count 3: %v", err)
	} else if _, ok := rep.(*api.ReportSessionEnvelopeCountNoContent); !ok {
		t.Fatalf("expected stale report 204, got %T", rep)
	}
	envelopeEvents := auditEventsBySession(t, env, persistence.AuditEventEnvelopeFlowed, sess.ID)
	if len(envelopeEvents) != 4 {
		t.Fatalf("expected 4 envelope.flowed rows, got %d", len(envelopeEvents))
	}
	assertEnvelopeDeltaRows(t, envelopeEvents[:2], 5, 5)
	assertEnvelopeDeltaRows(t, envelopeEvents[2:], 0, 3)

	if closeRes, err := bob.CloseSession(env.ctx, api.OptCloseSessionRequest{}, api.CloseSessionParams{SessionId: sess.ID}); err != nil {
		t.Fatalf("close session: %v", err)
	} else if _, ok := closeRes.(*api.CloseSessionNoContent); !ok {
		t.Fatalf("expected close 204, got %T", closeRes)
	}
	assertTwoPartyAuditRows(t, auditEventsBySession(t, env, persistence.AuditEventSessionClosed, sess.ID), orgA, aliceID, orgB, bobID)

	rejectProp, err := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: ad.ID, WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("propose for reject: %v", err)
	}
	rejectSess := rejectProp.(*api.Session)
	if rejectRes, err := alice.RejectSession(env.ctx, api.NewOptCloseSessionRequest(api.CloseSessionRequest{Reason: api.NewOptString("not taking new")}), api.RejectSessionParams{SessionId: rejectSess.ID}); err != nil {
		t.Fatalf("reject session: %v", err)
	} else if _, ok := rejectRes.(*api.RejectSessionNoContent); !ok {
		t.Fatalf("expected reject 204, got %T", rejectRes)
	}
	rejected := auditEventsBySession(t, env, persistence.AuditEventSessionRejected, rejectSess.ID)
	assertTwoPartyAuditRows(t, rejected, orgA, aliceID, orgB, bobID)
	if got := rejected[0].Data["reason"]; got != "not taking new" {
		t.Fatalf("expected reject reason, got %#v", got)
	}

	if retract, err := alice.RetractAdvertisement(env.ctx, api.RetractAdvertisementParams{AdvertisementId: ad.ID}); err != nil {
		t.Fatalf("retract advertisement: %v", err)
	} else if _, ok := retract.(*api.RetractAdvertisementNoContent); !ok {
		t.Fatalf("expected retract 204, got %T", retract)
	}
	retracted := auditEventsByAdvertisement(t, env, persistence.AuditEventAdvertisementRetracted, ad.ID)
	assertOnePartyAuditEvent(t, retracted, orgA, aliceID)
}

func TestAuditEventFailureRollsBackProposeSession(t *testing.T) {
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
	addAuditRejectConstraint(t, env, "audit_events_no_session_proposed", persistence.AuditEventSessionProposed)
	defer dropAuditRejectConstraint(t, env, "audit_events_no_session_proposed")

	bob := env.accountClient(t, bobToken)
	res, err := bob.ProposeSession(env.ctx, &api.ProposeSessionRequest{AdvertisementId: adID, WorkgroupId: wgID})
	if err != nil {
		t.Fatalf("propose with audit failure: %v", err)
	}
	if _, ok := res.(*api.ProposeSessionInternalServerError); !ok {
		t.Fatalf("expected propose internal error, got %T", res)
	}

	var sessionCount int
	if err := env.store.DB().GetContext(env.ctx, &sessionCount, `select count(*) from sessions where advertisement_id = $1`, adID); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("expected session insert rolled back, got %d session(s)", sessionCount)
	}
}

func TestAuditEventsLoginPathsAndFailOpen(t *testing.T) {
	t.Parallel()
	env, cleanup := newWorkgroupTestEnv(t)
	defer cleanup()

	_, _, token := env.createOrgWithAccount(t, "org-a", "alice@example.com")
	loginClient, err := api.NewClient(env.baseURL, staticSecuritySource{}, api.WithClient(env.ts.Client()))
	if err != nil {
		t.Fatalf("new login client: %v", err)
	}

	success, err := loginClient.Login(env.ctx, &api.LoginRequest{Email: "alice@example.com", Password: "test-password-1"})
	if err != nil {
		t.Fatalf("login success: %v", err)
	}
	if resp, ok := success.(*api.AccountTokenResponse); !ok {
		t.Fatalf("expected token response, got %T", success)
	} else if resp.AccountToken != token {
		t.Fatalf("expected account token to match")
	}
	loginEvents := auditEventsByType(t, env, persistence.AuditEventAccountLogin)
	if len(loginEvents) != 1 {
		t.Fatalf("expected 1 account.login row, got %d", len(loginEvents))
	}

	failed, err := loginClient.Login(env.ctx, &api.LoginRequest{Email: "alice@example.com", Password: "wrong-password"})
	if err != nil {
		t.Fatalf("login failure: %v", err)
	}
	if _, ok := failed.(*api.LoginUnauthorized); !ok {
		t.Fatalf("expected unauthorized, got %T", failed)
	}
	failedEvents := auditEventsByType(t, env, persistence.AuditEventAccountLoginFailed)
	if len(failedEvents) != 1 {
		t.Fatalf("expected 1 account.login_failed row, got %d", len(failedEvents))
	}

	unknown, err := loginClient.Login(env.ctx, &api.LoginRequest{Email: "nobody@example.com", Password: "wrong-password"})
	if err != nil {
		t.Fatalf("unknown login failure: %v", err)
	}
	if _, ok := unknown.(*api.LoginUnauthorized); !ok {
		t.Fatalf("expected unknown-email unauthorized, got %T", unknown)
	}
	if got := len(auditEventsByType(t, env, persistence.AuditEventAccountLoginFailed)); got != 1 {
		t.Fatalf("expected unknown email to skip audit insert, got %d login_failed rows", got)
	}

	addAuditRejectConstraint(t, env, "audit_events_no_account_login", persistence.AuditEventAccountLogin)
	successFailOpen, err := loginClient.Login(env.ctx, &api.LoginRequest{Email: "alice@example.com", Password: "test-password-1"})
	if err != nil {
		t.Fatalf("login success with audit failure: %v", err)
	}
	if _, ok := successFailOpen.(*api.AccountTokenResponse); !ok {
		t.Fatalf("expected fail-open token response, got %T", successFailOpen)
	}
	if got := len(auditEventsByType(t, env, persistence.AuditEventAccountLogin)); got != 1 {
		t.Fatalf("expected failed account.login audit insert to be skipped, got %d rows", got)
	}

	dropAuditRejectConstraint(t, env, "audit_events_no_account_login")
	addAuditRejectConstraint(t, env, "audit_events_no_account_login_failed", persistence.AuditEventAccountLoginFailed)
	defer dropAuditRejectConstraint(t, env, "audit_events_no_account_login_failed")
	failedFailOpen, err := loginClient.Login(env.ctx, &api.LoginRequest{Email: "alice@example.com", Password: "wrong-password"})
	if err != nil {
		t.Fatalf("login failure with audit failure: %v", err)
	}
	if _, ok := failedFailOpen.(*api.LoginUnauthorized); !ok {
		t.Fatalf("expected fail-open unauthorized, got %T", failedFailOpen)
	}
	if got := len(auditEventsByType(t, env, persistence.AuditEventAccountLoginFailed)); got != 1 {
		t.Fatalf("expected failed login_failed audit insert to be skipped, got %d rows", got)
	}
}

func auditEventsByType(t *testing.T, env *workgroupTestEnv, eventType persistence.AuditEventType) []persistence.AuditEvent {
	t.Helper()
	return auditEventsWhere(t, env, `event_type = $1`, eventType)
}

func auditEventsBySession(t *testing.T, env *workgroupTestEnv, eventType persistence.AuditEventType, sessionID string) []persistence.AuditEvent {
	t.Helper()
	return auditEventsWhere(t, env, `event_type = $1 and session_id = $2`, eventType, sessionID)
}

func auditEventsByAdvertisement(t *testing.T, env *workgroupTestEnv, eventType persistence.AuditEventType, advertisementID string) []persistence.AuditEvent {
	t.Helper()
	return auditEventsWhere(t, env, `event_type = $1 and advertisement_id = $2`, eventType, advertisementID)
}

func auditEventsWhere(t *testing.T, env *workgroupTestEnv, where string, args ...any) []persistence.AuditEvent {
	t.Helper()
	query := fmt.Sprintf(`select %s from audit_events where %s order by id`, auditEventSelectColumns, where)
	var events []persistence.AuditEvent
	if err := env.store.DB().SelectContext(env.ctx, &events, query, args...); err != nil {
		t.Fatalf("select audit events: %v", err)
	}
	return events
}

func assertTwoPartyAuditRows(t *testing.T, events []persistence.AuditEvent, providerOrg, providerAccount, consumerOrg, consumerAccount string) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("expected 2 audit rows, got %d", len(events))
	}
	expected := map[string]string{
		providerOrg: providerAccount,
		consumerOrg: consumerAccount,
	}
	for _, event := range events {
		expectedAccount, ok := expected[event.OrganizationID]
		if !ok {
			t.Fatalf("unexpected audit organization_id %q", event.OrganizationID)
		}
		if event.AccountID == nil || *event.AccountID != expectedAccount {
			t.Fatalf("expected account_id %q for org %q, got %#v", expectedAccount, event.OrganizationID, event.AccountID)
		}
		if got := event.Data["provider_organization_id"]; got != providerOrg {
			t.Fatalf("expected provider_organization_id %q, got %#v", providerOrg, got)
		}
		if got := event.Data["consumer_organization_id"]; got != consumerOrg {
			t.Fatalf("expected consumer_organization_id %q, got %#v", consumerOrg, got)
		}
	}
}

func assertOnePartyAuditEvent(t *testing.T, events []persistence.AuditEvent, organizationID, accountID string) {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(events))
	}
	if events[0].OrganizationID != organizationID {
		t.Fatalf("expected organization_id %q, got %q", organizationID, events[0].OrganizationID)
	}
	if events[0].AccountID == nil || *events[0].AccountID != accountID {
		t.Fatalf("expected account_id %q, got %#v", accountID, events[0].AccountID)
	}
}

func assertEnvelopeDeltaRows(t *testing.T, events []persistence.AuditEvent, countDelta, totalCount int) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("expected 2 envelope rows, got %d", len(events))
	}
	for _, event := range events {
		if got := auditDataInt(t, event, "count_delta"); got != countDelta {
			t.Fatalf("expected count_delta %d, got %d", countDelta, got)
		}
		if got := auditDataInt(t, event, "total_count"); got != totalCount {
			t.Fatalf("expected total_count %d, got %d", totalCount, got)
		}
	}
}

func auditDataInt(t *testing.T, event persistence.AuditEvent, key string) int {
	t.Helper()
	switch v := event.Data[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		t.Fatalf("expected numeric audit data key %q, got %#v", key, event.Data[key])
		return 0
	}
}

func addAuditRejectConstraint(t *testing.T, env *workgroupTestEnv, name string, eventType persistence.AuditEventType) {
	t.Helper()
	query := fmt.Sprintf(`alter table audit_events add constraint %s check (event_type <> '%s') not valid`, name, eventType)
	if _, err := env.store.DB().ExecContext(env.ctx, query); err != nil {
		t.Fatalf("add audit reject constraint: %v", err)
	}
}

func dropAuditRejectConstraint(t *testing.T, env *workgroupTestEnv, name string) {
	t.Helper()
	query := fmt.Sprintf(`alter table audit_events drop constraint if exists %s`, name)
	if _, err := env.store.DB().ExecContext(env.ctx, query); err != nil {
		t.Fatalf("drop audit reject constraint: %v", err)
	}
}
