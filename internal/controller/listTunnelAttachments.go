package controller

import (
	"context"
	"errors"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ListTunnelAttachments(ctx context.Context, params api.ListTunnelAttachmentsParams) (api.ListTunnelAttachmentsRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized list tunnel attachments request tunnel_id='%s'", params.TunnelId)
		return &api.ListTunnelAttachmentsUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	tunnel, err := s.requireManagedTunnel(ctx, principal, params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ListTunnelAttachmentsNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		return &api.ListTunnelAttachmentsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	attachments, err := s.store.TunnelAttachments.ListByTunnel(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID)
	if err != nil {
		return &api.ListTunnelAttachmentsInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	resp := make(api.ListTunnelAttachmentsResponse, 0, len(attachments))
	for i := range attachments {
		resp = append(resp, *mapTunnelAttachmentDetail(&attachments[i]))
	}
	return &resp, nil
}
