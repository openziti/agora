package tunnelruntime

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type udpClientConn struct {
	addr                 *net.UDPAddr
	conn                 net.Conn
	active               chan struct{}
	startedAt            time.Time
	localToTunnelBytes   atomic.Int64
	tunnelToLocalBytes   atomic.Int64
	localToTunnelPackets atomic.Int64
	tunnelToLocalPackets atomic.Int64
	closeOnce            sync.Once
}

func ServeUDP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, backendTarget string) error {
	handle, err := StartServeUDP(ctx, factory, identityPath, serviceName, backendTarget)
	if err != nil {
		return err
	}
	return <-handle.Done()
}

func StartServeUDP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, backendTarget string) (*Handle, error) {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return nil, err
	}
	listener, err := overlay.Listen(serviceName)
	if err != nil {
		return nil, err
	}
	closeOnDone(ctx, listener)

	return newHandle(func() error {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return ignoreClosedError(ctx, err)
			}
			info := logServeTunnelAccept("udp", serviceName, backendTarget, conn)
			go func(inbound net.Conn) {
				backend, err := net.Dial("udp", backendTarget)
				if err != nil {
					logServeTunnelBackendDialFailure("udp", serviceName, backendTarget, info, err)
					_ = inbound.Close()
					return
				}
				proxyConnPairWithStats(inbound, backend, func(stats proxyConnStats) {
					logServeTunnelSessionClosed("udp", serviceName, backendTarget, info, stats)
				})
			}(conn)
		}
	}), nil
}

func ConnectUDP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, listenAddress string) error {
	handle, err := StartConnectUDP(ctx, factory, identityPath, serviceName, listenAddress)
	if err != nil {
		return err
	}
	return <-handle.Done()
}

func StartConnectUDP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, listenAddress string) (*Handle, error) {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return nil, err
	}

	addr, err := net.ResolveUDPAddr("udp", listenAddress)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	closeOnDone(ctx, listener)

	clients := sync.Map{}
	closeClient := func(key string, client *udpClientConn) {
		client.closeOnce.Do(func() {
			clients.Delete(key)
			_ = client.conn.Close()
			logConnectUDPSessionClosed(serviceName, listenAddress, key, udpSessionStats{
				LocalToTunnelBytes:   client.localToTunnelBytes.Load(),
				TunnelToLocalBytes:   client.tunnelToLocalBytes.Load(),
				LocalToTunnelPackets: client.localToTunnelPackets.Load(),
				TunnelToLocalPackets: client.tunnelToLocalPackets.Load(),
				Duration:             time.Since(client.startedAt),
			})
		})
	}

	return newHandle(func() error {
		for {
			buf := make([]byte, 64*1024)
			n, clientAddr, err := listener.ReadFromUDP(buf)
			if err != nil {
				return ignoreClosedError(ctx, err)
			}

			key := clientAddr.String()
			value, ok := clients.Load(key)
			if !ok {
				overlayConn, err := overlay.Dial(serviceName)
				if err != nil {
					logConnectTunnelDialFailure("udp", serviceName, listenAddress, key, err)
					continue
				}
				client := &udpClientConn{
					addr:      clientAddr,
					conn:      overlayConn,
					active:    make(chan struct{}, 1),
					startedAt: time.Now(),
				}
				clients.Store(key, client)
				logConnectUDPSessionStart(serviceName, listenAddress, key)
				go proxyUDPResponse(ctx, listener, key, client, closeClient)
				value = client
			}

			client := value.(*udpClientConn)
			select {
			case client.active <- struct{}{}:
			default:
			}
			written, err := client.conn.Write(buf[:n])
			if err != nil {
				closeClient(key, client)
				continue
			}
			client.localToTunnelBytes.Add(int64(written))
			client.localToTunnelPackets.Add(1)
		}
	}), nil
}

func proxyUDPResponse(ctx context.Context, listener *net.UDPConn, key string, client *udpClientConn, closeFn func(string, *udpClientConn)) {
	defer closeFn(key, client)

	go func() {
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = client.conn.Close()
				return
			case <-client.active:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(time.Minute)
			case <-timer.C:
				_ = client.conn.Close()
				return
			}
		}
	}()

	buf := make([]byte, 64*1024)
	for {
		n, err := client.conn.Read(buf)
		if err != nil {
			return
		}
		written, err := listener.WriteToUDP(buf[:n], client.addr)
		if err != nil {
			return
		}
		client.tunnelToLocalBytes.Add(int64(written))
		client.tunnelToLocalPackets.Add(1)
	}
}
