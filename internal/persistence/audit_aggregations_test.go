package persistence

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAuditAggregationEnvelopeBuckets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	now := time.Date(2026, 5, 6, 15, 37, 0, 0, time.UTC)

	orgA, acctA := createAggregationOrgAccount(t, ctx, store, "org-a")
	orgB, _ := createAggregationOrgAccount(t, ctx, store, "org-b")
	wgA := createAggregationWorkgroup(t, ctx, store, orgA, acctA, "alpha")

	recordAggregationEnvelope(t, ctx, store, orgA.ID, wgA.ID, now.Add(-90*time.Minute), 7)
	recordAggregationSessionProposed(t, ctx, store, orgA.ID, wgA.ID, now.Add(-80*time.Minute))
	recordAggregationEnvelope(t, ctx, store, orgA.ID, wgA.ID, now.Add(-24*time.Hour+10*time.Minute), 100)
	recordAggregationEnvelope(t, ctx, store, orgB.ID, wgA.ID, now.Add(-90*time.Minute), 99)

	buckets, err := EnvelopeBucketsAt(ctx, store.DB(), orgA.ID, now, 24*time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("envelope buckets: %v", err)
	}
	if len(buckets) != 24 {
		t.Fatalf("expected 24 hourly buckets, got %d", len(buckets))
	}
	if !buckets[0].Start.Equal(time.Date(2026, 5, 5, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected first bucket: %s", buckets[0].Start)
	}
	if !buckets[len(buckets)-1].Start.Equal(time.Date(2026, 5, 6, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected last bucket: %s", buckets[len(buckets)-1].Start)
	}
	if got := sumBucketEnvelopes(buckets); got != 7 {
		t.Fatalf("expected org-a envelope sum 7, got %d", got)
	}
	if got := sumBucketSessions(buckets); got != 1 {
		t.Fatalf("expected org-a session sum 1, got %d", got)
	}

	target := bucketByStart(t, buckets, time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC))
	if target.Envelopes != 7 || target.Sessions != 1 {
		t.Fatalf("expected 14:00 bucket envelopes=7 sessions=1, got envelopes=%d sessions=%d", target.Envelopes, target.Sessions)
	}

	otherBuckets, err := EnvelopeBucketsAt(ctx, store.DB(), orgB.ID, now, 24*time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("other org envelope buckets: %v", err)
	}
	if got := sumBucketEnvelopes(otherBuckets); got != 99 {
		t.Fatalf("expected org-b envelope sum 99, got %d", got)
	}

	orgEmpty, _ := createAggregationOrgAccount(t, ctx, store, "org-empty")
	emptyBuckets, err := EnvelopeBucketsAt(ctx, store.DB(), orgEmpty.ID, now, 24*time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("empty-ish envelope buckets: %v", err)
	}
	if len(emptyBuckets) != 24 {
		t.Fatalf("expected empty window shape to retain 24 buckets, got %d", len(emptyBuckets))
	}
	if got := sumBucketEnvelopes(emptyBuckets); got != 0 {
		t.Fatalf("expected empty envelope sum 0, got %d", got)
	}
}

func TestAuditAggregationEnvelopeBucketsDailyWindow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	now := time.Date(2026, 5, 6, 15, 37, 0, 0, time.UTC)

	org, acct := createAggregationOrgAccount(t, ctx, store, "org-daily")
	wg := createAggregationWorkgroup(t, ctx, store, org, acct, "daily")
	recordAggregationEnvelope(t, ctx, store, org.ID, wg.ID, now.Add(-48*time.Hour), 11)
	recordAggregationSessionProposed(t, ctx, store, org.ID, wg.ID, now.Add(-72*time.Hour))

	buckets, err := EnvelopeBucketsAt(ctx, store.DB(), org.ID, now, 7*24*time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("daily envelope buckets: %v", err)
	}
	if len(buckets) != 7 {
		t.Fatalf("expected 7 daily buckets, got %d", len(buckets))
	}
	if !buckets[0].Start.Equal(time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected first daily bucket: %s", buckets[0].Start)
	}
	if got := sumBucketEnvelopes(buckets); got != 11 {
		t.Fatalf("expected daily envelope sum 11, got %d", got)
	}
	if got := sumBucketSessions(buckets); got != 1 {
		t.Fatalf("expected daily session sum 1, got %d", got)
	}
}

func TestAuditAggregationTwoPartyBucketRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	now := time.Date(2026, 5, 6, 15, 37, 0, 0, time.UTC)

	orgA, acctA := createAggregationOrgAccount(t, ctx, store, "org-provider")
	orgB, _ := createAggregationOrgAccount(t, ctx, store, "org-consumer")
	orgC, acctC := createAggregationOrgAccount(t, ctx, store, "org-intra")
	wgAB := createAggregationWorkgroup(t, ctx, store, orgA, acctA, "ab")
	wgC := createAggregationWorkgroup(t, ctx, store, orgC, acctC, "intra")

	recordAggregationEnvelope(t, ctx, store, orgA.ID, wgAB.ID, now.Add(-time.Hour), 13)
	recordAggregationEnvelope(t, ctx, store, orgB.ID, wgAB.ID, now.Add(-time.Hour), 13)
	recordAggregationSessionProposed(t, ctx, store, orgA.ID, wgAB.ID, now.Add(-time.Hour))
	recordAggregationSessionProposed(t, ctx, store, orgB.ID, wgAB.ID, now.Add(-time.Hour))

	recordAggregationEnvelope(t, ctx, store, orgC.ID, wgC.ID, now.Add(-time.Hour), 17)
	recordAggregationSessionProposed(t, ctx, store, orgC.ID, wgC.ID, now.Add(-time.Hour))

	for _, tc := range []struct {
		orgID             string
		expectedEnvelopes int
		expectedSessions  int
	}{
		{orgID: orgA.ID, expectedEnvelopes: 13, expectedSessions: 1},
		{orgID: orgB.ID, expectedEnvelopes: 13, expectedSessions: 1},
		{orgID: orgC.ID, expectedEnvelopes: 17, expectedSessions: 1},
	} {
		buckets, err := EnvelopeBucketsAt(ctx, store.DB(), tc.orgID, now, 24*time.Hour, time.Hour)
		if err != nil {
			t.Fatalf("envelope buckets for %s: %v", tc.orgID, err)
		}
		if got := sumBucketEnvelopes(buckets); got != tc.expectedEnvelopes {
			t.Fatalf("expected envelopes %d for %s, got %d", tc.expectedEnvelopes, tc.orgID, got)
		}
		if got := sumBucketSessions(buckets); got != tc.expectedSessions {
			t.Fatalf("expected sessions %d for %s, got %d", tc.expectedSessions, tc.orgID, got)
		}
	}
}

func TestAuditAggregationDashboardCountsFixedClock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	now := time.Date(2026, 5, 6, 15, 37, 0, 0, time.UTC)

	org, acct := createAggregationOrgAccount(t, ctx, store, "org-counts")
	otherOrg, otherAcct := createAggregationOrgAccount(t, ctx, store, "org-other")
	wgA := createAggregationWorkgroup(t, ctx, store, org, acct, "alpha")
	wgB := createAggregationWorkgroup(t, ctx, store, org, acct, "beta")
	env := createAggregationEnvironment(t, ctx, store, org, acct, "env-active", nil, EnvironmentStateEnabled)

	ad1 := createAggregationAdvertisement(t, ctx, store, org, acct, wgA, "ad-one")
	_ = createAggregationAdvertisement(t, ctx, store, org, acct, wgB, "ad-two")
	otherAd := createAggregationAdvertisement(t, ctx, store, otherOrg, otherAcct, wgA, "other-ad")

	createAggregationSession(t, ctx, store, org, org, acct, acct, wgA, ad1, SessionStateActive, now.Add(-10*24*time.Hour), nil)
	createAggregationSession(t, ctx, store, org, org, acct, acct, wgA, ad1, SessionStateClosed, now.Add(-10*24*time.Hour), timePtr(now.Add(-5*24*time.Hour)))
	createAggregationSession(t, ctx, store, org, org, acct, acct, wgA, ad1, SessionStateProposed, now.Add(-3*24*time.Hour), nil)
	createAggregationSession(t, ctx, store, org, org, acct, acct, wgA, ad1, SessionStateClosed, now.Add(-3*24*time.Hour), timePtr(now.Add(-24*time.Hour)))
	createAggregationSession(t, ctx, store, otherOrg, otherOrg, otherAcct, otherAcct, wgA, otherAd, SessionStateActive, now.Add(-24*time.Hour), nil)

	recordAggregationEnvelope(t, ctx, store, org.ID, wgA.ID, now.Add(-12*time.Hour), 3)
	recordAggregationEnvelope(t, ctx, store, org.ID, wgA.ID, now.Add(-36*time.Hour), 5)
	recordAggregationEnvelope(t, ctx, store, otherOrg.ID, wgA.ID, now.Add(-12*time.Hour), 99)
	recordAggregationSessionProposed(t, ctx, store, org.ID, wgA.ID, now.Add(-2*time.Hour))
	recordAggregationSessionProposed(t, ctx, store, org.ID, wgA.ID, now.Add(-3*time.Hour))
	recordAggregationSessionProposed(t, ctx, store, otherOrg.ID, wgA.ID, now.Add(-2*time.Hour))

	tunnel := createAggregationTunnel(t, ctx, store, org, acct, env)
	createAggregationAttachment(t, ctx, store, org, acct, env, tunnel, TunnelAttachmentStateActive)
	createAggregationServe(t, ctx, store, org, acct, env, tunnel, TunnelServeStateActive)

	disabledEnvName := "env-disabled"
	if _, err := store.Environments.Create(ctx, store.DB(), Environment{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Host:           &disabledEnvName,
		ZitiIdentityID: "ziti-" + NewResourceID(PrefixEnvironment),
		State:          EnvironmentStateDisabled,
	}); err != nil {
		t.Fatalf("create disabled environment: %v", err)
	}

	counts, err := DashboardCountsAt(ctx, store.DB(), org.ID, acct.ID, now)
	if err != nil {
		t.Fatalf("dashboard counts: %v", err)
	}
	if counts.Stats.ActiveSessions != 2 {
		t.Fatalf("expected activeSessions=2, got %d", counts.Stats.ActiveSessions)
	}
	if counts.Stats.ActiveSessionsDelta7d != 0 {
		t.Fatalf("expected activeSessionsDelta7d=0, got %d", counts.Stats.ActiveSessionsDelta7d)
	}
	if counts.Stats.EnvelopesToday != 3 {
		t.Fatalf("expected envelopesToday=3, got %d", counts.Stats.EnvelopesToday)
	}
	if counts.Stats.EnvelopesYesterday != 5 {
		t.Fatalf("expected envelopesYesterday=5, got %d", counts.Stats.EnvelopesYesterday)
	}
	if counts.Stats.ActiveWorkgroups != 2 || counts.Ribbon.WorkgroupCount != 2 {
		t.Fatalf("expected workgroup counts 2/2, got stats=%d ribbon=%d", counts.Stats.ActiveWorkgroups, counts.Ribbon.WorkgroupCount)
	}
	if counts.Stats.ActiveTunnels != 2 {
		t.Fatalf("expected activeTunnels=2, got %d", counts.Stats.ActiveTunnels)
	}
	if counts.Ribbon.AdvertisementCount != 2 {
		t.Fatalf("expected advertisementCount=2, got %d", counts.Ribbon.AdvertisementCount)
	}
	if counts.Ribbon.SessionsToday != 2 {
		t.Fatalf("expected sessionsToday=2, got %d", counts.Ribbon.SessionsToday)
	}
	if counts.Ribbon.EnvironmentCount != 1 {
		t.Fatalf("expected environmentCount=1, got %d", counts.Ribbon.EnvironmentCount)
	}
}

func TestAuditAggregationActiveSessionsOrgPredicate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	now := time.Date(2026, 5, 6, 15, 37, 0, 0, time.UTC)

	org, acct := createAggregationOrgAccount(t, ctx, store, "org-main")
	orgConsumer, consumerAcct := createAggregationOrgAccount(t, ctx, store, "org-consumer")
	orgProvider, providerAcct := createAggregationOrgAccount(t, ctx, store, "org-provider")
	orgOther, otherAcct := createAggregationOrgAccount(t, ctx, store, "org-unrelated")
	wg := createAggregationWorkgroup(t, ctx, store, org, acct, "shared")
	ad := createAggregationAdvertisement(t, ctx, store, org, acct, wg, "provider-ad")
	consumerAd := createAggregationAdvertisement(t, ctx, store, orgProvider, providerAcct, wg, "consumer-ad")
	otherAd := createAggregationAdvertisement(t, ctx, store, orgOther, otherAcct, wg, "other-ad")

	createAggregationSession(t, ctx, store, org, orgConsumer, acct, consumerAcct, wg, ad, SessionStateActive, now.Add(-time.Hour), nil)
	createAggregationSession(t, ctx, store, orgProvider, org, providerAcct, acct, wg, consumerAd, SessionStateProposed, now.Add(-time.Hour), nil)
	createAggregationSession(t, ctx, store, org, org, acct, acct, wg, ad, SessionStateAccepting, now.Add(-time.Hour), nil)
	createAggregationSession(t, ctx, store, orgOther, orgOther, otherAcct, otherAcct, wg, otherAd, SessionStateActive, now.Add(-time.Hour), nil)

	counts, err := DashboardCountsAt(ctx, store.DB(), org.ID, acct.ID, now)
	if err != nil {
		t.Fatalf("dashboard counts: %v", err)
	}
	if counts.Stats.ActiveSessions != 3 {
		t.Fatalf("expected activeSessions=3, got %d", counts.Stats.ActiveSessions)
	}
}

func TestAuditAggregationEnvironmentStatuses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	now := time.Date(2026, 5, 6, 15, 37, 0, 0, time.UTC)

	org, acct := createAggregationOrgAccount(t, ctx, store, "org-env")
	fresh := now.Add(-45 * time.Second)
	stale := now.Add(-46 * time.Second)
	disabledFresh := now.Add(-time.Second)

	online := createAggregationEnvironment(t, ctx, store, org, acct, "env-a-online", &fresh, EnvironmentStateEnabled)
	staleEnv := createAggregationEnvironment(t, ctx, store, org, acct, "env-b-stale", &stale, EnvironmentStateEnabled)
	unknown := createAggregationEnvironment(t, ctx, store, org, acct, "env-c-unknown", nil, EnvironmentStateEnabled)
	disabled := createAggregationEnvironment(t, ctx, store, org, acct, "env-d-disabled", &disabledFresh, EnvironmentStateDisabled)

	statuses, err := EnvironmentStatusesAt(ctx, store.DB(), org.ID, now)
	if err != nil {
		t.Fatalf("environment statuses: %v", err)
	}
	byID := map[string]EnvironmentStatus{}
	for _, status := range statuses {
		byID[status.ID] = status
	}
	if byID[online.ID].Status != EnvironmentStatusOnline {
		t.Fatalf("expected online at 45s old, got %s", byID[online.ID].Status)
	}
	if byID[staleEnv.ID].Status != EnvironmentStatusStale {
		t.Fatalf("expected stale at 46s old, got %s", byID[staleEnv.ID].Status)
	}
	if byID[unknown.ID].Status != EnvironmentStatusUnknown {
		t.Fatalf("expected unknown without heartbeat, got %s", byID[unknown.ID].Status)
	}
	if byID[disabled.ID].Status != EnvironmentStatusDisabled {
		t.Fatalf("expected disabled regardless of heartbeat, got %s", byID[disabled.ID].Status)
	}
	if !byID[online.ID].LastHeartbeatAt.Equal(fresh) {
		t.Fatalf("expected env-a heartbeat %s, got %s", fresh, byID[online.ID].LastHeartbeatAt)
	}
	if !byID[staleEnv.ID].LastHeartbeatAt.Equal(stale) {
		t.Fatalf("expected env-b heartbeat %s, got %s", stale, byID[staleEnv.ID].LastHeartbeatAt)
	}
	gotOrder := make([]EnvironmentStatusValue, 0, len(statuses))
	for _, status := range statuses {
		gotOrder = append(gotOrder, status.Status)
	}
	expectedOrder := []EnvironmentStatusValue{
		EnvironmentStatusOnline,
		EnvironmentStatusStale,
		EnvironmentStatusUnknown,
		EnvironmentStatusDisabled,
	}
	if !reflect.DeepEqual(gotOrder, expectedOrder) {
		t.Fatalf("expected status order %v, got %v", expectedOrder, gotOrder)
	}
}

func TestAuditAggregationWorkgroupBreakdowns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	now := time.Date(2026, 5, 6, 15, 37, 0, 0, time.UTC)

	org, acct := createAggregationOrgAccount(t, ctx, store, "org-workgroups")
	wgs := map[string]*Workgroup{}
	for _, name := range []string{"wg-a", "wg-b", "wg-c", "wg-d", "wg-e", "wg-f", "wg-g"} {
		wgs[name] = createAggregationWorkgroup(t, ctx, store, org, acct, name)
	}
	wgOther, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "wg-other",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create other workgroup: %v", err)
	}

	for name, count := range map[string]int{
		"wg-a": 100,
		"wg-b": 80,
		"wg-c": 60,
		"wg-d": 40,
		"wg-e": 20,
	} {
		recordAggregationEnvelope(t, ctx, store, org.ID, wgs[name].ID, now.Add(-time.Hour), count)
	}
	recordAggregationEnvelope(t, ctx, store, org.ID, wgOther.ID, now.Add(-time.Hour), 200)

	top, err := EnvelopesByWorkgroupAt(ctx, store.DB(), org.ID, now, 24*time.Hour, 5)
	if err != nil {
		t.Fatalf("envelopes by workgroup: %v", err)
	}
	topNames := workgroupEnvelopeNames(top)
	expectedTop := []string{"wg-other", "wg-a", "wg-b", "wg-c", "wg-d"}
	if !reflect.DeepEqual(topNames, expectedTop) {
		t.Fatalf("expected top workgroups %v, got %v", expectedTop, topNames)
	}

	visible, err := EnvelopesByVisibleWorkgroupsAt(ctx, store.DB(), org.ID, acct.ID, now, 24*time.Hour)
	if err != nil {
		t.Fatalf("visible envelopes by workgroup: %v", err)
	}
	visibleNames := workgroupEnvelopeNames(visible)
	expectedVisible := []string{"wg-a", "wg-b", "wg-c", "wg-d", "wg-e", "wg-f", "wg-g"}
	if !reflect.DeepEqual(visibleNames, expectedVisible) {
		t.Fatalf("expected visible workgroups %v, got %v", expectedVisible, visibleNames)
	}
	visibleCounts := map[string]int{}
	for _, row := range visible {
		visibleCounts[row.WorkgroupName] = row.Envelopes
	}
	if visibleCounts["wg-f"] != 0 || visibleCounts["wg-g"] != 0 {
		t.Fatalf("expected zero rows for wg-f/wg-g, got %d/%d", visibleCounts["wg-f"], visibleCounts["wg-g"])
	}
	if _, ok := visibleCounts["wg-other"]; ok {
		t.Fatalf("expected non-member wg-other to be absent from visible helper")
	}
}

func bucketByStart(t *testing.T, buckets []Bucket, start time.Time) Bucket {
	t.Helper()
	for _, bucket := range buckets {
		if bucket.Start.Equal(start) {
			return bucket
		}
	}
	t.Fatalf("bucket %s not found", start)
	return Bucket{}
}

func sumBucketEnvelopes(buckets []Bucket) int {
	total := 0
	for _, bucket := range buckets {
		total += bucket.Envelopes
	}
	return total
}

func sumBucketSessions(buckets []Bucket) int {
	total := 0
	for _, bucket := range buckets {
		total += bucket.Sessions
	}
	return total
}

func workgroupEnvelopeNames(rows []WorkgroupEnvelopes) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.WorkgroupName)
	}
	return names
}

func recordAggregationEnvelope(t *testing.T, ctx context.Context, store *Store, orgID, workgroupID string, occurredAt time.Time, countDelta int) {
	t.Helper()
	event := AuditEvent{
		OccurredAt:     occurredAt.UTC(),
		EventType:      AuditEventEnvelopeFlowed,
		OrganizationID: orgID,
		WorkgroupID:    stringPtr(workgroupID),
		Data: AuditEventData{
			"count_delta": countDelta,
			"total_count": countDelta,
		},
	}
	if err := store.AuditEvents.Record(ctx, store.DB(), event); err != nil {
		t.Fatalf("record envelope event: %v", err)
	}
}

func recordAggregationSessionProposed(t *testing.T, ctx context.Context, store *Store, orgID, workgroupID string, occurredAt time.Time) {
	t.Helper()
	event := AuditEvent{
		OccurredAt:     occurredAt.UTC(),
		EventType:      AuditEventSessionProposed,
		OrganizationID: orgID,
		WorkgroupID:    stringPtr(workgroupID),
		Data:           AuditEventData{},
	}
	if err := store.AuditEvents.Record(ctx, store.DB(), event); err != nil {
		t.Fatalf("record session proposed event: %v", err)
	}
}

func createAggregationOrgAccount(t *testing.T, ctx context.Context, store *Store, name string) (*Organization, *Account) {
	t.Helper()
	org, err := store.Organizations.Create(ctx, store.DB(), Organization{Name: name + "-" + NewResourceID(PrefixOrganization)})
	if err != nil {
		t.Fatalf("create aggregation org: %v", err)
	}
	email := strings.ReplaceAll(name, "_", "-") + "-" + NewResourceID(PrefixAccount) + "@example.com"
	acct, err := store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: org.ID,
		Email:          email,
		PasswordSalt:   "salt",
		PasswordHash:   "hash",
		AccountToken:   "token-" + NewResourceID(PrefixAccount),
		Role:           AccountRoleAdmin,
		Status:         AccountStatusActive,
	})
	if err != nil {
		t.Fatalf("create aggregation account: %v", err)
	}
	return org, acct
}

func createAggregationWorkgroup(t *testing.T, ctx context.Context, store *Store, org *Organization, acct *Account, name string) *Workgroup {
	t.Helper()
	wg, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                name,
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create aggregation workgroup: %v", err)
	}
	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID:    wg.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Role:           WorkgroupMembershipRoleAdmin,
	}); err != nil {
		t.Fatalf("create aggregation workgroup membership: %v", err)
	}
	return wg
}

func createAggregationAdvertisement(t *testing.T, ctx context.Context, store *Store, org *Organization, acct *Account, wg *Workgroup, name string) *Advertisement {
	t.Helper()
	ad, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:      org.ID,
		AccountID:           acct.ID,
		Name:                name + "-" + NewResourceID(PrefixAdvertisement),
		Capabilities:        CapabilitiesJSON{{Name: "quote"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wg.ID},
		TunnelMode:          TunnelModeTCP,
		Status:              AdvertisementStatusActive,
	})
	if err != nil {
		t.Fatalf("create aggregation advertisement: %v", err)
	}
	return ad
}

func createAggregationSession(t *testing.T, ctx context.Context, store *Store, providerOrg, consumerOrg *Organization, providerAcct, consumerAcct *Account, wg *Workgroup, ad *Advertisement, state SessionState, proposedAt time.Time, closedAt *time.Time) *Session {
	t.Helper()
	var closeReason *SessionCloseReason
	if state == SessionStateClosed {
		reason := SessionCloseReasonConsumerClose
		closeReason = &reason
	}
	sess, err := store.Sessions.Create(ctx, store.DB(), Session{
		AdvertisementID:        ad.ID,
		WorkgroupID:            wg.ID,
		ProviderAccountID:      providerAcct.ID,
		ProviderOrganizationID: providerOrg.ID,
		ConsumerAccountID:      consumerAcct.ID,
		ConsumerOrganizationID: consumerOrg.ID,
		TunnelMode:             TunnelModeTCP,
		State:                  state,
		CloseReason:            closeReason,
		ProposedAt:             proposedAt.UTC(),
		ClosedAt:               closedAt,
	})
	if err != nil {
		t.Fatalf("create aggregation session: %v", err)
	}
	return sess
}

func createAggregationEnvironment(t *testing.T, ctx context.Context, store *Store, org *Organization, acct *Account, name string, lastSeenAt *time.Time, state EnvironmentState) *Environment {
	t.Helper()
	host := name
	env, err := store.Environments.Create(ctx, store.DB(), Environment{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Host:           &host,
		ZitiIdentityID: "ziti-" + NewResourceID(PrefixEnvironment),
		State:          state,
		LastSeenAt:     lastSeenAt,
	})
	if err != nil {
		t.Fatalf("create aggregation environment: %v", err)
	}
	return env
}

func createAggregationTunnel(t *testing.T, ctx context.Context, store *Store, org *Organization, acct *Account, env *Environment) *Tunnel {
	t.Helper()
	tunnel, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  stringPtr(env.ID),
		Name:           "tunnel-" + NewResourceID(PrefixTunnel),
		Mode:           TunnelModeTCP,
		BackendTarget:  stringPtr("127.0.0.1:9000"),
		State:          TunnelStateActive,
	})
	if err != nil {
		t.Fatalf("create aggregation tunnel: %v", err)
	}
	return tunnel
}

func createAggregationAttachment(t *testing.T, ctx context.Context, store *Store, org *Organization, acct *Account, env *Environment, tunnel *Tunnel, state TunnelAttachmentState) *TunnelAttachment {
	t.Helper()
	attachment, err := store.TunnelAttachments.Create(ctx, store.DB(), TunnelAttachment{
		TunnelID:       tunnel.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		ListenAddress:  stringPtr("127.0.0.1:0"),
		State:          state,
	})
	if err != nil {
		t.Fatalf("create aggregation attachment: %v", err)
	}
	return attachment
}

func createAggregationServe(t *testing.T, ctx context.Context, store *Store, org *Organization, acct *Account, env *Environment, tunnel *Tunnel, state TunnelServeState) *TunnelServe {
	t.Helper()
	serve, err := store.TunnelServes.Create(ctx, store.DB(), TunnelServe{
		TunnelID:       tunnel.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		State:          state,
	})
	if err != nil {
		t.Fatalf("create aggregation serve: %v", err)
	}
	return serve
}

func timePtr(t time.Time) *time.Time {
	v := t.UTC()
	return &v
}
