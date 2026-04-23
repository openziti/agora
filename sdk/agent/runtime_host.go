package agent

import (
	"context"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/network/tunnelruntime"
)

type runtimeHandle interface {
	Done() <-chan error
}

type tunnelRuntimeHost interface {
	StartServe(context.Context, string, *api.Tunnel) (runtimeHandle, error)
	StartConnect(context.Context, string, *api.Tunnel, string) (runtimeHandle, error)
}

type defaultRuntimeHost struct{}

func (defaultRuntimeHost) StartServe(ctx context.Context, identityPath string, tunnel *api.Tunnel) (runtimeHandle, error) {
	factory := tunnelruntime.OpenZitiFactory{}
	switch tunnel.Mode {
	case api.TunnelModeHTTP:
		return tunnelruntime.StartServeHTTP(ctx, factory, identityPath, tunnel.ID, tunnel.BackendTarget)
	case api.TunnelModeTCP:
		return tunnelruntime.StartServeTCP(ctx, factory, identityPath, tunnel.ID, tunnel.BackendTarget)
	case api.TunnelModeUDP:
		return tunnelruntime.StartServeUDP(ctx, factory, identityPath, tunnel.ID, tunnel.BackendTarget)
	default:
		return nil, unsupportedTunnelMode(tunnel.Mode)
	}
}

func (defaultRuntimeHost) StartConnect(ctx context.Context, identityPath string, tunnel *api.Tunnel, listenAddress string) (runtimeHandle, error) {
	factory := tunnelruntime.OpenZitiFactory{}
	switch tunnel.Mode {
	case api.TunnelModeHTTP:
		return tunnelruntime.StartConnectHTTP(ctx, factory, identityPath, tunnel.ID, listenAddress)
	case api.TunnelModeTCP:
		return tunnelruntime.StartConnectTCP(ctx, factory, identityPath, tunnel.ID, listenAddress)
	case api.TunnelModeUDP:
		return tunnelruntime.StartConnectUDP(ctx, factory, identityPath, tunnel.ID, listenAddress)
	default:
		return nil, unsupportedTunnelMode(tunnel.Mode)
	}
}

func unsupportedTunnelMode(mode api.TunnelMode) error {
	return &unsupportedTunnelModeError{mode: string(mode)}
}

type unsupportedTunnelModeError struct {
	mode string
}

func (e *unsupportedTunnelModeError) Error() string {
	return "unsupported tunnel mode '" + e.mode + "'"
}
