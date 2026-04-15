package tunnelruntime

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func ServeHTTP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, backendTarget string) error {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return err
	}
	listener, err := overlay.Listen(serviceName)
	if err != nil {
		return err
	}

	targetURL, err := url.Parse(backendTarget)
	if err != nil {
		_ = listener.Close()
		return err
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
	return ignoreClosedError(ctx, server.Serve(listener))
}

func ConnectHTTP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, listenAddress string) error {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return err
	}

	targetURL, err := url.Parse("http://" + serviceName)
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(_ context.Context, _, _ string) (net.Conn, error) {
		return overlay.Dial(serviceName)
	}
	proxy.Transport = transport

	server := &http.Server{
		Addr:    listenAddress,
		Handler: connectHTTPRequestLogger(serviceName, listenAddress, proxy),
	}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	return ignoreClosedError(ctx, server.ListenAndServe())
}
