package tunnel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestEnsureConnectedSuccess(t *testing.T) {
	startedAt := time.Date(2026, 5, 11, 13, 0, 0, 0, time.UTC)
	fake := &fakeRuntime{
		ensureConnect: func(_ context.Context, req *networkpb.EnsureConnectRequest) (*networkpb.EnsureConnectResponse, error) {
			if req.Name != "llm-gateway" || req.ListenAddress != "127.0.0.1:9080" {
				t.Fatalf("unexpected request %#v", req)
			}
			return &networkpb.EnsureConnectResponse{Connect: &networkpb.ManagedConnectStatus{
				Desired: &networkpb.DesiredConnect{
					Name:          req.Name,
					ListenAddress: req.ListenAddress,
				},
				State:         networkpb.RuntimeState_RUNTIME_STATE_RUNNING,
				TunnelId:      "tt_abcdefghijkl",
				AttachmentId:  "ta_abcdefghijkl",
				LastStartedAt: timestamppb.New(startedAt),
			}}, nil
		},
	}

	got, err := ensureConnected(context.Background(), fake, ConnectSpec{
		Name:          " llm-gateway ",
		ListenAddress: " 127.0.0.1:9080 ",
	})
	if err != nil {
		t.Fatalf("ensure connected: %v", err)
	}
	if got.Name != "llm-gateway" || got.ListenAddress != "127.0.0.1:9080" || got.State != StateRunning {
		t.Fatalf("unexpected status %#v", got)
	}
	if got.TunnelID != "tt_abcdefghijkl" || got.AttachmentID != "ta_abcdefghijkl" || !got.LastStartedAt.Equal(startedAt) {
		t.Fatalf("unexpected identifiers/timestamp %#v", got)
	}
}

func TestEnsureConnectedValidation(t *testing.T) {
	tests := []struct {
		name string
		spec ConnectSpec
	}{
		{name: "missing name", spec: ConnectSpec{ListenAddress: "127.0.0.1:9080"}},
		{name: "missing listen", spec: ConnectSpec{Name: "gateway"}},
		{name: "listen port zero", spec: ConnectSpec{Name: "gateway", ListenAddress: "127.0.0.1:0"}},
		{name: "listen no host", spec: ConnectSpec{Name: "gateway", ListenAddress: ":9080"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ensureConnected(context.Background(), &fakeRuntime{}, tt.spec)
			assertErrorIs(t, err, ErrInvalidSpec)
		})
	}
}

func TestEnsureConnectedErrorMappings(t *testing.T) {
	tests := []struct {
		name string
		res  *networkpb.EnsureConnectResponse
		err  error
		want error
	}{
		{name: "runtime error", err: errors.New("dial failed"), want: ErrTransient},
		{name: "unsupported runtime mode", err: fakeUnsupportedMode{}, want: ErrUnsupportedMode},
		{name: "state error", res: &networkpb.EnsureConnectResponse{Connect: &networkpb.ManagedConnectStatus{
			Desired:   &networkpb.DesiredConnect{Name: "gateway", ListenAddress: "127.0.0.1:9080"},
			State:     networkpb.RuntimeState_RUNTIME_STATE_ERROR,
			LastError: "start failed",
		}}, want: ErrTransient},
		{name: "empty response", res: nil, want: ErrTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRuntime{ensureConnect: func(context.Context, *networkpb.EnsureConnectRequest) (*networkpb.EnsureConnectResponse, error) {
				return tt.res, tt.err
			}}
			_, err := ensureConnected(context.Background(), fake, ConnectSpec{Name: "gateway", ListenAddress: "127.0.0.1:9080"})
			assertErrorIs(t, err, tt.want)
		})
	}
}

func TestRemoveConnectIdempotentAndErrors(t *testing.T) {
	fake := &fakeRuntime{removeConnect: func(_ context.Context, req *networkpb.RemoveConnectRequest) (*networkpb.RemoveConnectResponse, error) {
		if req.Name != "gateway" || req.ListenAddress != "127.0.0.1:9080" {
			t.Fatalf("unexpected request %#v", req)
		}
		return nil, fakeManagedActorNotFound{}
	}}
	if err := removeConnect(context.Background(), fake, " gateway ", " 127.0.0.1:9080 "); err != nil {
		t.Fatalf("expected missing connect to succeed, got %v", err)
	}

	errFake := &fakeRuntime{removeConnect: func(context.Context, *networkpb.RemoveConnectRequest) (*networkpb.RemoveConnectResponse, error) {
		return nil, errors.New("disk failed")
	}}
	assertErrorIs(t, removeConnect(context.Background(), errFake, "gateway", "127.0.0.1:9080"), ErrTransient)
	assertErrorIs(t, removeConnect(context.Background(), fake, "gateway", "127.0.0.1:0"), ErrInvalidSpec)
}

func TestListConnects(t *testing.T) {
	fake := &fakeRuntime{listConnects: func(context.Context, *networkpb.ListConnectsRequest) (*networkpb.ListConnectsResponse, error) {
		return &networkpb.ListConnectsResponse{Connects: []*networkpb.ManagedConnectStatus{{
			Desired: &networkpb.DesiredConnect{
				Name:          "gateway",
				ListenAddress: "127.0.0.1:9080",
			},
			State:        networkpb.RuntimeState_RUNTIME_STATE_STARTING,
			TunnelId:     "tt_abcdefghijkl",
			AttachmentId: "ta_abcdefghijkl",
			RetryAttempt: 1,
		}}}, nil
	}}
	got, err := listConnects(context.Background(), fake)
	if err != nil {
		t.Fatalf("list connects: %v", err)
	}
	if len(got) != 1 || got[0].State != StateStarting || got[0].RetryAttempt != 1 || got[0].AttachmentID != "ta_abcdefghijkl" {
		t.Fatalf("unexpected connects %#v", got)
	}
}
