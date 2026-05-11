package tunnel

import (
	"context"

	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/networkpb"
)

const ensureConnectedOp = "tunnel.EnsureConnected"
const removeConnectOp = "tunnel.RemoveConnect"
const listConnectsOp = "tunnel.ListConnects"

// EnsureConnected ensures a connect actor exists for spec.Name and
// spec.ListenAddress. Existing matching actors are stopped and
// replaced by the embedded runtime.
func EnsureConnected(ctx context.Context, a *agent.Agent, spec ConnectSpec) (*ConnectStatus, error) {
	rt, err := runtimeFromAgent(ensureConnectedOp, a)
	if err != nil {
		return nil, err
	}
	return ensureConnected(ctx, rt, spec)
}

func ensureConnected(ctx context.Context, rt runtimeClient, spec ConnectSpec) (*ConnectStatus, error) {
	if rt == nil {
		return nil, runtimeMissing(ensureConnectedOp)
	}
	spec = normalizeConnectSpec(spec)
	if err := validateConnectSpec(spec); err != nil {
		return nil, err
	}

	res, err := rt.EnsureConnect(ctx, &networkpb.EnsureConnectRequest{
		Name:          spec.Name,
		ListenAddress: spec.ListenAddress,
	})
	if err != nil {
		return nil, transientRuntimeError(ensureConnectedOp, err)
	}
	if res == nil || res.Connect == nil {
		return nil, transientRuntimeMessage(ensureConnectedOp, "empty runtime response")
	}
	status := fromProtoConnectStatus(res.Connect)
	if status.State == StateError {
		return nil, transientRuntimeMessage(ensureConnectedOp, status.LastError)
	}
	return &status, nil
}

// RemoveConnect removes the connect actor with the given name and
// listen address. Missing actors are treated as successful removal.
func RemoveConnect(ctx context.Context, a *agent.Agent, name, listenAddress string) error {
	rt, err := runtimeFromAgent(removeConnectOp, a)
	if err != nil {
		return err
	}
	return removeConnect(ctx, rt, name, listenAddress)
}

func removeConnect(ctx context.Context, rt runtimeClient, name, listenAddress string) error {
	if rt == nil {
		return runtimeMissing(removeConnectOp)
	}
	spec := normalizeConnectSpec(ConnectSpec{Name: name, ListenAddress: listenAddress})
	if err := validateConnectSpec(spec); err != nil {
		return err
	}
	if _, err := rt.RemoveConnect(ctx, &networkpb.RemoveConnectRequest{
		Name:          spec.Name,
		ListenAddress: spec.ListenAddress,
	}); err != nil {
		if isManagedActorNotFound(err) {
			return nil
		}
		return transientRuntimeError(removeConnectOp, err)
	}
	return nil
}

// ListConnects returns every connect actor the runtime is managing.
func ListConnects(ctx context.Context, a *agent.Agent) ([]ConnectStatus, error) {
	rt, err := runtimeFromAgent(listConnectsOp, a)
	if err != nil {
		return nil, err
	}
	return listConnects(ctx, rt)
}

func listConnects(ctx context.Context, rt runtimeClient) ([]ConnectStatus, error) {
	if rt == nil {
		return nil, runtimeMissing(listConnectsOp)
	}
	res, err := rt.ListConnects(ctx, &networkpb.ListConnectsRequest{})
	if err != nil {
		return nil, transientRuntimeError(listConnectsOp, err)
	}
	if res == nil {
		return nil, transientRuntimeMessage(listConnectsOp, "empty runtime response")
	}
	out := make([]ConnectStatus, 0, len(res.Connects))
	for _, connect := range res.Connects {
		out = append(out, fromProtoConnectStatus(connect))
	}
	return out, nil
}
