package tunnelruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/sdk-golang/ziti/edge"
)

type overlayPeerInfo struct {
	IdentityName     string
	IdentityID       string
	IdentityTrust    string
	SourceIdentifier string
	CircuitID        string
	AppDataLength    int
}

type overlayPeerInfoContextKey struct{}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (w *loggingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten += int64(n)
	return n, err
}

func overlayPeerInfoFromConn(conn net.Conn) overlayPeerInfo {
	info := overlayPeerInfo{IdentityTrust: "missing"}

	if serviceConn, ok := conn.(edge.ServiceConn); ok {
		info.SourceIdentifier = serviceConn.SourceIdentifier()
		info.CircuitID = serviceConn.GetCircuitId()
		info.AppDataLength = len(serviceConn.GetAppData())
		identityID := serviceConn.GetDialerIdentityId()
		identityName := serviceConn.GetDialerIdentityName()
		if identityID != "" {
			info.IdentityID = identityID
		}
		if identityName != "" {
			info.IdentityName = identityName
		}
		if identityID != "" || identityName != "" {
			info.IdentityTrust = "router_attested"
		}
	}

	return info
}

func overlayPeerInfoFromContext(ctx context.Context) overlayPeerInfo {
	info, _ := ctx.Value(overlayPeerInfoContextKey{}).(overlayPeerInfo)
	return info
}

func logServeTunnelAccept(mode, serviceName, backendTarget string, conn net.Conn) overlayPeerInfo {
	info := overlayPeerInfoFromConn(conn)
	dl.Infof(
		"accepted %s tunnel session service='%s' backend='%s' %s",
		mode,
		serviceName,
		backendTarget,
		overlayPeerLogFields(info),
	)
	return info
}

func logServeTunnelSessionClosed(mode, serviceName, backendTarget string, info overlayPeerInfo, stats proxyConnStats) {
	dl.Infof(
		"closed %s tunnel session service='%s' backend='%s' duration_ms=%d dialer_to_backend_bytes=%d backend_to_dialer_bytes=%d %s%s",
		mode,
		serviceName,
		backendTarget,
		stats.Duration.Milliseconds(),
		stats.LeftToRightBytes,
		stats.RightToLeftBytes,
		overlayPeerLogFields(info),
		proxyErrorLogFields(stats),
	)
}

func logServeTunnelBackendDialFailure(mode, serviceName, backendTarget string, info overlayPeerInfo, err error) {
	dl.Warnf(
		"failed to connect %s tunnel backend service='%s' backend='%s' err='%s' %s",
		mode,
		serviceName,
		backendTarget,
		escapeLogValue(err.Error()),
		overlayPeerLogFields(info),
	)
}

func logConnectTunnelAccept(mode, serviceName, listenAddress, remoteAddr string) {
	dl.Infof(
		"accepted local %s tunnel session service='%s' listen='%s' remote_addr='%s'",
		mode,
		serviceName,
		listenAddress,
		escapeLogValue(remoteAddr),
	)
}

func logConnectTunnelDialFailure(mode, serviceName, listenAddress, remoteAddr string, err error) {
	dl.Warnf(
		"failed to open local %s tunnel session service='%s' listen='%s' remote_addr='%s' err='%s'",
		mode,
		serviceName,
		listenAddress,
		escapeLogValue(remoteAddr),
		escapeLogValue(err.Error()),
	)
}

func logConnectTunnelSessionClosed(mode, serviceName, listenAddress, remoteAddr string, stats proxyConnStats) {
	dl.Infof(
		"closed local %s tunnel session service='%s' listen='%s' remote_addr='%s' duration_ms=%d local_to_tunnel_bytes=%d tunnel_to_local_bytes=%d%s",
		mode,
		serviceName,
		listenAddress,
		escapeLogValue(remoteAddr),
		stats.Duration.Milliseconds(),
		stats.LeftToRightBytes,
		stats.RightToLeftBytes,
		proxyErrorLogFields(stats),
	)
}

type udpSessionStats struct {
	LocalToTunnelBytes   int64
	TunnelToLocalBytes   int64
	LocalToTunnelPackets int64
	TunnelToLocalPackets int64
	Duration             time.Duration
}

func logConnectUDPSessionStart(serviceName, listenAddress, remoteAddr string) {
	dl.Infof(
		"accepted local udp tunnel session service='%s' listen='%s' remote_addr='%s'",
		serviceName,
		listenAddress,
		escapeLogValue(remoteAddr),
	)
}

func logConnectUDPSessionClosed(serviceName, listenAddress, remoteAddr string, stats udpSessionStats) {
	dl.Infof(
		"closed local udp tunnel session service='%s' listen='%s' remote_addr='%s' duration_ms=%d local_to_tunnel_packets=%d local_to_tunnel_bytes=%d tunnel_to_local_packets=%d tunnel_to_local_bytes=%d",
		serviceName,
		listenAddress,
		escapeLogValue(remoteAddr),
		stats.Duration.Milliseconds(),
		stats.LocalToTunnelPackets,
		stats.LocalToTunnelBytes,
		stats.TunnelToLocalPackets,
		stats.TunnelToLocalBytes,
	)
}

func serveHTTPRequestLogger(serviceName, backendTarget string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := overlayPeerInfoFromContext(r.Context())
		writer := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		startedAt := time.Now()

		next.ServeHTTP(writer, r)

		dl.Infof(
			"served http tunnel request service='%s' backend='%s' method='%s' host='%s' uri='%s' status=%d duration_ms=%d response_bytes=%d %s",
			serviceName,
			backendTarget,
			r.Method,
			r.Host,
			r.URL.RequestURI(),
			writer.statusCode,
			time.Since(startedAt).Milliseconds(),
			writer.bytesWritten,
			overlayPeerLogFields(info),
		)
	})
}

func connectHTTPRequestLogger(serviceName, listenAddress string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		startedAt := time.Now()

		next.ServeHTTP(writer, r)

		dl.Infof(
			"served local http tunnel request service='%s' listen='%s' remote_addr='%s' method='%s' host='%s' uri='%s' status=%d duration_ms=%d response_bytes=%d",
			serviceName,
			listenAddress,
			escapeLogValue(r.RemoteAddr),
			r.Method,
			r.Host,
			r.URL.RequestURI(),
			writer.statusCode,
			time.Since(startedAt).Milliseconds(),
			writer.bytesWritten,
		)
	})
}

func overlayPeerLogFields(info overlayPeerInfo) string {
	fields := []string{
		fmt.Sprintf("identity_name='%s'", escapeLogValue(info.IdentityName)),
		fmt.Sprintf("identity_trust='%s'", escapeLogValue(info.IdentityTrust)),
		fmt.Sprintf("circuit_id='%s'", escapeLogValue(info.CircuitID)),
	}
	if info.IdentityID != "" {
		fields = append(fields, fmt.Sprintf("identity_id='%s'", escapeLogValue(info.IdentityID)))
	}
	if info.SourceIdentifier != "" && info.SourceIdentifier != info.IdentityName {
		fields = append(fields, fmt.Sprintf("source_identifier='%s'", escapeLogValue(info.SourceIdentifier)))
	}
	if info.AppDataLength > 0 {
		fields = append(fields, fmt.Sprintf("app_data_len=%d", info.AppDataLength))
	}
	return strings.Join(fields, " ")
}

func proxyErrorLogFields(stats proxyConnStats) string {
	fields := []string{}
	if err := normalizeProxyErr(stats.LeftToRightErr); err != nil {
		fields = append(fields, fmt.Sprintf(" left_to_right_err='%s'", escapeLogValue(err.Error())))
	}
	if err := normalizeProxyErr(stats.RightToLeftErr); err != nil {
		fields = append(fields, fmt.Sprintf(" right_to_left_err='%s'", escapeLogValue(err.Error())))
	}
	return strings.Join(fields, "")
}

func normalizeProxyErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func escapeLogValue(v string) string {
	v = strings.ReplaceAll(v, "\n", " ")
	v = strings.ReplaceAll(v, "\r", " ")
	return strings.ReplaceAll(v, "'", "\\'")
}
