package tunnelruntime

import (
	"context"
	"net"
)

func ServeTCP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, backendTarget string) error {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return err
	}
	listener, err := overlay.Listen(serviceName)
	if err != nil {
		return err
	}
	closeOnDone(ctx, listener)

	for {
		conn, err := listener.Accept()
		if err != nil {
			return ignoreClosedError(ctx, err)
		}
		go func(inbound net.Conn) {
			backend, err := net.Dial("tcp", backendTarget)
			if err != nil {
				_ = inbound.Close()
				return
			}
			proxyConnPair(inbound, backend)
		}(conn)
	}
}

func ConnectTCP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, listenAddress string) error {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return err
	}
	closeOnDone(ctx, listener)

	for {
		conn, err := listener.Accept()
		if err != nil {
			return ignoreClosedError(ctx, err)
		}
		go func(local net.Conn) {
			overlayConn, err := overlay.Dial(serviceName)
			if err != nil {
				_ = local.Close()
				return
			}
			proxyConnPair(local, overlayConn)
		}(conn)
	}
}
