package controller

import (
	"context"
	"errors"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ConnectTunnel(ctx context.Context, req *api.ConnectTunnelRequest) (api.ConnectTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized connect tunnel request name='%s'", req.Name)
		return &api.ConnectTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("connecting tunnel name='%s' environment_id='%s' listen_address='%s' %s", req.Name, req.EnvironmentId, req.ListenAddress, principalLogFields(principal))

	env, err := s.requireOwnedEnvironment(ctx, principal, req.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ConnectTunnelNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	tunnel, err := s.lookupTunnelByName(ctx, principal, req.Name)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.ConnectTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	allowed, err := s.canConnectToTunnel(ctx, tunnel, env, principal)
	if err != nil {
		return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if !allowed {
		dl.Warnf("connect tunnel rejected tunnel_id='%s' environment_id='%s' %s", tunnel.ID, env.ID, principalLogFields(principal))
		return &api.ConnectTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
	}

	if tunnel.ZitiServiceID == nil {
		return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: "tunnel is missing service metadata"}, nil
	}

	attachmentID := persistence.NewResourceID(persistence.PrefixAttachment)
	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dialPolicyID, err := tunnelLifecycle.CreateAttachmentDialPolicy(ctx, automation.TunnelAccessSpec{
		OrganizationID:        principal.OrganizationID,
		AccountID:             principal.AccountID,
		EnvironmentID:         env.ID,
		TunnelID:              tunnel.ID,
		TunnelName:            tunnel.Name,
		AttachmentID:          attachmentID,
		EnvironmentIdentityID: env.ZitiIdentityID,
		ServiceID:             *tunnel.ZitiServiceID,
		Version:               automation.DefaultAgoraVersion,
	})
	if err != nil {
		dl.Errorf("connect tunnel dial policy creation failed tunnel_id='%s' attachment_id='%s' %s: %v", tunnel.ID, attachmentID, principalLogFields(principal), err)
		return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	attachment, err := s.store.TunnelAttachments.Create(ctx, s.store.DB(), persistence.TunnelAttachment{
		ID:              attachmentID,
		TunnelID:        tunnel.ID,
		OrganizationID:  principal.OrganizationID,
		AccountID:       principal.AccountID,
		EnvironmentID:   env.ID,
		ListenAddress:   req.ListenAddress,
		DialPolicyID:    &dialPolicyID,
		State:           persistence.TunnelAttachmentStateActive,
		LastHeartbeatAt: time.Now().UTC(),
	})
	if err != nil {
		_ = tunnelLifecycle.Deprovision(ctx, automation.DeprovisionTunnelSpec{DialPolicyID: dialPolicyID})
		return &api.ConnectTunnelConflict{Code: "conflict", Message: err.Error()}, nil
	}

	resp := &api.ConnectTunnelResponse{
		Tunnel:     *mapTunnel(tunnel),
		Attachment: *mapTunnelAttachment(attachment),
	}
	dl.Infof("connected tunnel tunnel_id='%s' attachment_id='%s' environment_id='%s' %s", tunnel.ID, attachment.ID, env.ID, principalLogFields(principal))
	return resp, nil
}
