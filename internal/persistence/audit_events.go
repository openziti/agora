package persistence

import (
	"context"
	"fmt"
)

type AuditEventsRepository struct{}

const auditEventColumns = `id, occurred_at, event_type, organization_id, account_id, workgroup_id, session_id, advertisement_id, contract_id, envelope_id, data`

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
