package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestAuditEventConstructorsRecordRoundtrip(t *testing.T) {
	ctx := context.Background()
	store := migratedTestStore(t)

	providerOrgID := NewResourceID(PrefixOrganization)
	consumerOrgID := NewResourceID(PrefixOrganization)
	providerAccountID := NewResourceID(PrefixAccount)
	consumerAccountID := NewResourceID(PrefixAccount)
	workgroupID := NewResourceID(PrefixWorkgroup)
	advertisementID := NewResourceID(PrefixAdvertisement)
	sessionID := NewResourceID(PrefixSession)
	tunnelID := NewResourceID(PrefixTunnel)
	attachmentID := NewResourceID(PrefixAttachment)
	environmentID := NewResourceID(PrefixEnvironment)
	contractID := NewResourceID(PrefixContract)
	otherWorkgroupID := NewResourceID(PrefixWorkgroup)

	sess := Session{
		ID:                     sessionID,
		AdvertisementID:        advertisementID,
		WorkgroupID:            workgroupID,
		ProviderAccountID:      providerAccountID,
		ProviderOrganizationID: providerOrgID,
		ConsumerAccountID:      consumerAccountID,
		ConsumerOrganizationID: consumerOrgID,
		TunnelMode:             TunnelModeTCP,
		TunnelID:               &tunnelID,
		State:                  SessionStateActive,
		ProposedAt:             time.Date(2026, 5, 6, 11, 0, 0, 0, time.UTC),
	}
	attachment := TunnelAttachment{
		ID:             attachmentID,
		TunnelID:       tunnelID,
		OrganizationID: consumerOrgID,
		AccountID:      consumerAccountID,
		EnvironmentID:  environmentID,
		State:          TunnelAttachmentStateActive,
	}
	ad := Advertisement{
		ID:              advertisementID,
		OrganizationID:  providerOrgID,
		AccountID:       providerAccountID,
		Name:            "quote-feed",
		WorkgroupScopes: pq.StringArray{workgroupID, otherWorkgroupID},
		TunnelMode:      TunnelModeTCP,
		ContractID:      &contractID,
	}
	env := Environment{
		ID:             environmentID,
		OrganizationID: consumerOrgID,
		AccountID:      consumerAccountID,
		State:          EnvironmentStateEnabled,
	}
	acct := Account{
		ID:             consumerAccountID,
		OrganizationID: consumerOrgID,
		Email:          "demo@agora.local",
	}

	fixed := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		build func() AuditEvent
		want  AuditEvent
		data  AuditEventData
	}{
		{
			name:  "session proposed",
			build: func() AuditEvent { return NewSessionProposedEvent(sess, providerOrgID, providerAccountID) },
			want: AuditEvent{
				EventType:       AuditEventSessionProposed,
				OrganizationID:  providerOrgID,
				AccountID:       &providerAccountID,
				WorkgroupID:     &workgroupID,
				SessionID:       &sessionID,
				AdvertisementID: &advertisementID,
			},
			data: AuditEventData{
				"provider_account_id":      providerAccountID,
				"provider_organization_id": providerOrgID,
				"consumer_account_id":      consumerAccountID,
				"consumer_organization_id": consumerOrgID,
				"tunnel_mode":              "tcp",
			},
		},
		{
			name:  "session accepted",
			build: func() AuditEvent { return NewSessionAcceptedEvent(sess, consumerOrgID, consumerAccountID, &contractID) },
			want: AuditEvent{
				EventType:       AuditEventSessionAccepted,
				OrganizationID:  consumerOrgID,
				AccountID:       &consumerAccountID,
				WorkgroupID:     &workgroupID,
				SessionID:       &sessionID,
				AdvertisementID: &advertisementID,
				ContractID:      &contractID,
			},
			data: AuditEventData{
				"provider_account_id":      providerAccountID,
				"provider_organization_id": providerOrgID,
				"consumer_account_id":      consumerAccountID,
				"consumer_organization_id": consumerOrgID,
				"tunnel_id":                tunnelID,
				"contract_id":              contractID,
			},
		},
		{
			name:  "session rejected",
			build: func() AuditEvent { return NewSessionRejectedEvent(sess, consumerOrgID, consumerAccountID, "not_ready") },
			want: AuditEvent{
				EventType:       AuditEventSessionRejected,
				OrganizationID:  consumerOrgID,
				AccountID:       &consumerAccountID,
				WorkgroupID:     &workgroupID,
				SessionID:       &sessionID,
				AdvertisementID: &advertisementID,
			},
			data: AuditEventData{
				"provider_organization_id": providerOrgID,
				"consumer_organization_id": consumerOrgID,
				"reason":                   "not_ready",
			},
		},
		{
			name: "session closed",
			build: func() AuditEvent {
				return NewSessionClosedEvent(sess, providerOrgID, providerAccountID, SessionCloseReasonContractViolation, "envelope 4096 > 1024", 42, "envelope_bytes")
			},
			want: AuditEvent{
				EventType:       AuditEventSessionClosed,
				OrganizationID:  providerOrgID,
				AccountID:       &providerAccountID,
				WorkgroupID:     &workgroupID,
				SessionID:       &sessionID,
				AdvertisementID: &advertisementID,
			},
			data: AuditEventData{
				"provider_organization_id": providerOrgID,
				"consumer_organization_id": consumerOrgID,
				"close_reason":             "contract_violation",
				"close_detail":             "envelope 4096 > 1024",
				"duration_seconds":         int64(42),
				"violation_dimension":      "envelope_bytes",
			},
		},
		{
			name:  "envelope flowed",
			build: func() AuditEvent { return NewEnvelopeFlowedEvent(sess, providerOrgID, providerAccountID, 7, 21) },
			want: AuditEvent{
				EventType:       AuditEventEnvelopeFlowed,
				OrganizationID:  providerOrgID,
				AccountID:       &providerAccountID,
				WorkgroupID:     &workgroupID,
				SessionID:       &sessionID,
				AdvertisementID: &advertisementID,
			},
			data: AuditEventData{
				"provider_organization_id": providerOrgID,
				"consumer_organization_id": consumerOrgID,
				"count_delta":              7,
				"total_count":              21,
			},
		},
		{
			name:  "tunnel attached",
			build: func() AuditEvent { return NewTunnelAttachedEvent(attachment, &sessionID) },
			want: AuditEvent{
				EventType:      AuditEventTunnelAttached,
				OrganizationID: consumerOrgID,
				AccountID:      &consumerAccountID,
				SessionID:      &sessionID,
			},
			data: AuditEventData{"tunnel_id": tunnelID},
		},
		{
			name: "tunnel detached",
			build: func() AuditEvent {
				return NewTunnelDetachedEvent(attachment, &sessionID, TunnelAttachmentStateDisconnected)
			},
			want: AuditEvent{
				EventType:      AuditEventTunnelDetached,
				OrganizationID: consumerOrgID,
				AccountID:      &consumerAccountID,
				SessionID:      &sessionID,
			},
			data: AuditEventData{
				"tunnel_id":   tunnelID,
				"final_state": "disconnected",
			},
		},
		{
			name:  "advertisement published",
			build: func() AuditEvent { return NewAdvertisementPublishedEvent(ad) },
			want: AuditEvent{
				EventType:       AuditEventAdvertisementPublished,
				OrganizationID:  providerOrgID,
				AccountID:       &providerAccountID,
				AdvertisementID: &advertisementID,
				ContractID:      &contractID,
			},
			data: AuditEventData{
				"name":             "quote-feed",
				"tunnel_mode":      "tcp",
				"workgroup_scopes": []string{workgroupID, otherWorkgroupID},
			},
		},
		{
			name:  "advertisement retracted",
			build: func() AuditEvent { return NewAdvertisementRetractedEvent(ad, "demo reset") },
			want: AuditEvent{
				EventType:       AuditEventAdvertisementRetracted,
				OrganizationID:  providerOrgID,
				AccountID:       &providerAccountID,
				AdvertisementID: &advertisementID,
				ContractID:      &contractID,
			},
			data: AuditEventData{"reason": "demo reset"},
		},
		{
			name:  "environment heartbeat",
			build: func() AuditEvent { return NewEnvironmentHeartbeatEvent(env, 14) },
			want: AuditEvent{
				EventType:      AuditEventEnvironmentHeartbeat,
				OrganizationID: consumerOrgID,
				AccountID:      &consumerAccountID,
			},
			data: AuditEventData{
				"environment_id": environmentID,
				"latency_ms":     14,
			},
		},
		{
			name:  "account login",
			build: func() AuditEvent { return NewAccountLoginEvent(acct) },
			want: AuditEvent{
				EventType:      AuditEventAccountLogin,
				OrganizationID: consumerOrgID,
				AccountID:      &consumerAccountID,
			},
			data: AuditEventData{"email": "demo@agora.local"},
		},
		{
			name:  "account login failed",
			build: func() AuditEvent { return NewAccountLoginFailedEvent(acct, "Demo@Agora.Local") },
			want: AuditEvent{
				EventType:      AuditEventAccountLoginFailed,
				OrganizationID: consumerOrgID,
				AccountID:      &consumerAccountID,
			},
			data: AuditEventData{"email_attempted": "Demo@Agora.Local"},
		},
		{
			name:  "account logout",
			build: func() AuditEvent { return NewAccountLogoutEvent(acct) },
			want: AuditEvent{
				EventType:      AuditEventAccountLogout,
				OrganizationID: consumerOrgID,
				AccountID:      &consumerAccountID,
			},
			data: AuditEventData{"email": "demo@agora.local"},
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := tc.build()
			event.OccurredAt = fixed.Add(time.Duration(i) * time.Second)
			assertAuditEventShape(t, event, tc.want, tc.data)

			if err := store.AuditEvents.Record(ctx, store.DB(), event); err != nil {
				t.Fatalf("record audit event: %v", err)
			}
			reloaded := latestAuditEvent(t, ctx, store)
			assertAuditEventShape(t, reloaded, withOccurredAt(tc.want, event.OccurredAt), tc.data)
		})
	}
}

func TestAuditEventsRecordDirectAndTransactional(t *testing.T) {
	ctx := context.Background()
	store := migratedTestStore(t)

	accountID := NewResourceID(PrefixAccount)
	event := AuditEvent{
		OccurredAt:     time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
		EventType:      AuditEventAccountLogin,
		OrganizationID: NewResourceID(PrefixOrganization),
		AccountID:      &accountID,
		Data:           AuditEventData{"email": "demo@agora.local"},
	}

	if err := store.AuditEvents.Record(ctx, store.DB(), event); err != nil {
		t.Fatalf("record direct: %v", err)
	}
	if err := store.WithTx(ctx, func(tx Queryer) error {
		return store.AuditEvents.Record(ctx, tx, event)
	}); err != nil {
		t.Fatalf("record tx: %v", err)
	}

	events := listAuditEvents(t, ctx, store)
	if len(events) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(events))
	}
	assertAuditEventsEqualExceptID(t, events[0], events[1])
}

func TestAuditEventsRecordRollsBackWithTransaction(t *testing.T) {
	ctx := context.Background()
	store := migratedTestStore(t)

	accountID := NewResourceID(PrefixAccount)
	event := AuditEvent{
		EventType:      AuditEventAccountLogout,
		OrganizationID: NewResourceID(PrefixOrganization),
		AccountID:      &accountID,
		Data:           AuditEventData{"email": "demo@agora.local"},
	}

	err := store.WithTx(ctx, func(tx Queryer) error {
		if err := store.AuditEvents.Record(ctx, tx, event); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}

	var count int
	if err := store.DB().GetContext(ctx, &count, "select count(*) from audit_events"); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no audit events after rollback, got %d", count)
	}
}

func TestAuditEventsListFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	store := migratedTestStore(t)

	orgA := NewResourceID(PrefixOrganization)
	orgB := NewResourceID(PrefixOrganization)
	accountA := NewResourceID(PrefixAccount)
	accountB := NewResourceID(PrefixAccount)
	workgroupA := NewResourceID(PrefixWorkgroup)
	workgroupB := NewResourceID(PrefixWorkgroup)
	base := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

	record := func(event AuditEvent) AuditEvent {
		t.Helper()
		if err := store.AuditEvents.Record(ctx, store.DB(), event); err != nil {
			t.Fatalf("record audit event: %v", err)
		}
		return latestAuditEvent(t, ctx, store)
	}

	old := record(AuditEvent{
		OccurredAt:     base,
		EventType:      AuditEventAccountLogin,
		OrganizationID: orgA,
		AccountID:      &accountA,
		Data:           AuditEventData{"email": "a@example.com"},
	})
	middle := record(AuditEvent{
		OccurredAt:     base.Add(time.Minute),
		EventType:      AuditEventEnvelopeFlowed,
		OrganizationID: orgA,
		AccountID:      &accountA,
		WorkgroupID:    &workgroupA,
		Data:           AuditEventData{"count_delta": 3},
	})
	tiedFirstInserted := record(AuditEvent{
		OccurredAt:     base.Add(2 * time.Minute),
		EventType:      AuditEventSessionClosed,
		OrganizationID: orgA,
		AccountID:      &accountA,
		WorkgroupID:    &workgroupA,
		Data:           AuditEventData{"close_reason": "consumer_close"},
	})
	tiedSecondInserted := record(AuditEvent{
		OccurredAt:     base.Add(2 * time.Minute),
		EventType:      AuditEventSessionAccepted,
		OrganizationID: orgA,
		AccountID:      &accountB,
		WorkgroupID:    &workgroupB,
		Data:           AuditEventData{"tunnel_id": "tn_123"},
	})
	record(AuditEvent{
		OccurredAt:     base.Add(3 * time.Minute),
		EventType:      AuditEventAccountLogin,
		OrganizationID: orgB,
		AccountID:      &accountB,
		Data:           AuditEventData{"email": "b@example.com"},
	})

	page1, cursor, err := store.AuditEvents.List(ctx, store.DB(), AuditEventListParams{OrganizationID: orgA, Limit: 2})
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	assertAuditEventIDs(t, page1, []int64{tiedSecondInserted.ID, tiedFirstInserted.ID})
	if cursor == "" {
		t.Fatal("expected pagination cursor")
	}
	page2, cursor2, err := store.AuditEvents.List(ctx, store.DB(), AuditEventListParams{OrganizationID: orgA, Cursor: mustDecodeAuditCursor(t, cursor), Limit: 2})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	assertAuditEventIDs(t, page2, []int64{middle.ID, old.ID})
	if cursor2 != "" {
		t.Fatalf("expected final page to omit cursor, got %q", cursor2)
	}

	envelopes, _, err := store.AuditEvents.List(ctx, store.DB(), AuditEventListParams{
		OrganizationID: orgA,
		EventTypes:     []AuditEventType{AuditEventEnvelopeFlowed},
	})
	if err != nil {
		t.Fatalf("list event type filter: %v", err)
	}
	assertAuditEventIDs(t, envelopes, []int64{middle.ID})

	workgroupRows, _, err := store.AuditEvents.List(ctx, store.DB(), AuditEventListParams{
		OrganizationID: orgA,
		WorkgroupID:    workgroupA,
	})
	if err != nil {
		t.Fatalf("list workgroup filter: %v", err)
	}
	assertAuditEventIDs(t, workgroupRows, []int64{tiedFirstInserted.ID, middle.ID})

	accountRows, _, err := store.AuditEvents.List(ctx, store.DB(), AuditEventListParams{
		OrganizationID: orgA,
		AccountID:      accountB,
	})
	if err != nil {
		t.Fatalf("list account filter: %v", err)
	}
	assertAuditEventIDs(t, accountRows, []int64{tiedSecondInserted.ID})

	from := base.Add(time.Minute)
	to := base.Add(2 * time.Minute)
	windowRows, _, err := store.AuditEvents.List(ctx, store.DB(), AuditEventListParams{
		OrganizationID: orgA,
		From:           &from,
		To:             &to,
	})
	if err != nil {
		t.Fatalf("list time filter: %v", err)
	}
	assertAuditEventIDs(t, windowRows, []int64{middle.ID})
}

func latestAuditEvent(t *testing.T, ctx context.Context, store *Store) AuditEvent {
	t.Helper()

	query := fmt.Sprintf(`
select %s
from audit_events
order by id desc
limit 1`, auditEventColumns)
	var event AuditEvent
	if err := store.DB().GetContext(ctx, &event, query); err != nil {
		t.Fatalf("get latest audit event: %v", err)
	}
	return event
}

func listAuditEvents(t *testing.T, ctx context.Context, store *Store) []AuditEvent {
	t.Helper()

	query := fmt.Sprintf(`
select %s
from audit_events
order by id asc`, auditEventColumns)
	var events []AuditEvent
	if err := store.DB().SelectContext(ctx, &events, query); err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	return events
}

func mustDecodeAuditCursor(t *testing.T, cursor string) *AuditEventCursor {
	t.Helper()
	decoded, err := DecodeAuditEventCursor(cursor)
	if err != nil {
		t.Fatalf("decode audit cursor: %v", err)
	}
	return decoded
}

func assertAuditEventIDs(t *testing.T, events []AuditEvent, want []int64) {
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

func assertAuditEventShape(t *testing.T, got AuditEvent, want AuditEvent, wantData AuditEventData) {
	t.Helper()

	if got.EventType != want.EventType {
		t.Fatalf("event type: expected %s, got %s", want.EventType, got.EventType)
	}
	if got.OrganizationID != want.OrganizationID {
		t.Fatalf("organization_id: expected %s, got %s", want.OrganizationID, got.OrganizationID)
	}
	if !want.OccurredAt.IsZero() && !got.OccurredAt.Equal(want.OccurredAt) {
		t.Fatalf("occurred_at: expected %v, got %v", want.OccurredAt, got.OccurredAt)
	}
	assertStringPtrEqual(t, "account_id", got.AccountID, want.AccountID)
	assertStringPtrEqual(t, "workgroup_id", got.WorkgroupID, want.WorkgroupID)
	assertStringPtrEqual(t, "session_id", got.SessionID, want.SessionID)
	assertStringPtrEqual(t, "advertisement_id", got.AdvertisementID, want.AdvertisementID)
	assertStringPtrEqual(t, "contract_id", got.ContractID, want.ContractID)
	assertStringPtrEqual(t, "envelope_id", got.EnvelopeID, want.EnvelopeID)
	assertJSONEqual(t, got.Data, wantData)
}

func assertAuditEventsEqualExceptID(t *testing.T, first, second AuditEvent) {
	t.Helper()

	first.ID = 0
	second.ID = 0
	if !first.OccurredAt.Equal(second.OccurredAt) {
		t.Fatalf("occurred_at mismatch: %v != %v", first.OccurredAt, second.OccurredAt)
	}
	if first.EventType != second.EventType ||
		first.OrganizationID != second.OrganizationID ||
		!sameStringPtr(first.AccountID, second.AccountID) ||
		!sameStringPtr(first.WorkgroupID, second.WorkgroupID) ||
		!sameStringPtr(first.SessionID, second.SessionID) ||
		!sameStringPtr(first.AdvertisementID, second.AdvertisementID) ||
		!sameStringPtr(first.ContractID, second.ContractID) ||
		!sameStringPtr(first.EnvelopeID, second.EnvelopeID) {
		t.Fatalf("events differ:\nfirst=%+v\nsecond=%+v", first, second)
	}
	assertJSONEqual(t, first.Data, second.Data)
}

func withOccurredAt(event AuditEvent, occurredAt time.Time) AuditEvent {
	event.OccurredAt = occurredAt
	return event
}

func assertStringPtrEqual(t *testing.T, name string, got, want *string) {
	t.Helper()
	if !sameStringPtr(got, want) {
		t.Fatalf("%s: expected %v, got %v", name, printableStringPtr(want), printableStringPtr(got))
	}
}

func sameStringPtr(a, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func printableStringPtr(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func assertJSONEqual(t *testing.T, got, want any) {
	t.Helper()

	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got json: %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want json: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("json mismatch:\nwant=%s\n got=%s", wantJSON, gotJSON)
	}
}
