package tunnel

import (
	"context"
	"errors"
	"testing"

	"github.com/openziti/agora/sdk/agent/networkpb"
)

type fakeRuntime struct {
	ensureServe   func(context.Context, *networkpb.EnsureServeRequest) (*networkpb.EnsureServeResponse, error)
	removeServe   func(context.Context, *networkpb.RemoveServeRequest) (*networkpb.RemoveServeResponse, error)
	listServes    func(context.Context, *networkpb.ListServesRequest) (*networkpb.ListServesResponse, error)
	ensureConnect func(context.Context, *networkpb.EnsureConnectRequest) (*networkpb.EnsureConnectResponse, error)
	removeConnect func(context.Context, *networkpb.RemoveConnectRequest) (*networkpb.RemoveConnectResponse, error)
	listConnects  func(context.Context, *networkpb.ListConnectsRequest) (*networkpb.ListConnectsResponse, error)
}

func (f *fakeRuntime) EnsureServe(ctx context.Context, req *networkpb.EnsureServeRequest) (*networkpb.EnsureServeResponse, error) {
	if f.ensureServe == nil {
		panic("unexpected EnsureServe call")
	}
	return f.ensureServe(ctx, req)
}

func (f *fakeRuntime) RemoveServe(ctx context.Context, req *networkpb.RemoveServeRequest) (*networkpb.RemoveServeResponse, error) {
	if f.removeServe == nil {
		panic("unexpected RemoveServe call")
	}
	return f.removeServe(ctx, req)
}

func (f *fakeRuntime) ListServes(ctx context.Context, req *networkpb.ListServesRequest) (*networkpb.ListServesResponse, error) {
	if f.listServes == nil {
		panic("unexpected ListServes call")
	}
	return f.listServes(ctx, req)
}

func (f *fakeRuntime) EnsureConnect(ctx context.Context, req *networkpb.EnsureConnectRequest) (*networkpb.EnsureConnectResponse, error) {
	if f.ensureConnect == nil {
		panic("unexpected EnsureConnect call")
	}
	return f.ensureConnect(ctx, req)
}

func (f *fakeRuntime) RemoveConnect(ctx context.Context, req *networkpb.RemoveConnectRequest) (*networkpb.RemoveConnectResponse, error) {
	if f.removeConnect == nil {
		panic("unexpected RemoveConnect call")
	}
	return f.removeConnect(ctx, req)
}

func (f *fakeRuntime) ListConnects(ctx context.Context, req *networkpb.ListConnectsRequest) (*networkpb.ListConnectsResponse, error) {
	if f.listConnects == nil {
		panic("unexpected ListConnects call")
	}
	return f.listConnects(ctx, req)
}

type fakeManagedActorNotFound struct{}

func (fakeManagedActorNotFound) Error() string {
	return "managed serve not found"
}

func (fakeManagedActorNotFound) ManagedActorNotFound() bool {
	return true
}

type fakeUnsupportedMode struct{}

func (fakeUnsupportedMode) Error() string {
	return "unsupported tunnel mode 'smtp'"
}

func (fakeUnsupportedMode) UnsupportedTunnelMode() string {
	return "smtp"
}

func assertErrorIs(t *testing.T, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
