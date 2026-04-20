package agent

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
)

type accountSecuritySource struct {
	accountToken string
}

func (s accountSecuritySource) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	if s.accountToken == "" {
		return api.AccountTokenAuth{}, fmt.Errorf("account token auth not configured")
	}
	return api.AccountTokenAuth{APIKey: s.accountToken}, nil
}

func (accountSecuritySource) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{}, nil
}

type tunnelController interface {
	StartServe(context.Context, *env_core.Environment, env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error)
	HeartbeatServe(context.Context, *env_core.Environment, string) error
	StopServe(context.Context, *env_core.Environment, string) error
	StartConnect(context.Context, *env_core.Environment, env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error)
	HeartbeatAttachment(context.Context, *env_core.Environment, string) error
	StopAttachment(context.Context, *env_core.Environment, string) error
}

type apiTunnelController struct{}

func newAccountAPIClient(apiEndpoint, accountToken string) (*api.Client, error) {
	baseURL := strings.TrimRight(apiEndpoint, "/") + "/v1"
	return api.NewClient(baseURL, accountSecuritySource{accountToken: accountToken}, api.WithClient(http.DefaultClient))
}

func (apiTunnelController) StartServe(ctx context.Context, env *env_core.Environment, desired env_core.ManagedServe) (*api.Tunnel, *api.TunnelServe, error) {
	client, err := newControllerClient(env)
	if err != nil {
		return nil, nil, err
	}

	tunnel, err := ensureTunnelCreatedOrReused(client, &api.CreateTunnelRequest{
		EnvironmentId: env.EnvironmentID,
		Name:          desired.Name,
		Mode:          api.TunnelMode(desired.Mode),
		BackendTarget: desired.BackendTarget,
		GrantEmails:   append([]string(nil), desired.GrantEmails...),
	}, env.EnvironmentID)
	if err != nil {
		return nil, nil, err
	}
	for _, email := range desired.GrantEmails {
		if err := ignoreConflictGrantAdd(client, tunnel.ID, email); err != nil {
			return tunnel, nil, err
		}
	}

	res, err := client.StartTunnelServe(ctx, &api.StartTunnelServeRequest{
		EnvironmentId: env.EnvironmentID,
	}, api.StartTunnelServeParams{TunnelId: tunnel.ID})
	if err != nil {
		return tunnel, nil, err
	}

	switch typed := res.(type) {
	case *api.TunnelServe:
		return tunnel, typed, nil
	case *api.StartTunnelServeNotFound:
		return tunnel, nil, fmt.Errorf("%s", typed.Message)
	case *api.StartTunnelServeConflict:
		return tunnel, nil, fmt.Errorf("%s", typed.Message)
	case *api.StartTunnelServeUnauthorized:
		return tunnel, nil, fmt.Errorf("%s", typed.Message)
	case *api.StartTunnelServeInternalServerError:
		return tunnel, nil, fmt.Errorf("%s", typed.Message)
	default:
		return tunnel, nil, fmt.Errorf("unexpected start tunnel serve response: %T", res)
	}
}

func (apiTunnelController) HeartbeatServe(ctx context.Context, env *env_core.Environment, serveID string) error {
	client, err := newControllerClient(env)
	if err != nil {
		return err
	}
	res, err := client.HeartbeatTunnelServe(ctx, api.HeartbeatTunnelServeParams{ServeId: serveID})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.HeartbeatTunnelServeNoContent:
		return nil
	case *api.HeartbeatTunnelServeNotFound:
		return fmt.Errorf("%s", typed.Message)
	case *api.HeartbeatTunnelServeUnauthorized:
		return fmt.Errorf("%s", typed.Message)
	case *api.HeartbeatTunnelServeInternalServerError:
		return fmt.Errorf("%s", typed.Message)
	default:
		return fmt.Errorf("unexpected tunnel serve heartbeat response: %T", res)
	}
}

func (apiTunnelController) StopServe(ctx context.Context, env *env_core.Environment, serveID string) error {
	client, err := newControllerClient(env)
	if err != nil {
		return err
	}
	res, err := client.DeleteTunnelServe(ctx, api.DeleteTunnelServeParams{ServeId: serveID})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.DeleteTunnelServeNoContent, *api.DeleteTunnelServeNotFound:
		return nil
	case *api.DeleteTunnelServeUnauthorized:
		return fmt.Errorf("%s", typed.Message)
	case *api.DeleteTunnelServeInternalServerError:
		return fmt.Errorf("%s", typed.Message)
	default:
		return fmt.Errorf("unexpected delete tunnel serve response: %T", res)
	}
}

func (apiTunnelController) StartConnect(ctx context.Context, env *env_core.Environment, desired env_core.ManagedConnect) (*api.Tunnel, *api.TunnelAttachment, error) {
	client, err := newControllerClient(env)
	if err != nil {
		return nil, nil, err
	}
	res, err := client.ConnectTunnel(ctx, &api.ConnectTunnelRequest{
		EnvironmentId: env.EnvironmentID,
		Name:          desired.Name,
		ListenAddress: desired.ListenAddress,
	})
	if err != nil {
		return nil, nil, err
	}
	switch typed := res.(type) {
	case *api.ConnectTunnelResponse:
		return &typed.Tunnel, &typed.Attachment, nil
	case *api.ConnectTunnelNotFound:
		return nil, nil, fmt.Errorf("%s", typed.Message)
	case *api.ConnectTunnelConflict:
		return nil, nil, fmt.Errorf("%s", typed.Message)
	case *api.ConnectTunnelUnauthorized:
		return nil, nil, fmt.Errorf("%s", typed.Message)
	case *api.ConnectTunnelInternalServerError:
		return nil, nil, fmt.Errorf("%s", typed.Message)
	default:
		return nil, nil, fmt.Errorf("unexpected connect tunnel response: %T", res)
	}
}

func (apiTunnelController) HeartbeatAttachment(ctx context.Context, env *env_core.Environment, attachmentID string) error {
	client, err := newControllerClient(env)
	if err != nil {
		return err
	}
	res, err := client.HeartbeatTunnelAttachment(ctx, api.HeartbeatTunnelAttachmentParams{AttachmentId: attachmentID})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.HeartbeatTunnelAttachmentNoContent:
		return nil
	case *api.HeartbeatTunnelAttachmentNotFound:
		return fmt.Errorf("%s", typed.Message)
	case *api.HeartbeatTunnelAttachmentUnauthorized:
		return fmt.Errorf("%s", typed.Message)
	case *api.HeartbeatTunnelAttachmentInternalServerError:
		return fmt.Errorf("%s", typed.Message)
	default:
		return fmt.Errorf("unexpected attachment heartbeat response: %T", res)
	}
}

func (apiTunnelController) StopAttachment(ctx context.Context, env *env_core.Environment, attachmentID string) error {
	client, err := newControllerClient(env)
	if err != nil {
		return err
	}
	res, err := client.DeleteTunnelAttachment(ctx, api.DeleteTunnelAttachmentParams{AttachmentId: attachmentID})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.DeleteTunnelAttachmentNoContent, *api.DeleteTunnelAttachmentNotFound:
		return nil
	case *api.DeleteTunnelAttachmentUnauthorized:
		return fmt.Errorf("%s", typed.Message)
	case *api.DeleteTunnelAttachmentInternalServerError:
		return fmt.Errorf("%s", typed.Message)
	default:
		return fmt.Errorf("unexpected delete tunnel attachment response: %T", res)
	}
}

func newControllerClient(env *env_core.Environment) (*api.Client, error) {
	if env == nil {
		return nil, fmt.Errorf("no environment is enabled")
	}
	if env.APIEndpoint == "" {
		return nil, fmt.Errorf("enabled environment is missing api endpoint")
	}
	if env.AccountToken == "" {
		return nil, fmt.Errorf("enabled environment is missing account token")
	}
	return newAccountAPIClient(env.APIEndpoint, env.AccountToken)
}

func ensureTunnelCreatedOrReused(client *api.Client, req *api.CreateTunnelRequest, environmentID string) (*api.Tunnel, error) {
	res, err := client.CreateTunnel(context.Background(), req)
	if err != nil {
		return nil, err
	}

	switch typed := res.(type) {
	case *api.Tunnel:
		return typed, nil
	case *api.CreateTunnelConflict:
		existing, err := resolveTunnelByName(client, req.Name, api.ListTunnelsScopeAll)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, fmt.Errorf("%s", typed.Message)
		}
		if existing.EnvironmentId != environmentID {
			return nil, fmt.Errorf("existing tunnel is not owned by the current environment")
		}
		if existing.Mode != req.Mode || existing.BackendTarget != req.BackendTarget {
			return nil, fmt.Errorf("existing tunnel configuration does not match requested mode/backend")
		}
		return existing, nil
	case *api.CreateTunnelNotFound:
		return nil, fmt.Errorf("%s", typed.Message)
	case *api.CreateTunnelUnauthorized:
		return nil, fmt.Errorf("%s", typed.Message)
	case *api.CreateTunnelInternalServerError:
		return nil, fmt.Errorf("%s", typed.Message)
	default:
		return nil, fmt.Errorf("unexpected create tunnel response: %T", res)
	}
}

func ignoreConflictGrantAdd(client *api.Client, tunnelID, email string) error {
	res, err := client.AddTunnelGrant(context.Background(), &api.AddTunnelGrantRequest{Email: email}, api.AddTunnelGrantParams{TunnelId: tunnelID})
	if err != nil {
		return err
	}
	switch res.(type) {
	case *api.TunnelGrant, *api.AddTunnelGrantConflict:
		return nil
	default:
		return fmt.Errorf("unexpected add tunnel grant response: %T", res)
	}
}

func resolveTunnelByName(client *api.Client, name string, scope api.ListTunnelsScope) (*api.Tunnel, error) {
	params := api.ListTunnelsParams{}
	params.Scope.SetTo(scope)
	res, err := client.ListTunnels(context.Background(), params)
	if err != nil {
		return nil, err
	}
	tunnels, ok := res.(*api.ListTunnelsResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected list tunnels response: %T", res)
	}
	for _, tunnel := range *tunnels {
		if strings.EqualFold(strings.TrimSpace(tunnel.Name), strings.TrimSpace(name)) {
			return &tunnel, nil
		}
	}
	return nil, nil
}
