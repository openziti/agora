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
		info := logServeTunnelAccept("tcp", serviceName, backendTarget, conn)
		go func(inbound net.Conn) {
			backend, err := net.Dial("tcp", backendTarget)
			if err != nil {
				logServeTunnelBackendDialFailure("tcp", serviceName, backendTarget, info, err)
				_ = inbound.Close()
				return
			}
			proxyConnPairWithStats(inbound, backend, func(stats proxyConnStats) {
				logServeTunnelSessionClosed("tcp", serviceName, backendTarget, info, stats)
			})
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
		remoteAddr := conn.RemoteAddr().String()
		logConnectTunnelAccept("tcp", serviceName, listenAddress, remoteAddr)
		go func(local net.Conn, remoteAddr string) {
			overlayConn, err := overlay.Dial(serviceName)
			if err != nil {
				logConnectTunnelDialFailure("tcp", serviceName, listenAddress, remoteAddr, err)
				_ = local.Close()
				return
			}
			proxyConnPairWithStats(local, overlayConn, func(stats proxyConnStats) {
				logConnectTunnelSessionClosed("tcp", serviceName, listenAddress, remoteAddr, stats)
			})
		}(conn, remoteAddr)
	}
}
