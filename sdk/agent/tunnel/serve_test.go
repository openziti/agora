package tunnel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestEnsureServedSuccess(t *testing.T) {
	startedAt := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	fake := &fakeRuntime{
		ensureServe: func(_ context.Context, req *networkpb.EnsureServeRequest) (*networkpb.EnsureServeResponse, error) {
			if req.Name != "llm-gateway" || req.Mode != "http" || req.BackendTarget != "http://127.0.0.1:8080" {
				t.Fatalf("unexpected request %#v", req)
			}
			if len(req.GrantEmails) != 1 || req.GrantEmails[0] != "alice@example.com" {
				t.Fatalf("unexpected grants %#v", req.GrantEmails)
			}
			return &networkpb.EnsureServeResponse{Serve: &networkpb.ManagedServeStatus{
				Desired: &networkpb.DesiredServe{
					Name:          req.Name,
					Mode:          req.Mode,
					BackendTarget: req.BackendTarget,
				},
				State:         networkpb.RuntimeState_RUNTIME_STATE_RUNNING,
				TunnelId:      "tt_abcdefghijkl",
				ServeId:       "ts_abcdefghijkl",
				LastStartedAt: timestamppb.New(startedAt),
			}}, nil
		},
	}

	got, err := ensureServed(context.Background(), fake, ServeSpec{
		Name:          " llm-gateway ",
		Mode:          ModeHTTP,
		BackendTarget: " http://127.0.0.1:8080 ",
		GrantEmails:   []string{"", " alice@example.com "},
	})
	if err != nil {
		t.Fatalf("ensure served: %v", err)
	}
	if got.Name != "llm-gateway" || got.Mode != ModeHTTP || got.State != StateRunning {
		t.Fatalf("unexpected status %#v", got)
	}
	if got.TunnelID != "tt_abcdefghijkl" || got.ServeID != "ts_abcdefghijkl" || !got.LastStartedAt.Equal(startedAt) {
		t.Fatalf("unexpected identifiers/timestamp %#v", got)
	}
}

func TestEnsureServedValidation(t *testing.T) {
	tests := []struct {
		name string
		spec ServeSpec
		want error
	}{
		{name: "missing name", spec: ServeSpec{Mode: ModeTCP, BackendTarget: "127.0.0.1:8080"}, want: ErrInvalidSpec},
		{name: "missing mode", spec: ServeSpec{Name: "gateway", BackendTarget: "127.0.0.1:8080"}, want: ErrInvalidSpec},
		{name: "bad mode", spec: ServeSpec{Name: "gateway", Mode: Mode("smtp"), BackendTarget: "127.0.0.1:8080"}, want: ErrUnsupportedMode},
		{name: "bad http target", spec: ServeSpec{Name: "gateway", Mode: ModeHTTP, BackendTarget: "127.0.0.1:8080"}, want: ErrInvalidSpec},
		{name: "bad tcp target", spec: ServeSpec{Name: "gateway", Mode: ModeTCP, BackendTarget: "127.0.0.1"}, want: ErrInvalidSpec},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ensureServed(context.Background(), &fakeRuntime{}, tt.spec)
			assertErrorIs(t, err, tt.want)
		})
	}
}

func TestEnsureServedErrorMappings(t *testing.T) {
	tests := []struct {
		name string
		res  *networkpb.EnsureServeResponse
		err  error
		want error
	}{
		{name: "runtime error", err: errors.New("dial failed"), want: ErrTransient},
		{name: "unsupported runtime mode", err: fakeUnsupportedMode{}, want: ErrUnsupportedMode},
		{name: "state error", res: &networkpb.EnsureServeResponse{Serve: &networkpb.ManagedServeStatus{
			Desired:   &networkpb.DesiredServe{Name: "gateway", Mode: "tcp", BackendTarget: "127.0.0.1:8080"},
			State:     networkpb.RuntimeState_RUNTIME_STATE_ERROR,
			LastError: "start failed",
		}}, want: ErrTransient},
		{name: "empty response", res: nil, want: ErrTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRuntime{ensureServe: func(context.Context, *networkpb.EnsureServeRequest) (*networkpb.EnsureServeResponse, error) {
				return tt.res, tt.err
			}}
			_, err := ensureServed(context.Background(), fake, ServeSpec{Name: "gateway", Mode: ModeTCP, BackendTarget: "127.0.0.1:8080"})
			assertErrorIs(t, err, tt.want)
		})
	}
}

func TestRemoveServeIdempotentAndErrors(t *testing.T) {
	fake := &fakeRuntime{removeServe: func(_ context.Context, req *networkpb.RemoveServeRequest) (*networkpb.RemoveServeResponse, error) {
		if req.Name != "gateway" {
			t.Fatalf("unexpected request %#v", req)
		}
		return nil, fakeManagedActorNotFound{}
	}}
	if err := removeServe(context.Background(), fake, " gateway "); err != nil {
		t.Fatalf("expected missing serve to succeed, got %v", err)
	}

	errFake := &fakeRuntime{removeServe: func(context.Context, *networkpb.RemoveServeRequest) (*networkpb.RemoveServeResponse, error) {
		return nil, errors.New("disk failed")
	}}
	assertErrorIs(t, removeServe(context.Background(), errFake, "gateway"), ErrTransient)
	assertErrorIs(t, removeServe(context.Background(), fake, ""), ErrInvalidSpec)
}

func TestListServes(t *testing.T) {
	fake := &fakeRuntime{listServes: func(context.Context, *networkpb.ListServesRequest) (*networkpb.ListServesResponse, error) {
		return &networkpb.ListServesResponse{Serves: []*networkpb.ManagedServeStatus{{
			Desired: &networkpb.DesiredServe{
				Name:          "gateway",
				Mode:          "udp",
				BackendTarget: "127.0.0.1:9000",
			},
			State:        networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
			TunnelId:     "tt_abcdefghijkl",
			RetryAttempt: 2,
		}}}, nil
	}}
	got, err := listServes(context.Background(), fake)
	if err != nil {
		t.Fatalf("list serves: %v", err)
	}
	if len(got) != 1 || got[0].Mode != ModeUDP || got[0].State != StateConfigured || got[0].RetryAttempt != 2 {
		t.Fatalf("unexpected serves %#v", got)
	}
}
