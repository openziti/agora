package controller

import (
	"context"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListAuditEvents(ctx context.Context, params api.ListAuditEventsParams) (api.ListAuditEventsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		return &api.ListAuditEventsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	var cursor *persistence.AuditEventCursor
	if params.Cursor.Set && params.Cursor.Value != "" {
		decoded, err := persistence.DecodeAuditEventCursor(params.Cursor.Value)
		if err != nil {
			return &api.ListAuditEventsBadRequest{Code: "invalid_request", Message: "cursor is no longer valid; restart the audit event query"}, nil
		}
		cursor = decoded
	}

	if params.From.Set && params.To.Set && !params.From.Value.Before(params.To.Value) {
		return &api.ListAuditEventsBadRequest{Code: "invalid_request", Message: "from must be before to"}, nil
	}

	limit := 0
	if params.Limit.Set {
		if params.Limit.Value > 200 {
			return &api.ListAuditEventsBadRequest{Code: "invalid_request", Message: "limit must be 200 or less"}, nil
		}
		limit = params.Limit.Value
	}

	eventTypes := make([]persistence.AuditEventType, 0, len(params.EventType))
	for _, eventType := range params.EventType {
		eventTypes = append(eventTypes, persistence.AuditEventType(eventType))
	}

	listParams := persistence.AuditEventListParams{
		OrganizationID: principal.OrganizationID,
		EventTypes:     eventTypes,
		WorkgroupID:    optString(params.WorkgroupId),
		AccountID:      optString(params.AccountId),
		Cursor:         cursor,
		Limit:          limit,
	}
	if params.From.Set {
		listParams.From = &params.From.Value
	}
	if params.To.Set {
		listParams.To = &params.To.Value
	}

	rows, nextCursor, err := s.store.AuditEvents.List(ctx, s.store.DB(), listParams)
	if err != nil {
		dl.Errorf("list audit events failed %s: %v", principalLogFields(principal), err)
		return &api.ListAuditEventsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	resp := &api.ListAuditEventsResponse{Items: make([]api.AuditEvent, 0, len(rows))}
	for i := range rows {
		resp.Items = append(resp.Items, *mapAuditEvent(&rows[i]))
	}
	if nextCursor != "" {
		resp.NextCursor.SetTo(nextCursor)
	}
	return resp, nil
}
