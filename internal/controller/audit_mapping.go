package controller

import (
	"encoding/json"

	"github.com/go-faster/jx"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func mapAuditEvent(event *persistence.AuditEvent) *api.AuditEvent {
	out := &api.AuditEvent{
		ID:             event.ID,
		OccurredAt:     event.OccurredAt,
		EventType:      api.AuditEventType(event.EventType),
		OrganizationId: event.OrganizationID,
		Data:           mapAuditEventData(event.Data),
	}
	if event.AccountID != nil {
		out.AccountId.SetTo(*event.AccountID)
	}
	if event.WorkgroupID != nil {
		out.WorkgroupId.SetTo(*event.WorkgroupID)
	}
	if event.SessionID != nil {
		out.SessionId.SetTo(*event.SessionID)
	}
	if event.AdvertisementID != nil {
		out.AdvertisementId.SetTo(*event.AdvertisementID)
	}
	if event.ContractID != nil {
		out.ContractId.SetTo(*event.ContractID)
	}
	if event.EnvelopeID != nil {
		out.EnvelopeId.SetTo(*event.EnvelopeID)
	}
	return out
}

func mapAuditEventData(data persistence.AuditEventData) api.AuditEventData {
	out := api.AuditEventData{}
	for key, value := range data {
		encoded, err := json.Marshal(value)
		if err != nil {
			encoded = []byte("null")
		}
		out[key] = jx.Raw(encoded)
	}
	return out
}
