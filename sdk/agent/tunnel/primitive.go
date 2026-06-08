package tunnel

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/network/tunnelruntime"
	"github.com/openziti/agora/sdk/agent"
)

const (
	createOp                  = "tunnel.Create"
	deleteOp                  = "tunnel.Delete"
	listenOp                  = "tunnel.Listen"
	environmentIdentityName   = "environment"
	tunnelIDPatternExpression = `^tt_[a-z0-9]{12}$`
)

var (
	tunnelIDPattern                      = regexp.MustCompile(tunnelIDPatternExpression)
	defaultOverlayFactory overlayFactory = tunnelruntime.OpenZitiFactory{}
)

type primitiveAgent interface {
	controller() primitiveController
	environmentID() string
	envRoot() identityRoot
}

type primitiveController interface {
	CreateTunnel(context.Context, *api.CreateTunnelRequest) (api.CreateTunnelRes, error)
	DeleteTunnel(context.Context, api.DeleteTunnelParams) (api.DeleteTunnelRes, error)
	GetTunnel(context.Context, api.GetTunnelParams) (api.GetTunnelRes, error)
	ListTunnels(context.Context, api.ListTunnelsParams) (api.ListTunnelsRes, error)
}

type identityRoot interface {
	ZitiIdentityNamed(name string) (string, error)
}

type overlayFactory interface {
	New(identityPath string) (tunnelruntime.OverlayContext, error)
}

type realPrimitiveAgent struct {
	agent *agent.Agent
}

func (a realPrimitiveAgent) controller() primitiveController {
	return a.agent.Controller()
}

func (a realPrimitiveAgent) environmentID() string {
	env := a.agent.Environment()
	if env == nil {
		return ""
	}
	return env.EnvironmentID
}

func (a realPrimitiveAgent) envRoot() identityRoot {
	return a.agent.EnvRoot()
}

// Create provisions a direct tunnel record and its bind policy. The
// returned tunnel persists until Delete is called.
func Create(ctx context.Context, a *agent.Agent, spec Spec) (*Tunnel, error) {
	if a == nil {
		return nil, invalidSpec("%s: agent is required", createOp)
	}
	return create(ctx, realPrimitiveAgent{agent: a}, spec)
}

func create(ctx context.Context, a primitiveAgent, spec Spec) (*Tunnel, error) {
	spec = normalizeSpec(spec)
	if spec.EnvironmentID == "" {
		spec.EnvironmentID = a.environmentID()
	}
	if err := validateSpec(spec); err != nil {
		return nil, err
	}

	res, err := a.controller().CreateTunnel(ctx, &api.CreateTunnelRequest{
		EnvironmentId: spec.EnvironmentID,
		Name:          spec.Name,
		Mode:          api.TunnelMode(spec.Mode),
		GrantEmails:   append([]string(nil), spec.GrantEmails...),
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %v: %w", createOp, err, ErrTransient)
	}

	switch typed := res.(type) {
	case *api.Tunnel:
		return fromAPITunnel(typed), nil
	case *api.CreateTunnelConflict:
		return nil, fmt.Errorf("%s: %s: %w", createOp, typed.Message, ErrConflict)
	case *api.CreateTunnelNotFound:
		return nil, fmt.Errorf("%s: %s: %w", createOp, typed.Message, ErrNotFound)
	case *api.CreateTunnelUnauthorized:
		return nil, fmt.Errorf("%s: %s: %w", createOp, typed.Message, ErrInvalidSpec)
	case *api.CreateTunnelInternalServerError:
		return nil, fmt.Errorf("%s: %s: %w", createOp, typed.Message, ErrTransient)
	default:
		return nil, fmt.Errorf("%s: unexpected create tunnel response %T: %w", createOp, res, ErrTransient)
	}
}

// Delete removes a direct tunnel record and its controller-owned fabric
// policy. The tunnel must have been provisioned with Create or another
// controller API call.
func Delete(ctx context.Context, a *agent.Agent, tunnel *Tunnel) error {
	if a == nil {
		return invalidSpec("%s: agent is required", deleteOp)
	}
	return deleteTunnel(ctx, realPrimitiveAgent{agent: a}, tunnel)
}

func deleteTunnel(ctx context.Context, a primitiveAgent, tunnel *Tunnel) error {
	if tunnel == nil || strings.TrimSpace(tunnel.ID) == "" {
		return invalidSpec("%s: tunnel id is required", deleteOp)
	}
	res, err := a.controller().DeleteTunnel(ctx, api.DeleteTunnelParams{TunnelId: strings.TrimSpace(tunnel.ID)})
	if err != nil {
		return fmt.Errorf("%s: %v: %w", deleteOp, err, ErrTransient)
	}
	switch typed := res.(type) {
	case *api.DeleteTunnelNoContent:
		return nil
	case *api.DeleteTunnelNotFound:
		return fmt.Errorf("%s: %s: %w", deleteOp, typed.Message, ErrNotFound)
	case *api.DeleteTunnelUnauthorized:
		return fmt.Errorf("%s: %s: %w", deleteOp, typed.Message, ErrInvalidSpec)
	case *api.DeleteTunnelInternalServerError:
		return fmt.Errorf("%s: %s: %w", deleteOp, typed.Message, ErrTransient)
	default:
		return fmt.Errorf("%s: unexpected delete tunnel response %T: %w", deleteOp, res, ErrTransient)
	}
}

// Listen resolves a direct tunnel by name or ID and returns the raw
// overlay listener for the tunnel's service ID. It does not start the
// managed runtime, create a serve record, heartbeat, or wrap protocol
// serving.
func Listen(ctx context.Context, a *agent.Agent, nameOrID string) (net.Listener, error) {
	if a == nil {
		return nil, invalidSpec("%s: agent is required", listenOp)
	}
	return listen(ctx, realPrimitiveAgent{agent: a}, defaultOverlayFactory, nameOrID)
}

func listen(ctx context.Context, a primitiveAgent, factory overlayFactory, nameOrID string) (net.Listener, error) {
	ref := strings.TrimSpace(nameOrID)
	if ref == "" {
		return nil, invalidSpec("%s: tunnel name or id is required", listenOp)
	}
	tunnel, err := resolveListenTunnel(ctx, a.controller(), ref)
	if err != nil {
		return nil, err
	}
	if err := validateListenTunnel(tunnel, a.environmentID()); err != nil {
		return nil, err
	}

	identityPath, err := a.envRoot().ZitiIdentityNamed(environmentIdentityName)
	if err != nil {
		return nil, fmt.Errorf("%s: locate environment identity: %v: %w", listenOp, err, ErrTransient)
	}
	overlay, err := factory.New(identityPath)
	if err != nil {
		return nil, fmt.Errorf("%s: open overlay context: %v: %w", listenOp, err, ErrTransient)
	}
	listener, err := overlay.Listen(tunnel.ID)
	if err != nil {
		return nil, fmt.Errorf("%s: listen on tunnel '%s': %v: %w", listenOp, tunnel.ID, err, ErrTransient)
	}
	return listener, nil
}

func resolveListenTunnel(ctx context.Context, controller primitiveController, ref string) (*api.Tunnel, error) {
	if tunnelIDPattern.MatchString(ref) {
		res, err := controller.GetTunnel(ctx, api.GetTunnelParams{TunnelId: ref})
		if err != nil {
			return nil, fmt.Errorf("%s: resolve tunnel id '%s': %v: %w", listenOp, ref, err, ErrTransient)
		}
		switch typed := res.(type) {
		case *api.Tunnel:
			return typed, nil
		case *api.GetTunnelNotFound:
			return nil, fmt.Errorf("%s: tunnel '%s' not found: %w", listenOp, ref, ErrNotFound)
		case *api.GetTunnelUnauthorized:
			return nil, fmt.Errorf("%s: %s: %w", listenOp, typed.Message, ErrInvalidSpec)
		case *api.GetTunnelInternalServerError:
			return nil, fmt.Errorf("%s: %s: %w", listenOp, typed.Message, ErrTransient)
		default:
			return nil, fmt.Errorf("%s: unexpected get tunnel response %T: %w", listenOp, res, ErrTransient)
		}
	}

	params := api.ListTunnelsParams{}
	params.Scope.SetTo(api.ListTunnelsScopeAll)
	res, err := controller.ListTunnels(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve tunnel name '%s': %v: %w", listenOp, ref, err, ErrTransient)
	}
	switch typed := res.(type) {
	case *api.ListTunnelsResponse:
		for i := range *typed {
			tunnel := &(*typed)[i]
			if strings.EqualFold(strings.TrimSpace(tunnel.Name), ref) {
				return tunnel, nil
			}
		}
		return nil, fmt.Errorf("%s: tunnel '%s' not found: %w", listenOp, ref, ErrNotFound)
	case *api.ListTunnelsUnauthorized:
		return nil, fmt.Errorf("%s: %s: %w", listenOp, typed.Message, ErrInvalidSpec)
	case *api.ListTunnelsInternalServerError:
		return nil, fmt.Errorf("%s: %s: %w", listenOp, typed.Message, ErrTransient)
	default:
		return nil, fmt.Errorf("%s: unexpected list tunnels response %T: %w", listenOp, res, ErrTransient)
	}
}

func validateListenTunnel(tunnel *api.Tunnel, environmentID string) error {
	if tunnel.Kind != api.TunnelKindDirect {
		return invalidSpec("%s: tunnel '%s' is not direct", listenOp, tunnel.Name)
	}
	switch tunnel.Mode {
	case api.TunnelModeHTTP, api.TunnelModeTCP:
	case api.TunnelModeUDP:
		return unsupportedMode("%s: tunnel '%s' uses packet mode '%s'", listenOp, tunnel.Name, tunnel.Mode)
	default:
		return unsupportedMode("%s: tunnel '%s' has unsupported mode '%s'", listenOp, tunnel.Name, tunnel.Mode)
	}
	if tunnel.EnvironmentId != environmentID {
		return invalidSpec("%s: tunnel '%s' is bound to environment '%s'", listenOp, tunnel.Name, tunnel.EnvironmentId)
	}
	if !tunnel.ZitiServiceId.Set || !tunnel.BindPolicyId.Set {
		return fmt.Errorf("%s: tunnel '%s' is missing service metadata: %w", listenOp, tunnel.Name, ErrTransient)
	}
	return nil
}

func normalizeSpec(spec Spec) Spec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Mode = Mode(strings.ToLower(strings.TrimSpace(string(spec.Mode))))
	spec.EnvironmentID = strings.TrimSpace(spec.EnvironmentID)
	grants := make([]string, 0, len(spec.GrantEmails))
	for _, email := range spec.GrantEmails {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		grants = append(grants, email)
	}
	spec.GrantEmails = grants
	return spec
}

func validateSpec(spec Spec) error {
	if spec.Name == "" {
		return invalidSpec("%s: tunnel name is required", createOp)
	}
	if spec.EnvironmentID == "" {
		return invalidSpec("%s: environment id is required", createOp)
	}
	switch spec.Mode {
	case ModeHTTP, ModeTCP, ModeUDP:
		return nil
	case "":
		return invalidSpec("%s: tunnel mode is required", createOp)
	default:
		return unsupportedMode("%s: unsupported tunnel mode '%s'", createOp, spec.Mode)
	}
}

func fromAPITunnel(tunnel *api.Tunnel) *Tunnel {
	if tunnel == nil {
		return nil
	}
	return &Tunnel{
		ID:   tunnel.ID,
		Name: tunnel.Name,
		Kind: Kind(tunnel.Kind),
		Mode: Mode(tunnel.Mode),
	}
}

var _ primitiveController = (*api.Client)(nil)
var _ identityRoot = (env_core.Root)(nil)
