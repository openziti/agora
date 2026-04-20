package agent

import (
	"context"
	"time"

	"github.com/openziti/agora/environment/env_core"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type managedServe struct {
	desired       env_core.ManagedServe
	env           *env_core.Environment
	state         networkpb.RuntimeState
	tunnelID      string
	serveID       string
	lastError     string
	lastStartedAt *time.Time
	cancel        context.CancelFunc
}

type managedConnect struct {
	desired       env_core.ManagedConnect
	env           *env_core.Environment
	state         networkpb.RuntimeState
	tunnelID      string
	attachmentID  string
	lastError     string
	lastStartedAt *time.Time
	cancel        context.CancelFunc
}

func configuredServes(networkState *env_core.Network) map[string]*managedServe {
	serves := make(map[string]*managedServe, len(networkState.Serves))
	for _, desired := range networkState.Serves {
		serves[serveKey(desired)] = &managedServe{
			desired:  desired,
			state:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
			tunnelID: desired.TunnelID,
		}
	}
	return serves
}

func configuredConnects(networkState *env_core.Network) map[string]*managedConnect {
	connects := make(map[string]*managedConnect, len(networkState.Connects))
	for _, desired := range networkState.Connects {
		connects[connectKey(desired)] = &managedConnect{
			desired:  desired,
			state:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
			tunnelID: desired.TunnelID,
		}
	}
	return connects
}

func serveStatusProto(actor *managedServe) *networkpb.ManagedServeStatus {
	status := &networkpb.ManagedServeStatus{
		Desired: &networkpb.DesiredServe{
			TunnelId:      actor.desired.TunnelID,
			Name:          actor.desired.Name,
			Mode:          string(actor.desired.Mode),
			BackendTarget: actor.desired.BackendTarget,
			GrantEmails:   append([]string(nil), actor.desired.GrantEmails...),
		},
		State:     actor.state,
		TunnelId:  actor.tunnelID,
		ServeId:   actor.serveID,
		LastError: actor.lastError,
	}
	if actor.lastStartedAt != nil {
		status.LastStartedAt = timestamppb.New(actor.lastStartedAt.UTC())
	}
	return status
}

func connectStatusProto(actor *managedConnect) *networkpb.ManagedConnectStatus {
	status := &networkpb.ManagedConnectStatus{
		Desired: &networkpb.DesiredConnect{
			TunnelId:      actor.desired.TunnelID,
			Name:          actor.desired.Name,
			ListenAddress: actor.desired.ListenAddress,
		},
		State:        actor.state,
		TunnelId:     actor.tunnelID,
		AttachmentId: actor.attachmentID,
		LastError:    actor.lastError,
	}
	if actor.lastStartedAt != nil {
		status.LastStartedAt = timestamppb.New(actor.lastStartedAt.UTC())
	}
	return status
}
