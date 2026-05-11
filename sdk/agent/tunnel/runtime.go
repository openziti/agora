package tunnel

import (
	"context"

	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/networkpb"
)

type runtimeClient interface {
	EnsureServe(context.Context, *networkpb.EnsureServeRequest) (*networkpb.EnsureServeResponse, error)
	RemoveServe(context.Context, *networkpb.RemoveServeRequest) (*networkpb.RemoveServeResponse, error)
	ListServes(context.Context, *networkpb.ListServesRequest) (*networkpb.ListServesResponse, error)
	EnsureConnect(context.Context, *networkpb.EnsureConnectRequest) (*networkpb.EnsureConnectResponse, error)
	RemoveConnect(context.Context, *networkpb.RemoveConnectRequest) (*networkpb.RemoveConnectResponse, error)
	ListConnects(context.Context, *networkpb.ListConnectsRequest) (*networkpb.ListConnectsResponse, error)
}

func runtimeFromAgent(op string, a *agent.Agent) (runtimeClient, error) {
	if a == nil {
		return nil, invalidSpec("%s: agent is required", op)
	}
	rt := a.Runtime()
	if rt == nil {
		return nil, runtimeMissing(op)
	}
	return rt, nil
}
