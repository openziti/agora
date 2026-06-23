package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/persistence"
)

// TakeoverTunnel reclaims a standalone (account-owned) tunnel for a new host by evicting whatever is
// currently hosting it. Takeover is blunt and unconditional per spec: it displaces whatever it finds
// rather than judging whether the current host is healthy.
func (s *Service) TakeoverTunnel(ctx context.Context, params api.TakeoverTunnelParams) (api.TakeoverTunnelRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized takeover tunnel request tunnel_id='%s'", params.TunnelId)
		return &api.TakeoverTunnelUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("takeover tunnel tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))

	tunnel, err := s.requireManagedTunnel(ctx, principal, params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("takeover tunnel not found tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
			return &api.TakeoverTunnelNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		dl.Errorf("takeover tunnel lookup failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.TakeoverTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	// takeover applies only to account-owned standalone tunnels (environment_id NULL). a Layer 2 session
	// tunnel is environment-owned and session-scoped; reject before any fabric or DB mutation so the
	// session-tunnel path stays untouched (finding R2-C1).
	if tunnel.EnvironmentID != nil {
		dl.Warnf("takeover tunnel rejected for session tunnel tunnel_id='%s' environment_id='%s' %s", tunnel.ID, *tunnel.EnvironmentID, principalLogFields(principal))
		return &api.TakeoverTunnelConflict{
			Code:    "conflict",
			Message: fmt.Sprintf("tunnel '%s' is a session tunnel and cannot be taken over", tunnel.Name),
		}, nil
	}

	if err := s.evictTunnelHost(ctx, tunnel); err != nil {
		dl.Errorf("takeover tunnel eviction failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
		return &api.TakeoverTunnelInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Infof("took over tunnel tunnel_id='%s' name='%s' %s", tunnel.ID, tunnel.Name, principalLogFields(principal))
	return &api.TakeoverTunnelNoContent{}, nil
}

// evictTunnelHost is the shape-agnostic takeover eviction primitive. It removes whatever host is
// currently bound to the tunnel on the fabric (deleting its terminators), and clears the active serve
// record for a proxy tunnel. A direct tunnel keeps no serve record, so the terminator delete is the
// whole job for it. Blunt and unconditional per spec -- no health check.
func (s *Service) evictTunnelHost(ctx context.Context, tunnel *persistence.Tunnel) error {
	_, tunnelLifecycle, err := s.lifecycleFactory(ctx)
	if err != nil {
		return err
	}
	if tunnel.ZitiServiceID != nil {
		if err := tunnelLifecycle.EvictTerminatorsByService(ctx, *tunnel.ZitiServiceID); err != nil {
			return err
		}
	}

	serve, err := s.store.TunnelServes.GetActiveByTunnel(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return nil
		}
		return err
	}
	disconnectedAt := time.Now().UTC()
	return s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := s.store.TunnelServes.UpdateState(ctx, tx, serve.ID, persistence.TunnelServeStateDisconnected, &disconnectedAt); err != nil {
			return err
		}
		return s.store.TunnelServes.Delete(ctx, tx, serve.ID, serve.OrganizationID, serve.AccountID)
	})
}
