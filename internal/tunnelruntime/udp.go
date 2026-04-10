package tunnelruntime

import (
	"context"
	"net"
	"sync"
	"time"
)

type udpClientConn struct {
	addr   *net.UDPAddr
	conn   net.Conn
	active chan struct{}
}

func ServeUDP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, backendTarget string) error {
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
			backend, err := net.Dial("udp", backendTarget)
			if err != nil {
				_ = inbound.Close()
				return
			}
			proxyConnPair(inbound, backend)
		}(conn)
	}
}

func ConnectUDP(ctx context.Context, factory OverlayFactory, identityPath, serviceName, listenAddress string) error {
	overlay, err := factory.New(identityPath)
	if err != nil {
		return err
	}

	addr, err := net.ResolveUDPAddr("udp", listenAddress)
	if err != nil {
		return err
	}
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	closeOnDone(ctx, listener)

	clients := sync.Map{}
	closeClient := func(key string, client *udpClientConn) {
		clients.Delete(key)
		_ = client.conn.Close()
	}

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
				continue
			}
			client := &udpClientConn{
				addr:   clientAddr,
				conn:   overlayConn,
				active: make(chan struct{}, 1),
			}
			clients.Store(key, client)
			go proxyUDPResponse(ctx, listener, key, client, closeClient)
			value = client
		}

		client := value.(*udpClientConn)
		select {
		case client.active <- struct{}{}:
		default:
		}
		if _, err := client.conn.Write(buf[:n]); err != nil {
			closeClient(key, client)
		}
	}
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
		if _, err := listener.WriteToUDP(buf[:n], client.addr); err != nil {
			return
		}
	}
}
