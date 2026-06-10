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

func (s *Service) StartTunnelServe(ctx context.Context, req *api.StartTunnelServeRequest, params api.StartTunnelServeParams) (api.StartTunnelServeRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized start tunnel serve request tunnel_id='%s' environment_id='%s'", params.TunnelId, req.EnvironmentId)
		return &api.StartTunnelServeUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Infof("starting tunnel serve tunnel_id='%s' environment_id='%s' %s", params.TunnelId, req.EnvironmentId, principalLogFields(principal))

	tunnel, err := s.requireManagedTunnel(ctx, principal, params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("start tunnel serve tunnel not found tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
			return &api.StartTunnelServeNotFound{Code: "not_found", Message: "tunnel not found"}, nil
		}
		dl.Errorf("start tunnel serve tunnel lookup failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.StartTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if tunnel.Kind != persistence.TunnelKindProxy {
		dl.Warnf("start tunnel serve rejected for non-proxy tunnel tunnel_id='%s' kind='%s' %s", tunnel.ID, tunnel.Kind, principalLogFields(principal))
		return &api.StartTunnelServeConflict{
			Code:    "conflict",
			Message: fmt.Sprintf("tunnel '%s' is a direct listener tunnel", tunnel.Name),
		}, nil
	}

	env, err := s.requireOwnedEnvironment(ctx, principal, req.EnvironmentId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("start tunnel serve environment not found environment_id='%s' %s", req.EnvironmentId, principalLogFields(principal))
			return &api.StartTunnelServeNotFound{Code: "not_found", Message: "environment not found"}, nil
		}
		dl.Errorf("start tunnel serve environment lookup failed environment_id='%s' %s: %v", req.EnvironmentId, principalLogFields(principal), err)
		return &api.StartTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	if tunnel.EnvironmentID != env.ID {
		dl.Warnf(
			"start tunnel serve rejected due to environment mismatch tunnel_id='%s' tunnel_environment_id='%s' requested_environment_id='%s' %s",
			tunnel.ID,
			tunnel.EnvironmentID,
			env.ID,
			principalLogFields(principal),
		)
		return &api.StartTunnelServeConflict{
			Code:    "conflict",
			Message: fmt.Sprintf("tunnel '%s' is bound to environment '%s'", tunnel.Name, tunnel.EnvironmentID),
		}, nil
	}

	activeServe, err := s.store.TunnelServes.GetActiveByTunnel(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID)
	if err == nil {
		message := describeActiveTunnelServeConflict(activeServe)
		dl.Warnf("start tunnel serve rejected due to existing active serve tunnel_id='%s' active_serve_id='%s' %s", tunnel.ID, activeServe.ID, principalLogFields(principal))
		return &api.StartTunnelServeConflict{Code: "conflict", Message: message}, nil
	}
	if !errors.Is(err, persistence.ErrNotFound) {
		dl.Errorf("start tunnel serve active-serve lookup failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
		return &api.StartTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	created, err := s.store.TunnelServes.Create(ctx, s.store.DB(), persistence.TunnelServe{
		TunnelID:        tunnel.ID,
		OrganizationID:  principal.OrganizationID,
		AccountID:       principal.AccountID,
		EnvironmentID:   env.ID,
		State:           persistence.TunnelServeStateActive,
		LastHeartbeatAt: time.Now().UTC(),
	})
	if err != nil {
		activeServe, activeErr := s.store.TunnelServes.GetActiveByTunnel(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID)
		if activeErr == nil {
			message := describeActiveTunnelServeConflict(activeServe)
			dl.Warnf("start tunnel serve lost race to existing active serve tunnel_id='%s' active_serve_id='%s' %s", tunnel.ID, activeServe.ID, principalLogFields(principal))
			return &api.StartTunnelServeConflict{Code: "conflict", Message: message}, nil
		}
		dl.Errorf("start tunnel serve create failed tunnel_id='%s' environment_id='%s' %s: %v", tunnel.ID, env.ID, principalLogFields(principal), err)
		return &api.StartTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	detail := &persistence.TunnelServeDetail{
		TunnelServe:     *created,
		EnvironmentHost: env.Host,
		TunnelName:      tunnel.Name,
		TunnelMode:      string(tunnel.Mode),
	}
	dl.Infof("started tunnel serve tunnel_id='%s' serve_id='%s' environment_id='%s' %s", tunnel.ID, created.ID, env.ID, principalLogFields(principal))
	return mapTunnelServe(detail), nil
}

func (s *Service) GetTunnelServe(ctx context.Context, params api.GetTunnelServeParams) (api.GetTunnelServeRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized get tunnel serve request tunnel_id='%s'", params.TunnelId)
		return &api.GetTunnelServeUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}
	dl.Debugf("getting tunnel serve tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))

	tunnel, err := s.requireManagedTunnel(ctx, principal, params.TunnelId)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Warnf("get tunnel serve tunnel not found tunnel_id='%s' %s", params.TunnelId, principalLogFields(principal))
			return &api.GetTunnelServeNotFound{Code: "not_found", Message: "tunnel serve not found"}, nil
		}
		dl.Errorf("get tunnel serve tunnel lookup failed tunnel_id='%s' %s: %v", params.TunnelId, principalLogFields(principal), err)
		return &api.GetTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	serve, err := s.store.TunnelServes.GetActiveByTunnel(ctx, s.store.DB(), tunnel.ID, tunnel.OrganizationID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			dl.Debugf("get tunnel serve no active serve tunnel_id='%s' %s", tunnel.ID, principalLogFields(principal))
			return &api.GetTunnelServeNotFound{Code: "not_found", Message: "tunnel serve not found"}, nil
		}
		dl.Errorf("get tunnel serve failed tunnel_id='%s' %s: %v", tunnel.ID, principalLogFields(principal), err)
		return &api.GetTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Debugf("got tunnel serve tunnel_id='%s' serve_id='%s' %s", tunnel.ID, serve.ID, principalLogFields(principal))
	return mapTunnelServe(serve), nil
}

func (s *Service) HeartbeatTunnelServe(ctx context.Context, params api.HeartbeatTunnelServeParams) (api.HeartbeatTunnelServeRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized tunnel serve heartbeat serve_id='%s'", params.ServeId)
		return &api.HeartbeatTunnelServeUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	if _, err := s.store.TunnelServes.GetByIDForAccount(ctx, s.store.DB(), params.ServeId, principal.OrganizationID, principal.AccountID); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.HeartbeatTunnelServeNotFound{Code: "not_found", Message: "tunnel serve not found"}, nil
		}
		return &api.HeartbeatTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}
	if err := s.store.TunnelServes.Heartbeat(ctx, s.store.DB(), params.ServeId, principal.OrganizationID, principal.AccountID, time.Now().UTC()); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.HeartbeatTunnelServeNotFound{Code: "not_found", Message: "tunnel serve not found"}, nil
		}
		return &api.HeartbeatTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	return &api.HeartbeatTunnelServeNoContent{}, nil
}

func (s *Service) DeleteTunnelServe(ctx context.Context, params api.DeleteTunnelServeParams) (api.DeleteTunnelServeRes, error) {
	principal, err := requireAccountPrincipal(ctx)
	if err != nil {
		dl.Warnf("unauthorized delete tunnel serve request serve_id='%s'", params.ServeId)
		return &api.DeleteTunnelServeUnauthorized{Code: "unauthorized", Message: "unauthorized"}, nil
	}

	serve, err := s.store.TunnelServes.GetByIDForAccount(ctx, s.store.DB(), params.ServeId, principal.OrganizationID, principal.AccountID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteTunnelServeNotFound{Code: "not_found", Message: "tunnel serve not found"}, nil
		}
		return &api.DeleteTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	disconnectedAt := time.Now().UTC()
	if err := s.store.WithTx(ctx, func(tx persistence.Queryer) error {
		if err := s.store.TunnelServes.UpdateState(ctx, tx, serve.ID, persistence.TunnelServeStateDisconnected, &disconnectedAt); err != nil {
			return err
		}
		return s.store.TunnelServes.Delete(ctx, tx, serve.ID, principal.OrganizationID, principal.AccountID)
	}); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return &api.DeleteTunnelServeNotFound{Code: "not_found", Message: "tunnel serve not found"}, nil
		}
		return &api.DeleteTunnelServeInternalServerError{Code: "internal_error", Message: err.Error()}, nil
	}

	dl.Infof("deleted tunnel serve serve_id='%s' tunnel_id='%s' %s", serve.ID, serve.TunnelID, principalLogFields(principal))
	return &api.DeleteTunnelServeNoContent{}, nil
}

func describeActiveTunnelServeConflict(serve *persistence.TunnelServeDetail) string {
	message := fmt.Sprintf(
		"tunnel '%s' is already being served by environment '%s'",
		serve.TunnelName,
		serve.EnvironmentID,
	)
	if serve.EnvironmentHost != nil && *serve.EnvironmentHost != "" {
		message += fmt.Sprintf(" host '%s'", *serve.EnvironmentHost)
	}
	message += fmt.Sprintf(" (last heartbeat %s)", serve.LastHeartbeatAt.UTC().Format(time.RFC3339))
	return message
}
