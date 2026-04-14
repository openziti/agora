package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/tunnelruntime"
	"github.com/spf13/cobra"
)

const (
	tunnelAttachmentHeartbeatInterval = 15 * time.Second
	tunnelServeHeartbeatInterval      = 15 * time.Second
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Tunnel lifecycle commands",
}

func init() {
	rootCmd.AddCommand(tunnelCmd)
}

func requireEnabledRoot() env_core.Root {
	root, err := environment.LoadRoot()
	if err != nil {
		panic(err)
	}
	if !root.IsEnabled() {
		panic("no environment is enabled")
	}
	return root
}

func requireEnvironmentIdentityPath(root env_core.Root) string {
	path, err := root.ZitiIdentityNamed(environmentIdentityName)
	if err != nil {
		panic(err)
	}
	return path
}

func openEnvironmentAPIClient(root env_core.Root) *api.Client {
	env := root.Environment()
	return openAccountAPIClient(env.APIEndpoint, env.AccountToken)
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT)
}

func resolveTunnelByName(client *api.Client, name string, scope api.ListTunnelsScope) *api.Tunnel {
	params := api.ListTunnelsParams{}
	params.Scope.SetTo(scope)
	res, err := client.ListTunnels(context.Background(), params)
	if err != nil {
		panic(err)
	}
	tunnels, ok := res.(*api.ListTunnelsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list tunnels response: %T", res))
	}
	for _, tunnel := range *tunnels {
		if strings.EqualFold(strings.TrimSpace(tunnel.Name), strings.TrimSpace(name)) {
			return &tunnel
		}
	}
	return nil
}

func validateModeAndTarget(mode api.TunnelMode, target string) error {
	switch mode {
	case api.TunnelModeHTTP:
		u, err := url.Parse(target)
		if err != nil {
			return err
		}
		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("http backend target must include scheme and host")
		}
		return nil
	case api.TunnelModeTCP, api.TunnelModeUDP:
		_, _, err := net.SplitHostPort(target)
		return err
	default:
		return fmt.Errorf("unsupported tunnel mode '%s'", mode)
	}
}

func validateListenAddress(address string) error {
	_, _, err := net.SplitHostPort(address)
	return err
}

func runServeRuntime(ctx context.Context, identityPath string, tunnel *api.Tunnel) error {
	factory := tunnelruntime.OpenZitiFactory{}
	switch tunnel.Mode {
	case api.TunnelModeHTTP:
		return tunnelruntime.ServeHTTP(ctx, factory, identityPath, tunnel.ID, tunnel.BackendTarget)
	case api.TunnelModeTCP:
		return tunnelruntime.ServeTCP(ctx, factory, identityPath, tunnel.ID, tunnel.BackendTarget)
	case api.TunnelModeUDP:
		return tunnelruntime.ServeUDP(ctx, factory, identityPath, tunnel.ID, tunnel.BackendTarget)
	default:
		return fmt.Errorf("unsupported tunnel mode '%s'", tunnel.Mode)
	}
}

func runConnectRuntime(ctx context.Context, identityPath string, tunnel *api.Tunnel, listenAddress string) error {
	factory := tunnelruntime.OpenZitiFactory{}
	switch tunnel.Mode {
	case api.TunnelModeHTTP:
		return tunnelruntime.ConnectHTTP(ctx, factory, identityPath, tunnel.ID, listenAddress)
	case api.TunnelModeTCP:
		return tunnelruntime.ConnectTCP(ctx, factory, identityPath, tunnel.ID, listenAddress)
	case api.TunnelModeUDP:
		return tunnelruntime.ConnectUDP(ctx, factory, identityPath, tunnel.ID, listenAddress)
	default:
		return fmt.Errorf("unsupported tunnel mode '%s'", tunnel.Mode)
	}
}

func startAttachmentHeartbeats(ctx context.Context, client *api.Client, attachmentID string) {
	ticker := time.NewTicker(tunnelAttachmentHeartbeatInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := client.HeartbeatTunnelAttachment(context.Background(), api.HeartbeatTunnelAttachmentParams{AttachmentId: attachmentID}); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "warning: attachment heartbeat failed: %v\n", err)
				}
			}
		}
	}()
}

func cleanupAttachment(client *api.Client, attachmentID string) {
	res, err := client.DeleteTunnelAttachment(context.Background(), api.DeleteTunnelAttachmentParams{AttachmentId: attachmentID})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: attachment cleanup failed: %v\n", err)
		return
	}
	switch res.(type) {
	case *api.DeleteTunnelAttachmentNoContent:
		return
	case *api.DeleteTunnelAttachmentNotFound:
		return
	default:
		_, _ = fmt.Fprintf(os.Stderr, "warning: attachment cleanup returned %T\n", res)
	}
}

func startTunnelServeHeartbeats(ctx context.Context, client *api.Client, serveID string) {
	ticker := time.NewTicker(tunnelServeHeartbeatInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := client.HeartbeatTunnelServe(context.Background(), api.HeartbeatTunnelServeParams{ServeId: serveID}); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "warning: tunnel serve heartbeat failed: %v\n", err)
				}
			}
		}
	}()
}

func cleanupTunnelServe(client *api.Client, serveID string) {
	res, err := client.DeleteTunnelServe(context.Background(), api.DeleteTunnelServeParams{ServeId: serveID})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: tunnel serve cleanup failed: %v\n", err)
		return
	}
	switch res.(type) {
	case *api.DeleteTunnelServeNoContent:
		return
	case *api.DeleteTunnelServeNotFound:
		return
	default:
		_, _ = fmt.Fprintf(os.Stderr, "warning: tunnel serve cleanup returned %T\n", res)
	}
}

func ensureTunnelCreatedOrReused(client *api.Client, req *api.CreateTunnelRequest, environmentID string) *api.Tunnel {
	res, err := client.CreateTunnel(context.Background(), req)
	if err != nil {
		panic(err)
	}

	switch typed := res.(type) {
	case *api.Tunnel:
		return typed
	case *api.CreateTunnelConflict:
		existing := resolveTunnelByName(client, req.Name, api.ListTunnelsScopeAll)
		if existing == nil {
			panic(typed.Message)
		}
		if existing.EnvironmentId != environmentID {
			panic("existing tunnel is not owned by the current environment")
		}
		if existing.Mode != req.Mode || existing.BackendTarget != req.BackendTarget {
			panic("existing tunnel configuration does not match requested mode/backend")
		}
		return existing
	case *api.CreateTunnelNotFound:
		panic(typed.Message)
	case *api.CreateTunnelUnauthorized:
		panic(typed.Message)
	case *api.CreateTunnelInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected create tunnel response: %T", res))
	}
}

func findGrantByEmail(grants *api.ListTunnelGrantsResponse, email string) *api.TunnelGrant {
	for _, grant := range *grants {
		if strings.EqualFold(grant.Email, email) {
			return &grant
		}
	}
	return nil
}

func listTunnelGrantsOrPanic(client *api.Client, tunnelID string) *api.ListTunnelGrantsResponse {
	res, err := client.ListTunnelGrants(context.Background(), api.ListTunnelGrantsParams{TunnelId: tunnelID})
	if err != nil {
		panic(err)
	}
	switch typed := res.(type) {
	case *api.ListTunnelGrantsResponse:
		return typed
	case *api.ListTunnelGrantsNotFound:
		panic(typed.Message)
	case *api.ListTunnelGrantsUnauthorized:
		panic(typed.Message)
	case *api.ListTunnelGrantsInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected list tunnel grants response: %T", res))
	}
}

func isNotFoundResponse(res any) bool {
	switch res.(type) {
	case *api.DeleteTunnelAttachmentNotFound, *api.DeleteTunnelNotFound, *api.DeleteTunnelServeNotFound, *api.RemoveTunnelGrantNotFound:
		return true
	default:
		return false
	}
}

func mustResolveManagedTunnel(client *api.Client, name string) *api.Tunnel {
	tunnel := resolveTunnelByName(client, name, api.ListTunnelsScopeAll)
	if tunnel == nil {
		panic(fmt.Sprintf("tunnel '%s' not found", name))
	}
	return tunnel
}

func ignoreConflictGrantAdd(client *api.Client, tunnelID, email string) {
	res, err := client.AddTunnelGrant(context.Background(), &api.AddTunnelGrantRequest{Email: email}, api.AddTunnelGrantParams{TunnelId: tunnelID})
	if err != nil {
		panic(err)
	}
	switch res.(type) {
	case *api.TunnelGrant:
		return
	case *api.AddTunnelGrantConflict:
		return
	default:
		panic(fmt.Sprintf("unexpected add tunnel grant response: %T", res))
	}
}

func optionalString(v api.OptString) string {
	value, _ := v.Get()
	return value
}

func optionalTime(v api.OptDateTime) string {
	value, ok := v.Get()
	if !ok {
		return "-"
	}
	return value.UTC().Format("2006-01-02 15:04:05")
}

func panicIfUnsupportedMode(mode string) {
	switch mode {
	case string(api.TunnelModeHTTP), string(api.TunnelModeTCP), string(api.TunnelModeUDP):
		return
	default:
		panic(fmt.Sprintf("unsupported tunnel mode '%s'", mode))
	}
}

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

func ensureResponseNoContent(res any) {
	if isNotFoundResponse(res) {
		return
	}
	switch res.(type) {
	case *api.DeleteTunnelNoContent, *api.RemoveTunnelGrantNoContent, *api.DeleteTunnelAttachmentNoContent, *api.DeleteTunnelServeNoContent, *api.HeartbeatTunnelAttachmentNoContent, *api.HeartbeatTunnelServeNoContent:
		return
	default:
		panic(fmt.Sprintf("unexpected no-content response: %T", res))
	}
}

func ignoreNotFound(err error, res any) error {
	if err != nil {
		return err
	}
	if isNotFoundResponse(res) {
		return nil
	}
	return nil
}

func isClosedError(err error) bool {
	return errors.Is(err, net.ErrClosed)
}
