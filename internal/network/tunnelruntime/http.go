package tunnelruntime

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func ServeHTTP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, backendTarget string) error {
	handle, err := StartServeHTTP(ctx, factory, identityPath, serviceName, backendTarget)
	if err != nil {
		return err
	}
	return <-handle.Done()
}

func StartServeHTTP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, backendTarget string) (*Handle, error) {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return nil, err
	}
	listener, err := overlay.Listen(serviceName)
	if err != nil {
		return nil, err
	}

	targetURL, err := url.Parse(backendTarget)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}

	server := &http.Server{
		Handler: serveHTTPRequestLogger(serviceName, backendTarget, httputil.NewSingleHostReverseProxy(targetURL)),
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			return context.WithValue(ctx, overlayPeerInfoContextKey{}, overlayPeerInfoFromConn(conn))
		},
	}
	closeOnDone(ctx, listener)
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	return newHandle(func() error {
		return ignoreClosedError(ctx, server.Serve(listener))
	}), nil
}

func ConnectHTTP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, listenAddress string) error {
	handle, err := StartConnectHTTP(ctx, factory, identityPath, serviceName, listenAddress)
	if err != nil {
		return err
	}
	return <-handle.Done()
}

func StartConnectHTTP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, listenAddress string) (*Handle, error) {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return nil, err
	}

	targetURL, err := url.Parse("http://" + serviceName)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(_ context.Context, _, _ string) (net.Conn, error) {
		return overlay.Dial(serviceName)
	}
	proxy.Transport = transport

	server := &http.Server{
		Handler: connectHTTPRequestLogger(serviceName, listenAddress, proxy),
	}
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return nil, err
	}
	closeOnDone(ctx, listener)
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	return newHandle(func() error {
		return ignoreClosedError(ctx, server.Serve(listener))
	}), nil
}
