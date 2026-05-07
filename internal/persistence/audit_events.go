package persistence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type AuditEventsRepository struct{}

const auditEventColumns = `id, occurred_at, event_type, organization_id, account_id, workgroup_id, session_id, advertisement_id, contract_id, envelope_id, data`
const auditEventsDefaultLimit = 100
const auditEventsMaxLimit = 200

type AuditEventListParams struct {
	OrganizationID string
	EventTypes     []AuditEventType
	WorkgroupID    string
	AccountID      string
	From           *time.Time
	To             *time.Time
	Cursor         *AuditEventCursor
	Limit          int
}

// AuditEventCursor encodes the position of the last returned audit row
// in the deterministic sort order (occurred_at desc, id desc).
type AuditEventCursor struct {
	OccurredAt time.Time `json:"occurredAt"`
	ID         int64     `json:"id"`
}

func EncodeAuditEventCursor(c *AuditEventCursor) (string, error) {
	if c == nil {
		return "", nil
	}
	bytes, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode audit cursor: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func DecodeAuditEventCursor(raw string) (*AuditEventCursor, error) {
	if raw == "" {
		return nil, nil
	}
	bytes, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode audit cursor: %w", err)
	}
	var c AuditEventCursor
	if err := json.Unmarshal(bytes, &c); err != nil {
		return nil, fmt.Errorf("unmarshal audit cursor: %w", err)
	}
	return &c, nil
}

func auditEventInsertColumns() string {
	return `occurred_at, event_type, organization_id, account_id, workgroup_id, session_id, advertisement_id, contract_id, envelope_id, data`
}

func auditEventInsertArgs(event AuditEvent) []any {
	return []any{
		event.OccurredAt,
		event.EventType,
		event.OrganizationID,
		event.AccountID,
		event.WorkgroupID,
		event.SessionID,
		event.AdvertisementID,
		event.ContractID,
		event.EnvelopeID,
		event.Data,
	}
}

func (r *AuditEventsRepository) Record(ctx context.Context, db Queryer, event AuditEvent) error {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if event.Data == nil {
		event.Data = AuditEventData{}
	}

	query := fmt.Sprintf(`
insert into audit_events (%s)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, auditEventInsertColumns())
	if _, err := db.ExecContext(ctx, query, auditEventInsertArgs(event)...); err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func (r *AuditEventsRepository) GetByID(ctx context.Context, db Queryer, id int64) (*AuditEvent, error) {
	query := fmt.Sprintf(`
select %s
from audit_events
where id = $1`, auditEventColumns)

	var event AuditEvent
	if err := db.GetContext(ctx, &event, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get audit event: %w", err)
	}
	return &event, nil
}

func (r *AuditEventsRepository) List(ctx context.Context, db Queryer, params AuditEventListParams) ([]AuditEvent, string, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = auditEventsDefaultLimit
	}
	if limit > auditEventsMaxLimit {
		limit = auditEventsMaxLimit
	}

	clauses := []string{"organization_id = $1"}
	args := []any{params.OrganizationID}

	if len(params.EventTypes) > 0 {
		eventTypes := make([]string, 0, len(params.EventTypes))
		for _, eventType := range params.EventTypes {
			eventTypes = append(eventTypes, string(eventType))
		}
		args = append(args, pq.StringArray(eventTypes))
		clauses = append(clauses, fmt.Sprintf("event_type = any($%d)", len(args)))
	}
	if params.WorkgroupID != "" {
		args = append(args, params.WorkgroupID)
		clauses = append(clauses, fmt.Sprintf("workgroup_id = $%d", len(args)))
	}
	if params.AccountID != "" {
		args = append(args, params.AccountID)
		clauses = append(clauses, fmt.Sprintf("account_id = $%d", len(args)))
	}
	if params.From != nil {
		args = append(args, *params.From)
		clauses = append(clauses, fmt.Sprintf("occurred_at >= $%d", len(args)))
	}
	if params.To != nil {
		args = append(args, *params.To)
		clauses = append(clauses, fmt.Sprintf("occurred_at < $%d", len(args)))
	}
	if params.Cursor != nil {
		args = append(args, params.Cursor.OccurredAt)
		occurredIdx := len(args)
		args = append(args, params.Cursor.ID)
		idIdx := len(args)
		clauses = append(clauses, fmt.Sprintf("(occurred_at < $%d or (occurred_at = $%d and id < $%d))", occurredIdx, occurredIdx, idIdx))
	}

	args = append(args, limit+1)
	limitIdx := len(args)

	query := fmt.Sprintf(`
select %s
from audit_events
where %s
order by occurred_at desc, id desc
limit $%d`, auditEventColumns, strings.Join(clauses, " and "), limitIdx)

	var rows []AuditEvent
	if err := db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, "", fmt.Errorf("list audit events: %w", err)
	}

	var nextCursor string
	if len(rows) > limit {
		last := rows[limit-1]
		encoded, err := EncodeAuditEventCursor(&AuditEventCursor{OccurredAt: last.OccurredAt, ID: last.ID})
		if err != nil {
			return nil, "", err
		}
		nextCursor = encoded
		rows = rows[:limit]
	}
	return rows, nextCursor, nil
}

func NewSessionProposedEvent(sess Session, organizationID, accountID string) AuditEvent {
	return sessionAuditEvent(AuditEventSessionProposed, sess, organizationID, accountID, AuditEventData{
		"provider_account_id":      sess.ProviderAccountID,
		"provider_organization_id": sess.ProviderOrganizationID,
		"consumer_account_id":      sess.ConsumerAccountID,
		"consumer_organization_id": sess.ConsumerOrganizationID,
		"tunnel_mode":              string(sess.TunnelMode),
	})
}

func NewSessionAcceptedEvent(sess Session, organizationID, accountID string, contractID *string) AuditEvent {
	data := AuditEventData{
		"provider_account_id":      sess.ProviderAccountID,
		"provider_organization_id": sess.ProviderOrganizationID,
		"consumer_account_id":      sess.ConsumerAccountID,
		"consumer_organization_id": sess.ConsumerOrganizationID,
		"tunnel_id":                nil,
		"contract_id":              nil,
	}
	if sess.TunnelID != nil {
		data["tunnel_id"] = *sess.TunnelID
	}
	if contractID != nil {
		data["contract_id"] = *contractID
	}

	event := sessionAuditEvent(AuditEventSessionAccepted, sess, organizationID, accountID, data)
	event.ContractID = cloneStringPtr(contractID)
	return event
}

func NewSessionRejectedEvent(sess Session, organizationID, accountID, reason string) AuditEvent {
	return sessionAuditEvent(AuditEventSessionRejected, sess, organizationID, accountID, AuditEventData{
		"provider_organization_id": sess.ProviderOrganizationID,
		"consumer_organization_id": sess.ConsumerOrganizationID,
		"reason":                   reason,
	})
}

func NewSessionClosedEvent(sess Session, organizationID, accountID string, reason SessionCloseReason, detail string, durationSeconds int64, violationDimension string) AuditEvent {
	data := AuditEventData{
		"provider_organization_id": sess.ProviderOrganizationID,
		"consumer_organization_id": sess.ConsumerOrganizationID,
		"close_reason":             string(reason),
		"duration_seconds":         durationSeconds,
	}
	if detail != "" {
		data["close_detail"] = detail
	}
	if violationDimension != "" {
		data["violation_dimension"] = violationDimension
	}
	return sessionAuditEvent(AuditEventSessionClosed, sess, organizationID, accountID, data)
}

func NewEnvelopeFlowedEvent(sess Session, organizationID, accountID string, countDelta, totalCount int) AuditEvent {
	return sessionAuditEvent(AuditEventEnvelopeFlowed, sess, organizationID, accountID, AuditEventData{
		"provider_organization_id": sess.ProviderOrganizationID,
		"consumer_organization_id": sess.ConsumerOrganizationID,
		"count_delta":              countDelta,
		"total_count":              totalCount,
	})
}

func NewTunnelAttachedEvent(attachment TunnelAttachment, sessionID *string) AuditEvent {
	event := newAuditEvent(AuditEventTunnelAttached, attachment.OrganizationID)
	event.AccountID = stringPtr(attachment.AccountID)
	event.SessionID = cloneStringPtr(sessionID)
	event.Data = AuditEventData{
		"tunnel_id": attachment.TunnelID,
	}
	return event
}

func NewTunnelDetachedEvent(attachment TunnelAttachment, sessionID *string, finalState TunnelAttachmentState) AuditEvent {
	event := newAuditEvent(AuditEventTunnelDetached, attachment.OrganizationID)
	event.AccountID = stringPtr(attachment.AccountID)
	event.SessionID = cloneStringPtr(sessionID)
	event.Data = AuditEventData{
		"tunnel_id":   attachment.TunnelID,
		"final_state": string(finalState),
	}
	return event
}

func NewAdvertisementPublishedEvent(ad Advertisement) AuditEvent {
	event := newAuditEvent(AuditEventAdvertisementPublished, ad.OrganizationID)
	event.AccountID = stringPtr(ad.AccountID)
	event.AdvertisementID = stringPtr(ad.ID)
	event.ContractID = cloneStringPtr(ad.ContractID)
	event.Data = AuditEventData{
		"name":             ad.Name,
		"tunnel_mode":      string(ad.TunnelMode),
		"workgroup_scopes": []string(ad.WorkgroupScopes),
	}
	return event
}

func NewAdvertisementRetractedEvent(ad Advertisement, reason string) AuditEvent {
	event := newAuditEvent(AuditEventAdvertisementRetracted, ad.OrganizationID)
	event.AccountID = stringPtr(ad.AccountID)
	event.AdvertisementID = stringPtr(ad.ID)
	event.ContractID = cloneStringPtr(ad.ContractID)
	event.Data = AuditEventData{"reason": reason}
	return event
}

func NewEnvironmentHeartbeatEvent(env Environment, latencyMS int) AuditEvent {
	event := newAuditEvent(AuditEventEnvironmentHeartbeat, env.OrganizationID)
	event.AccountID = stringPtr(env.AccountID)
	event.Data = AuditEventData{
		"environment_id": env.ID,
		"latency_ms":     latencyMS,
	}
	return event
}

func NewAccountLoginEvent(acct Account) AuditEvent {
	event := newAuditEvent(AuditEventAccountLogin, acct.OrganizationID)
	event.AccountID = stringPtr(acct.ID)
	event.Data = AuditEventData{"email": acct.Email}
	return event
}

func NewAccountLoginFailedEvent(acct Account, emailAttempted string) AuditEvent {
	event := newAuditEvent(AuditEventAccountLoginFailed, acct.OrganizationID)
	event.AccountID = stringPtr(acct.ID)
	event.Data = AuditEventData{"email_attempted": emailAttempted}
	return event
}

func NewAccountLogoutEvent(acct Account) AuditEvent {
	event := newAuditEvent(AuditEventAccountLogout, acct.OrganizationID)
	event.AccountID = stringPtr(acct.ID)
	event.Data = AuditEventData{"email": acct.Email}
	return event
}

func sessionAuditEvent(eventType AuditEventType, sess Session, organizationID, accountID string, data AuditEventData) AuditEvent {
	event := newAuditEvent(eventType, organizationID)
	event.AccountID = stringPtr(accountID)
	event.WorkgroupID = stringPtr(sess.WorkgroupID)
	event.SessionID = stringPtr(sess.ID)
	event.AdvertisementID = stringPtr(sess.AdvertisementID)
	return withData(event, data)
}

func newAuditEvent(eventType AuditEventType, organizationID string) AuditEvent {
	return AuditEvent{
		OccurredAt:     time.Now().UTC(),
		EventType:      eventType,
		OrganizationID: organizationID,
		Data:           AuditEventData{},
	}
}

func withData(event AuditEvent, data AuditEventData) AuditEvent {
	if data == nil {
		event.Data = AuditEventData{}
		return event
	}
	event.Data = data
	return event
}

func stringPtr(v string) *string {
	return &v
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	return stringPtr(*v)
}
