package tunnel

import (
	"strings"
	"time"

	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func fromProtoServeStatus(status *networkpb.ManagedServeStatus) ServeStatus {
	out := ServeStatus{}
	if status == nil {
		return out
	}
	if status.Desired != nil {
		out.Name = status.Desired.Name
		out.Mode = Mode(status.Desired.Mode)
		out.BackendTarget = status.Desired.BackendTarget
	}
	out.TunnelID = status.TunnelId
	out.ServeID = status.ServeId
	out.State = fromProtoState(status.State)
	out.LastError = status.LastError
	out.LastStartedAt = timestampTime(status.LastStartedAt)
	out.NextRetryAt = timestampTime(status.NextRetryAt)
	out.RetryAttempt = status.RetryAttempt
	return out
}

func fromProtoConnectStatus(status *networkpb.ManagedConnectStatus) ConnectStatus {
	out := ConnectStatus{}
	if status == nil {
		return out
	}
	if status.Desired != nil {
		out.Name = status.Desired.Name
		out.ListenAddress = status.Desired.ListenAddress
	}
	out.TunnelID = status.TunnelId
	out.AttachmentID = status.AttachmentId
	out.State = fromProtoState(status.State)
	out.LastError = status.LastError
	out.LastStartedAt = timestampTime(status.LastStartedAt)
	out.NextRetryAt = timestampTime(status.NextRetryAt)
	out.RetryAttempt = status.RetryAttempt
	return out
}

func fromProtoState(state networkpb.RuntimeState) State {
	switch state {
	case networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED:
		return StateConfigured
	case networkpb.RuntimeState_RUNTIME_STATE_STARTING:
		return StateStarting
	case networkpb.RuntimeState_RUNTIME_STATE_RUNNING:
		return StateRunning
	case networkpb.RuntimeState_RUNTIME_STATE_ERROR:
		return StateError
	case networkpb.RuntimeState_RUNTIME_STATE_STOPPED:
		return StateStopped
	default:
		return State(strings.ToLower(strings.TrimPrefix(state.String(), "RUNTIME_STATE_")))
	}
}

func timestampTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}
