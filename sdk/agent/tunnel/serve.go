package tunnel

import (
	"context"
	"strings"

	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/networkpb"
)

const ensureServedOp = "tunnel.EnsureServed"
const removeServeOp = "tunnel.RemoveServe"
const listServesOp = "tunnel.ListServes"

// EnsureServed ensures a serve actor exists for spec.Name. Existing
// matching actors are stopped and replaced by the embedded runtime.
func EnsureServed(ctx context.Context, a *agent.Agent, spec ServeSpec) (*ServeStatus, error) {
	rt, err := runtimeFromAgent(ensureServedOp, a)
	if err != nil {
		return nil, err
	}
	return ensureServed(ctx, rt, spec)
}

func ensureServed(ctx context.Context, rt runtimeClient, spec ServeSpec) (*ServeStatus, error) {
	if rt == nil {
		return nil, runtimeMissing(ensureServedOp)
	}
	spec = normalizeServeSpec(spec)
	if err := validateServeSpec(spec); err != nil {
		return nil, err
	}

	res, err := rt.EnsureServe(ctx, &networkpb.EnsureServeRequest{
		Name:          spec.Name,
		Mode:          string(spec.Mode),
		BackendTarget: spec.BackendTarget,
		GrantEmails:   append([]string(nil), spec.GrantEmails...),
	})
	if err != nil {
		return nil, transientRuntimeError(ensureServedOp, err)
	}
	if res == nil || res.Serve == nil {
		return nil, transientRuntimeMessage(ensureServedOp, "empty runtime response")
	}
	status := fromProtoServeStatus(res.Serve)
	if status.State == StateError {
		return nil, transientRuntimeMessage(ensureServedOp, status.LastError)
	}
	return &status, nil
}

// RemoveServe removes the serve actor with the given name. Missing
// actors are treated as successful removal.
func RemoveServe(ctx context.Context, a *agent.Agent, name string) error {
	rt, err := runtimeFromAgent(removeServeOp, a)
	if err != nil {
		return err
	}
	return removeServe(ctx, rt, name)
}

func removeServe(ctx context.Context, rt runtimeClient, name string) error {
	if rt == nil {
		return runtimeMissing(removeServeOp)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return invalidSpec("serve name is required")
	}
	if _, err := rt.RemoveServe(ctx, &networkpb.RemoveServeRequest{Name: name}); err != nil {
		if isManagedActorNotFound(err) {
			return nil
		}
		return transientRuntimeError(removeServeOp, err)
	}
	return nil
}

// ListServes returns every serve actor the runtime is managing.
func ListServes(ctx context.Context, a *agent.Agent) ([]ServeStatus, error) {
	rt, err := runtimeFromAgent(listServesOp, a)
	if err != nil {
		return nil, err
	}
	return listServes(ctx, rt)
}

func listServes(ctx context.Context, rt runtimeClient) ([]ServeStatus, error) {
	if rt == nil {
		return nil, runtimeMissing(listServesOp)
	}
	res, err := rt.ListServes(ctx, &networkpb.ListServesRequest{})
	if err != nil {
		return nil, transientRuntimeError(listServesOp, err)
	}
	if res == nil {
		return nil, transientRuntimeMessage(listServesOp, "empty runtime response")
	}
	out := make([]ServeStatus, 0, len(res.Serves))
	for _, serve := range res.Serves {
		out = append(out, fromProtoServeStatus(serve))
	}
	return out, nil
}
