package tunnel

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/network/tunnelruntime"
)

func TestCreateDirectTunnelOmitsBackendTarget(t *testing.T) {
	controller := &fakePrimitiveController{
		createTunnel: func(_ context.Context, req *api.CreateTunnelRequest) (api.CreateTunnelRes, error) {
			if req.EnvironmentId.Set {
				t.Fatalf("standalone create should not send environmentId, got %#v", req.EnvironmentId)
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
				ID:   "tt_abcdefghijkl",
				Name: req.Name,
				Mode: req.Mode,
				Kind: api.TunnelKindDirect,
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
		{name: "missing metadata", tunnel: withTunnelMetadata(base, api.OptString{}, api.NewOptString("bind-1")), want: ErrTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateListenTunnel(&tt.tunnel)
			assertErrorIs(t, err, tt.want)
		})
	}
}

func TestAttachCreatesDialerAttachment(t *testing.T) {
	controller := &fakePrimitiveController{
		connectTunnel: func(_ context.Context, req *api.ConnectTunnelRequest) (api.ConnectTunnelRes, error) {
			if req.EnvironmentId != "ev_test00000001" || req.Name != "gateway" {
				t.Fatalf("unexpected attach request %#v", req)
			}
			if req.ListenAddress.Set {
				t.Fatalf("dialer attach should omit listenAddress, got %#v", req.ListenAddress)
			}
			return &api.ConnectTunnelResponse{
				Tunnel: api.Tunnel{
					ID:   "tt_abcdefghijkl",
					Name: "gateway",
					Mode: api.TunnelModeHTTP,
					Kind: api.TunnelKindDirect,
				},
				Attachment: api.TunnelAttachment{
					ID:            "ta_abcdefghijkl",
					TunnelId:      "tt_abcdefghijkl",
					EnvironmentId: "ev_test00000001",
					Kind:          api.TunnelAttachmentKindDialer,
					State:         api.TunnelAttachmentStateActive,
				},
			}, nil
		},
	}

	got, err := attach(context.Background(), fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
	}, " gateway ")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if got.ID != "ta_abcdefghijkl" || got.TunnelID != "tt_abcdefghijkl" || got.Kind != AttachmentDialer {
		t.Fatalf("unexpected attachment %#v", got)
	}
}

func TestDetachAcceptsAttachmentTarget(t *testing.T) {
	controller := &fakePrimitiveController{
		detachDialerAttachment: func(_ context.Context, params api.DetachDialerAttachmentParams) (api.DetachDialerAttachmentRes, error) {
			if params.EnvironmentId != "ev_test00000001" || params.Tunnel != "tt_abcdefghijkl" {
				t.Fatalf("unexpected detach params %#v", params)
			}
			return &api.DetachDialerAttachmentNoContent{}, nil
		},
	}

	err := detach(context.Background(), fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
	}, &Attachment{ID: "ta_abcdefghijkl", TunnelID: "tt_abcdefghijkl"})
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
}

func TestDialUsesActiveDialerAndOverlayTunnelID(t *testing.T) {
	conn := &fakeNetConn{}
	overlay := &fakeOverlay{conn: conn}
	factory := &fakeOverlayFactory{overlay: overlay}
	controller := &fakePrimitiveController{
		getActiveDialerAttachment: func(_ context.Context, params api.GetActiveDialerAttachmentParams) (api.GetActiveDialerAttachmentRes, error) {
			if params.EnvironmentId != "ev_test00000001" || params.Tunnel != "gateway" {
				t.Fatalf("unexpected dial lookup params %#v", params)
			}
			return &api.ActiveDialerAttachment{
				Attachment: api.TunnelAttachment{
					ID:            "ta_abcdefghijkl",
					TunnelId:      "tt_abcdefghijkl",
					EnvironmentId: "ev_test00000001",
					Kind:          api.TunnelAttachmentKindDialer,
					State:         api.TunnelAttachmentStateActive,
				},
				TunnelId:   "tt_abcdefghijkl",
				TunnelMode: api.TunnelModeHTTP,
			}, nil
		},
	}

	got, err := dial(context.Background(), fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
		root:  fakeIdentityRoot{path: "/tmp/environment.json"},
	}, factory, " gateway ")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if got != conn {
		t.Fatalf("expected fake conn, got %#v", got)
	}
	if factory.identityPath != "/tmp/environment.json" {
		t.Fatalf("unexpected identity path %q", factory.identityPath)
	}
	if overlay.dialService != "tt_abcdefghijkl" {
		t.Fatalf("expected overlay dial on tunnel id, got %q", overlay.dialService)
	}
}

func TestDialRejectsUDPTunnel(t *testing.T) {
	controller := &fakePrimitiveController{
		getActiveDialerAttachment: func(context.Context, api.GetActiveDialerAttachmentParams) (api.GetActiveDialerAttachmentRes, error) {
			return &api.ActiveDialerAttachment{
				TunnelId:   "tt_abcdefghijkl",
				TunnelMode: api.TunnelModeUDP,
			}, nil
		},
	}

	_, err := dial(context.Background(), fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
		root:  fakeIdentityRoot{path: "/tmp/environment.json"},
	}, &fakeOverlayFactory{overlay: &fakeOverlay{}}, "gateway")
	assertErrorIs(t, err, ErrUnsupportedMode)
}

func TestDialPreservesContextCancellation(t *testing.T) {
	controller := &fakePrimitiveController{
		getActiveDialerAttachment: func(context.Context, api.GetActiveDialerAttachmentParams) (api.GetActiveDialerAttachmentRes, error) {
			return &api.ActiveDialerAttachment{
				TunnelId:   "tt_abcdefghijkl",
				TunnelMode: api.TunnelModeTCP,
			}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := dial(ctx, fakePrimitiveAgent{
		ctrl:  controller,
		envID: "ev_test00000001",
		root:  fakeIdentityRoot{path: "/tmp/environment.json"},
	}, &fakeOverlayFactory{overlay: &fakeOverlay{dialErr: context.Canceled}}, "gateway")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type fakePrimitiveAgent struct {
	ctrl  primitiveController
	envID string
	root  identityRoot
}

func (f fakePrimitiveAgent) controller() primitiveController { return f.ctrl }
func (f fakePrimitiveAgent) environmentID() string           { return f.envID }
func (f fakePrimitiveAgent) identityPath() string {
	if f.root == nil {
		return ""
	}
	path, _ := f.root.ZitiIdentityNamed(environmentIdentityName)
	return path
}

type identityRoot interface {
	ZitiIdentityNamed(name string) (string, error)
}

type fakePrimitiveController struct {
	createTunnel              func(context.Context, *api.CreateTunnelRequest) (api.CreateTunnelRes, error)
	deleteTunnel              func(context.Context, api.DeleteTunnelParams) (api.DeleteTunnelRes, error)
	getTunnel                 func(context.Context, api.GetTunnelParams) (api.GetTunnelRes, error)
	listTunnels               func(context.Context, api.ListTunnelsParams) (api.ListTunnelsRes, error)
	connectTunnel             func(context.Context, *api.ConnectTunnelRequest) (api.ConnectTunnelRes, error)
	getActiveDialerAttachment func(context.Context, api.GetActiveDialerAttachmentParams) (api.GetActiveDialerAttachmentRes, error)
	detachDialerAttachment    func(context.Context, api.DetachDialerAttachmentParams) (api.DetachDialerAttachmentRes, error)
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

func (f *fakePrimitiveController) ConnectTunnel(ctx context.Context, req *api.ConnectTunnelRequest) (api.ConnectTunnelRes, error) {
	if f.connectTunnel == nil {
		panic("unexpected ConnectTunnel call")
	}
	return f.connectTunnel(ctx, req)
}

func (f *fakePrimitiveController) GetActiveDialerAttachment(ctx context.Context, params api.GetActiveDialerAttachmentParams) (api.GetActiveDialerAttachmentRes, error) {
	if f.getActiveDialerAttachment == nil {
		panic("unexpected GetActiveDialerAttachment call")
	}
	return f.getActiveDialerAttachment(ctx, params)
}

func (f *fakePrimitiveController) DetachDialerAttachment(ctx context.Context, params api.DetachDialerAttachmentParams) (api.DetachDialerAttachmentRes, error) {
	if f.detachDialerAttachment == nil {
		panic("unexpected DetachDialerAttachment call")
	}
	return f.detachDialerAttachment(ctx, params)
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
	conn          net.Conn
	listenService string
	dialService   string
	listenErr     error
	dialErr       error
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

func (f *fakeOverlay) DialContext(_ context.Context, serviceName string) (net.Conn, error) {
	f.dialService = serviceName
	if f.dialErr != nil {
		return nil, f.dialErr
	}
	return f.conn, nil
}

type fakeNetListener struct{}

func (*fakeNetListener) Accept() (net.Conn, error) { return nil, errors.New("closed") }
func (*fakeNetListener) Close() error              { return nil }
func (*fakeNetListener) Addr() net.Addr            { return fakeAddr("listener") }

type fakeNetConn struct{}

func (*fakeNetConn) Read([]byte) (int, error)         { return 0, errors.New("closed") }
func (*fakeNetConn) Write([]byte) (int, error)        { return 0, errors.New("closed") }
func (*fakeNetConn) Close() error                     { return nil }
func (*fakeNetConn) LocalAddr() net.Addr              { return fakeAddr("local") }
func (*fakeNetConn) RemoteAddr() net.Addr             { return fakeAddr("remote") }
func (*fakeNetConn) SetDeadline(time.Time) error      { return nil }
func (*fakeNetConn) SetReadDeadline(time.Time) error  { return nil }
func (*fakeNetConn) SetWriteDeadline(time.Time) error { return nil }

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

func TestGetResolvesTunnelByName(t *testing.T) {
	controller := &fakePrimitiveController{
		listTunnels: func(_ context.Context, params api.ListTunnelsParams) (api.ListTunnelsRes, error) {
			return &api.ListTunnelsResponse{{
				ID:   "tt_abcdefghijkl",
				Name: "gateway",
				Mode: api.TunnelModeTCP,
				Kind: api.TunnelKindDirect,
			}}, nil
		},
	}
	tun, err := get(context.Background(), controller, "gateway")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if tun.Mode != ModeTCP {
		t.Fatalf("mode = %q, want tcp", tun.Mode)
	}
	if tun.ID != "tt_abcdefghijkl" {
		t.Fatalf("id = %q", tun.ID)
	}
}

func TestGetNotFound(t *testing.T) {
	controller := &fakePrimitiveController{
		listTunnels: func(context.Context, api.ListTunnelsParams) (api.ListTunnelsRes, error) {
			return &api.ListTunnelsResponse{}, nil
		},
	}
	_, err := get(context.Background(), controller, "missing")
	assertErrorIs(t, err, ErrNotFound)
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
