package tunnel

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/network/tunnelruntime"
)

func TestCreateDirectTunnelOmitsBackendTarget(t *testing.T) {
	controller := &fakePrimitiveController{
		createTunnel: func(_ context.Context, req *api.CreateTunnelRequest) (api.CreateTunnelRes, error) {
			if req.EnvironmentId != "ev_test00000001" {
				t.Fatalf("unexpected environment id %q", req.EnvironmentId)
			}
			if req.Name != "gateway" || req.Mode != api.TunnelModeHTTP {
				t.Fatalf("unexpected create request %#v", req)
			}
			if req.BackendTarget.Set {
				t.Fatalf("direct create should omit backendTarget, got %#v", req.BackendTarget)
			}
			if len(req.GrantEmails) != 1 || req.GrantEmails[0] != "alice@example.com" {
				t.Fatalf("unexpected grant emails %#v", req.GrantEmails)
			}
			return &api.Tunnel{
				ID:            "tt_abcdefghijkl",
				Name:          req.Name,
				Mode:          req.Mode,
				Kind:          api.TunnelKindDirect,
				EnvironmentId: req.EnvironmentId,
			}, nil
		},
	}

	got, err := create(context.Background(), fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
	}, Spec{
		Name:        " gateway ",
		Mode:        ModeHTTP,
		GrantEmails: []string{"", " alice@example.com "},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ID != "tt_abcdefghijkl" || got.Kind != KindDirect || got.Mode != ModeHTTP {
		t.Fatalf("unexpected tunnel %#v", got)
	}
}

func TestListenResolvesNameAndOpensOverlayByTunnelID(t *testing.T) {
	listener := &fakeNetListener{}
	overlay := &fakeOverlay{listener: listener}
	factory := &fakeOverlayFactory{overlay: overlay}
	controller := &fakePrimitiveController{
		listTunnels: func(_ context.Context, params api.ListTunnelsParams) (api.ListTunnelsRes, error) {
			if params.Scope.Or(api.ListTunnelsScopeOwned) != api.ListTunnelsScopeAll {
				t.Fatalf("expected all scope, got %#v", params.Scope)
			}
			return &api.ListTunnelsResponse{{
				ID:            "tt_abcdefghijkl",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				Kind:          api.TunnelKindDirect,
				EnvironmentId: "ev_test00000001",
				ZitiServiceId: api.NewOptString("svc-1"),
				BindPolicyId:  api.NewOptString("bind-1"),
			}}, nil
		},
	}

	got, err := listen(context.Background(), fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
		root:  fakeIdentityRoot{path: "/tmp/environment.json"},
	}, factory, " gateway ")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if got != listener {
		t.Fatalf("expected fake listener, got %#v", got)
	}
	if factory.identityPath != "/tmp/environment.json" {
		t.Fatalf("unexpected identity path %q", factory.identityPath)
	}
	if overlay.listenService != "tt_abcdefghijkl" {
		t.Fatalf("expected overlay listen on tunnel id, got %q", overlay.listenService)
	}
}

func TestListenValidation(t *testing.T) {
	base := api.Tunnel{
		ID:            "tt_abcdefghijkl",
		Name:          "gateway",
		Mode:          api.TunnelModeHTTP,
		Kind:          api.TunnelKindDirect,
		EnvironmentId: "ev_test00000001",
		ZitiServiceId: api.NewOptString("svc-1"),
		BindPolicyId:  api.NewOptString("bind-1"),
	}

	tests := []struct {
		name   string
		tunnel api.Tunnel
		want   error
	}{
		{name: "proxy", tunnel: withTunnelKind(base, api.TunnelKindProxy), want: ErrInvalidSpec},
		{name: "udp", tunnel: withTunnelMode(base, api.TunnelModeUDP), want: ErrUnsupportedMode},
		{name: "wrong environment", tunnel: withTunnelEnvironment(base, "ev_test00000002"), want: ErrInvalidSpec},
		{name: "missing metadata", tunnel: withTunnelMetadata(base, api.OptString{}, api.NewOptString("bind-1")), want: ErrTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateListenTunnel(&tt.tunnel, "ev_test00000001")
			assertErrorIs(t, err, tt.want)
		})
	}
}

type fakePrimitiveAgent struct {
	ctrl  primitiveController
	envID string
	root  identityRoot
}

func (f fakePrimitiveAgent) controller() primitiveController { return f.ctrl }
func (f fakePrimitiveAgent) environmentID() string           { return f.envID }
func (f fakePrimitiveAgent) envRoot() identityRoot           { return f.root }

type fakePrimitiveController struct {
	createTunnel func(context.Context, *api.CreateTunnelRequest) (api.CreateTunnelRes, error)
	deleteTunnel func(context.Context, api.DeleteTunnelParams) (api.DeleteTunnelRes, error)
	getTunnel    func(context.Context, api.GetTunnelParams) (api.GetTunnelRes, error)
	listTunnels  func(context.Context, api.ListTunnelsParams) (api.ListTunnelsRes, error)
}

func (f *fakePrimitiveController) CreateTunnel(ctx context.Context, req *api.CreateTunnelRequest) (api.CreateTunnelRes, error) {
	if f.createTunnel == nil {
		panic("unexpected CreateTunnel call")
	}
	return f.createTunnel(ctx, req)
}

func (f *fakePrimitiveController) DeleteTunnel(ctx context.Context, params api.DeleteTunnelParams) (api.DeleteTunnelRes, error) {
	if f.deleteTunnel == nil {
		panic("unexpected DeleteTunnel call")
	}
	return f.deleteTunnel(ctx, params)
}

func (f *fakePrimitiveController) GetTunnel(ctx context.Context, params api.GetTunnelParams) (api.GetTunnelRes, error) {
	if f.getTunnel == nil {
		panic("unexpected GetTunnel call")
	}
	return f.getTunnel(ctx, params)
}

func (f *fakePrimitiveController) ListTunnels(ctx context.Context, params api.ListTunnelsParams) (api.ListTunnelsRes, error) {
	if f.listTunnels == nil {
		panic("unexpected ListTunnels call")
	}
	return f.listTunnels(ctx, params)
}

type fakeIdentityRoot struct {
	path string
	err  error
}

func (f fakeIdentityRoot) ZitiIdentityNamed(string) (string, error) {
	return f.path, f.err
}

type fakeOverlayFactory struct {
	identityPath string
	overlay      *fakeOverlay
	err          error
}

func (f *fakeOverlayFactory) New(identityPath string) (tunnelruntime.OverlayContext, error) {
	f.identityPath = identityPath
	if f.err != nil {
		return nil, f.err
	}
	return f.overlay, nil
}

type fakeOverlay struct {
	listener      net.Listener
	listenService string
	listenErr     error
}

func (f *fakeOverlay) Listen(serviceName string) (net.Listener, error) {
	f.listenService = serviceName
	if f.listenErr != nil {
		return nil, f.listenErr
	}
	return f.listener, nil
}

func (f *fakeOverlay) Dial(string) (net.Conn, error) {
	return nil, errors.New("unexpected dial")
}

type fakeNetListener struct{}

func (*fakeNetListener) Accept() (net.Conn, error) { return nil, errors.New("closed") }
func (*fakeNetListener) Close() error              { return nil }
func (*fakeNetListener) Addr() net.Addr            { return fakeAddr("listener") }

type fakeAddr string

func (a fakeAddr) Network() string { return string(a) }
func (a fakeAddr) String() string  { return string(a) }

func withTunnelKind(tunnel api.Tunnel, kind api.TunnelKind) api.Tunnel {
	tunnel.Kind = kind
	return tunnel
}

func withTunnelMode(tunnel api.Tunnel, mode api.TunnelMode) api.Tunnel {
	tunnel.Mode = mode
	return tunnel
}

func withTunnelEnvironment(tunnel api.Tunnel, environmentID string) api.Tunnel {
	tunnel.EnvironmentId = environmentID
	return tunnel
}

func withTunnelMetadata(tunnel api.Tunnel, serviceID, bindPolicyID api.OptString) api.Tunnel {
	tunnel.ZitiServiceId = serviceID
	tunnel.BindPolicyId = bindPolicyID
	return tunnel
}

var _ net.Listener = (*fakeNetListener)(nil)

func TestDeleteDirectTunnel(t *testing.T) {
	controller := &fakePrimitiveController{
		deleteTunnel: func(_ context.Context, params api.DeleteTunnelParams) (api.DeleteTunnelRes, error) {
			if params.TunnelId != "tt_abcdefghijkl" {
				t.Fatalf("unexpected tunnel id %q", params.TunnelId)
			}
			return &api.DeleteTunnelNoContent{}, nil
		},
	}
	if err := deleteTunnel(context.Background(), fakePrimitiveAgent{ctrl: controller}, &Tunnel{ID: " tt_abcdefghijkl "}); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestListenResolvesIDThroughGetTunnel(t *testing.T) {
	listener := &fakeNetListener{}
	overlay := &fakeOverlay{listener: listener}
	factory := &fakeOverlayFactory{overlay: overlay}
	controller := &fakePrimitiveController{
		getTunnel: func(_ context.Context, params api.GetTunnelParams) (api.GetTunnelRes, error) {
			if params.TunnelId != "tt_abcdefghijkl" {
				t.Fatalf("unexpected tunnel id %q", params.TunnelId)
			}
			return &api.Tunnel{
				ID:            "tt_abcdefghijkl",
				Name:          "gateway",
				Mode:          api.TunnelModeTCP,
				Kind:          api.TunnelKindDirect,
				EnvironmentId: "ev_test00000001",
				ZitiServiceId: api.NewOptString("svc-1"),
				BindPolicyId:  api.NewOptString("bind-1"),
			}, nil
		},
	}
	if _, err := listen(context.Background(), fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
		root:  fakeIdentityRoot{path: "/tmp/environment.json"},
	}, factory, "tt_abcdefghijkl"); err != nil {
		t.Fatalf("listen by id: %v", err)
	}
	if overlay.listenService != "tt_abcdefghijkl" {
		t.Fatalf("expected overlay listen on tunnel id, got %q", overlay.listenService)
	}
}

func TestCreateValidation(t *testing.T) {
	_, err := create(context.Background(), fakePrimitiveAgent{envID: "ev_test00000001"}, Spec{Name: "gateway"})
	assertErrorIs(t, err, ErrInvalidSpec)
}

func TestListenIdentityFailure(t *testing.T) {
	errRoot := fakeIdentityRoot{err: errors.New("missing identity")}
	controller := &fakePrimitiveController{
		listTunnels: func(context.Context, api.ListTunnelsParams) (api.ListTunnelsRes, error) {
			return &api.ListTunnelsResponse{{
				ID:            "tt_abcdefghijkl",
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				Kind:          api.TunnelKindDirect,
				EnvironmentId: "ev_test00000001",
				ZitiServiceId: api.NewOptString("svc-1"),
				BindPolicyId:  api.NewOptString("bind-1"),
			}}, nil
		},
	}
	_, err := listen(context.Background(), fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
		root:  errRoot,
	}, &fakeOverlayFactory{}, "gateway")
	assertErrorIs(t, err, ErrTransient)
}
