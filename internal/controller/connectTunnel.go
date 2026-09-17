package controller

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/fabric/openziti/automation"
	"github.com/openziti/agora/internal/persistence"
)

func (s *Service) ConnectTunnel(ctx context.Context, req *api.ConnectTunnelRequest) (api.ConnectTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized connect tunnel request name='%s'", req.Name)
		return &api.ConnectTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	attachmentKind, listenAddress, err := inferConnectAttachmentKind(req.ListenAddress)
	if err != nil {
		return &api.ConnectTunnelConflict{Code: "conflict", Message: err.Error()}, nil
	}
	listenAddressValue := ""
	if listenAddress != nil {
		listenAddressValue = *listenAddress
	}
	dl.Infof("connecting tunnel name='%s' environment_id='%s' attachment_kind='%s' listen_address='%s' %s", req.Name, req.EnvironmentId, attachmentKind, listenAddressValue, principalLogFields(principal))

	attachmentID := persistence.NewResourceID(persistence.PrefixAttachment)
	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	var env *persistence.Environment
	var tunnel *persistence.Tunnel
	var attachment *persistence.TunnelAttachment
	var dialPolicyID string
	var dialPolicyCreated bool
	var compensationAttachment *persistence.TunnelAttachment
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := lockEnvironmentScope(ctx, tx, req.EnvironmentId); err != nil {
			return err
		}

		var err error
		env, err = s.requireOwnedEnvironmentWithQuery(ctx, tx, principal, req.EnvironmentId)
		if err != nil {
			return err
		}
		tunnel, err = s.resolveConsumerTunnel(ctx, tx, principal, env, req.Name)
		if err != nil {
			return err
		}
		if err := lockTunnelScope(ctx, tx, tunnel.ID); err != nil {
			return err
		}
		tunnel, err = s.store.Tunnels.GetByID(ctx, tx, tunnel.ID)
		if err != nil {
			return err
		}
		allowed, err := s.canConnectToTunnel(ctx, tx, tunnel, env, principal)
		if err != nil {
			return err
		}
		if !allowed {
			return persistence.ErrNotFound
		}

		if attachmentKind == persistence.TunnelAttachmentKindDialer {
			if tunnel.Mode == persistence.TunnelModeUDP {
				return errUnsupportedDialerMode
			}
			existing, err := s.store.TunnelAttachments.GetActiveDialerByEnvironmentAndTunnel(ctx, tx, env.ID, tunnel.ID)
			if err == nil {
				attachment = existing
				return nil
			}
			if !errors.Is(err, persistence.ErrNotFound) {
				return err
			}
		}
		if tunnel.ZitiServiceID == nil {
			return errTunnelMissingServiceMetadata
		}
		if attachmentKind == persistence.TunnelAttachmentKindProxy {
			if priorAttachmentID, reclaimRequested := req.AttachmentId.Get(); reclaimRequested {
				existing, err := s.store.TunnelAttachments.GetReclaimableProxyByID(
					ctx,
					tx,
					priorAttachmentID,
					env.ID,
					tunnel.ID,
					principal.OrganizationID,
					principal.AccountID,
					*listenAddress,
				)
				if err == nil {
					compensationAttachment = existing
					dialPolicyID, dialPolicyCreated, err = tunnelLifecycle.EnsureAttachmentDialPolicy(ctx, automation.TunnelAccessSpec{
						OrganizationID:        principal.OrganizationID,
						AccountID:             principal.AccountID,
						EnvironmentID:         env.ID,
						TunnelID:              tunnel.ID,
						TunnelName:            tunnel.Name,
						AttachmentID:          existing.ID,
						EnvironmentIdentityID: env.ZitiIdentityID,
						ServiceID:             *tunnel.ZitiServiceID,
						Version:               automation.DefaultAgoraVersion,
					})
					if err != nil {
						dl.Errorf("connect tunnel dial policy reconciliation failed tunnel_id='%s' attachment_id='%s' %s error='%v'", tunnel.ID, existing.ID, principalLogFields(principal), err)
						return err
					}
					heartbeatAt := time.Now().UTC()
					if err := s.store.TunnelAttachments.HeartbeatWithDialPolicy(ctx, tx, existing.ID, principal.OrganizationID, principal.AccountID, dialPolicyID, heartbeatAt); err != nil {
						return err
					}
					restored := *existing
					restored.DialPolicyID = &dialPolicyID
					restored.State = persistence.TunnelAttachmentStateActive
					restored.LastHeartbeatAt = heartbeatAt
					restored.DisconnectedAt = nil
					attachment = &restored
					return nil
				}
				if !errors.Is(err, persistence.ErrNotFound) {
					return err
				}
			}
		}

		dialPolicyID, err = tunnelLifecycle.CreateAttachmentDialPolicy(ctx, automation.TunnelAccessSpec{
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
			dl.Errorf("connect tunnel dial policy creation failed tunnel_id='%s' attachment_id='%s' %s error='%v'", tunnel.ID, attachmentID, principalLogFields(principal), err)
			return err
		}
		dialPolicyCreated = true
		candidate := persistence.TunnelAttachment{
			ID:              attachmentID,
			TunnelID:        tunnel.ID,
			OrganizationID:  principal.OrganizationID,
			AccountID:       principal.AccountID,
			EnvironmentID:   env.ID,
			Kind:            attachmentKind,
			ListenAddress:   listenAddress,
			DialPolicyID:    &dialPolicyID,
			State:           persistence.TunnelAttachmentStateActive,
			LastHeartbeatAt: time.Now().UTC(),
		}
		compensationAttachment = &candidate
		attachment, err = s.attachTunnel(ctx, tx, candidate)
		return err
	}); err != nil {
		if dialPolicyCreated && dialPolicyID != "" && compensationAttachment != nil {
			if compensationErr := s.compensateCreatedAttachmentDialPolicy(ctx, tunnelLifecycle, compensationAttachment, dialPolicyID); compensationErr != nil {
				err = errors.Join(err, compensationErr)
			}
		}
		if errors.Is(err, persistence.ErrNotFound) {
			if env == nil {
				return &api.ConnectTunnelNotFound{Code: "not_found", Message: "environment not found"}, nil
			}
			return &api.ConnectTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		if errors.Is(err, errTunnelReferenceAmbiguous) {
			return &api.ConnectTunnelConflict{Code: "conflict", Message: "tunnel name is ambiguous"}, nil
		}
		if errors.Is(err, errUnsupportedDialerMode) {
			return &api.ConnectTunnelConflict{Code: "conflict", Message: "dialer attachments do not support udp tunnels"}, nil
		}
		if errors.Is(err, errTunnelMissingServiceMetadata) {
			return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: "tunnel is missing service metadata"}, nil
		}
		if attachmentKind == persistence.TunnelAttachmentKindDialer && isUniqueViolation(err) {
			existing, lookupErr := s.store.TunnelAttachments.GetActiveDialerByEnvironmentAndTunnel(ctx, s.store.DB(), env.ID, tunnel.ID)
			if lookupErr == nil {
				resp := &api.ConnectTunnelResponse{
					Tunnel:     *mapTunnel(tunnel),
					Attachment: *mapTunnelAttachment(existing),
				}
				return resp, nil
			}
			if !errors.Is(lookupErr, persistence.ErrNotFound) {
				return &api.ConnectTunnelInternalServerError{Code: "internal_error", Message: lookupErr.Error()}, nil
			}
		}
		return &api.ConnectTunnelConflict{Code: "conflict", Message: err.Error()}, nil
	}

	resp := &api.ConnectTunnelResponse{
		Tunnel:     *mapTunnel(tunnel),
		Attachment: *mapTunnelAttachment(attachment),
	}
	dl.Infof("connected tunnel tunnel_id='%s' attachment_id='%s' environment_id='%s' %s", tunnel.ID, attachment.ID, env.ID, principalLogFields(principal))
	return resp, nil
}

func inferConnectAttachmentKind(listenAddress api.OptString) (persistence.TunnelAttachmentKind, *string, error) {
	value, ok := listenAddress.Get()
	if !ok {
		return persistence.TunnelAttachmentKindDialer, nil, nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil, errors.New("listenAddress must be omitted or non-empty")
	}
	return persistence.TunnelAttachmentKindProxy, &value, nil
}

var (
	errUnsupportedDialerMode        = errors.New("unsupported dialer mode")
	errTunnelMissingServiceMetadata = errors.New("tunnel missing service metadata")
)
