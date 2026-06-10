package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/network/tunnelruntime"
	"github.com/openziti/agora/sdk/agent"
)

const (
	createOp                  = "tunnel.Create"
	deleteOp                  = "tunnel.Delete"
	getOp                     = "tunnel.Get"
	listenOp                  = "tunnel.Listen"
	attachOp                  = "tunnel.Attach"
	detachOp                  = "tunnel.Detach"
	dialOp                    = "tunnel.Dial"
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
	identityPath() string
}

type primitiveController interface {
	CreateTunnel(context.Context, *api.CreateTunnelRequest) (api.CreateTunnelRes, error)
	DeleteTunnel(context.Context, api.DeleteTunnelParams) (api.DeleteTunnelRes, error)
	GetTunnel(context.Context, api.GetTunnelParams) (api.GetTunnelRes, error)
	ListTunnels(context.Context, api.ListTunnelsParams) (api.ListTunnelsRes, error)
	ConnectTunnel(context.Context, *api.ConnectTunnelRequest) (api.ConnectTunnelRes, error)
	GetActiveDialerAttachment(context.Context, api.GetActiveDialerAttachmentParams) (api.GetActiveDialerAttachmentRes, error)
	DetachDialerAttachment(context.Context, api.DetachDialerAttachmentParams) (api.DetachDialerAttachmentRes, error)
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

func (a realPrimitiveAgent) identityPath() string {
	return a.agent.EnvironmentIdentityPath()
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

// Get resolves a direct tunnel record by name or ID without opening a
// listener or attachment. It returns ErrNotFound when no tunnel matches,
// letting callers inspect a tunnel's mode (e.g. before binding to an
// operator-provisioned share).
func Get(ctx context.Context, a *agent.Agent, nameOrID string) (*Tunnel, error) {
	if a == nil {
		return nil, invalidSpec("%s: agent is required", getOp)
	}
	return get(ctx, realPrimitiveAgent{agent: a}.controller(), nameOrID)
}

func get(ctx context.Context, controller primitiveController, nameOrID string) (*Tunnel, error) {
	ref := strings.TrimSpace(nameOrID)
	if ref == "" {
		return nil, invalidSpec("%s: tunnel name or id is required", getOp)
	}
	apiTunnel, err := resolveListenTunnel(ctx, controller, ref)
	if err != nil {
		return nil, err
	}
	return fromAPITunnel(apiTunnel), nil
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

	identityPath := strings.TrimSpace(a.identityPath())
	if identityPath == "" {
		return nil, fmt.Errorf("%s: locate environment identity: %w", listenOp, ErrTransient)
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

// Attach provisions a direct dialer attachment for the agent's environment.
// The attachment persists until Detach is called.
func Attach(ctx context.Context, a *agent.Agent, nameOrID string) (*Attachment, error) {
	if a == nil {
		return nil, invalidSpec("%s: agent is required", attachOp)
	}
	return attach(ctx, realPrimitiveAgent{agent: a}, nameOrID)
}

func attach(ctx context.Context, a primitiveAgent, nameOrID string) (*Attachment, error) {
	ref := strings.TrimSpace(nameOrID)
	if ref == "" {
		return nil, invalidSpec("%s: tunnel name or id is required", attachOp)
	}
	environmentID := a.environmentID()
	if environmentID == "" {
		return nil, invalidSpec("%s: environment id is required", attachOp)
	}

	res, err := a.controller().ConnectTunnel(ctx, &api.ConnectTunnelRequest{
		EnvironmentId: environmentID,
		Name:          ref,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %v: %w", attachOp, err, ErrTransient)
	}
	switch typed := res.(type) {
	case *api.ConnectTunnelResponse:
		return fromAPIAttachment(&typed.Attachment), nil
	case *api.ConnectTunnelNotFound:
		return nil, fmt.Errorf("%s: %s: %w", attachOp, typed.Message, ErrNotFound)
	case *api.ConnectTunnelConflict:
		return nil, fmt.Errorf("%s: %s: %w", attachOp, typed.Message, ErrConflict)
	case *api.ConnectTunnelUnauthorized:
		return nil, fmt.Errorf("%s: %s: %w", attachOp, typed.Message, ErrInvalidSpec)
	case *api.ConnectTunnelInternalServerError:
		return nil, fmt.Errorf("%s: %s: %w", attachOp, typed.Message, ErrTransient)
	default:
		return nil, fmt.Errorf("%s: unexpected connect tunnel response %T: %w", attachOp, res, ErrTransient)
	}
}

// Detach removes a direct dialer attachment by natural key. The target may be
// a tunnel name/ID string or the Attachment returned by Attach.
func Detach(ctx context.Context, a *agent.Agent, target any) error {
	if a == nil {
		return invalidSpec("%s: agent is required", detachOp)
	}
	return detach(ctx, realPrimitiveAgent{agent: a}, target)
}

func detach(ctx context.Context, a primitiveAgent, target any) error {
	ref, err := detachTargetRef(target)
	if err != nil {
		return err
	}
	environmentID := a.environmentID()
	if environmentID == "" {
		return invalidSpec("%s: environment id is required", detachOp)
	}

	res, err := a.controller().DetachDialerAttachment(ctx, api.DetachDialerAttachmentParams{
		EnvironmentId: environmentID,
		Tunnel:        ref,
	})
	if err != nil {
		return fmt.Errorf("%s: %v: %w", detachOp, err, ErrTransient)
	}
	switch typed := res.(type) {
	case *api.DetachDialerAttachmentNoContent:
		return nil
	case *api.DetachDialerAttachmentNotFound:
		return fmt.Errorf("%s: %s: %w", detachOp, typed.Message, ErrNotFound)
	case *api.DetachDialerAttachmentConflict:
		return fmt.Errorf("%s: %s: %w", detachOp, typed.Message, ErrConflict)
	case *api.DetachDialerAttachmentForbidden:
		return fmt.Errorf("%s: %s: %w", detachOp, typed.Message, ErrInvalidSpec)
	case *api.DetachDialerAttachmentUnauthorized:
		return fmt.Errorf("%s: %s: %w", detachOp, typed.Message, ErrInvalidSpec)
	case *api.DetachDialerAttachmentInternalServerError:
		return fmt.Errorf("%s: %s: %w", detachOp, typed.Message, ErrTransient)
	default:
		return fmt.Errorf("%s: unexpected detach dialer response %T: %w", detachOp, res, ErrTransient)
	}
}

// Dial opens a raw overlay connection for an active direct dialer attachment.
func Dial(ctx context.Context, a *agent.Agent, nameOrID string) (net.Conn, error) {
	if a == nil {
		return nil, invalidSpec("%s: agent is required", dialOp)
	}
	return dial(ctx, realPrimitiveAgent{agent: a}, defaultOverlayFactory, nameOrID)
}

func dial(ctx context.Context, a primitiveAgent, factory overlayFactory, nameOrID string) (net.Conn, error) {
	ref := strings.TrimSpace(nameOrID)
	if ref == "" {
		return nil, invalidSpec("%s: tunnel name or id is required", dialOp)
	}
	environmentID := a.environmentID()
	if environmentID == "" {
		return nil, invalidSpec("%s: environment id is required", dialOp)
	}

	active, err := getActiveDialerAttachment(ctx, a.controller(), environmentID, ref)
	if err != nil {
		return nil, err
	}
	switch active.TunnelMode {
	case api.TunnelModeHTTP, api.TunnelModeTCP:
	case api.TunnelModeUDP:
		return nil, unsupportedMode("%s: tunnel '%s' uses packet mode '%s'", dialOp, ref, active.TunnelMode)
	default:
		return nil, unsupportedMode("%s: tunnel '%s' has unsupported mode '%s'", dialOp, ref, active.TunnelMode)
	}

	identityPath := strings.TrimSpace(a.identityPath())
	if identityPath == "" {
		return nil, fmt.Errorf("%s: locate environment identity: %w", dialOp, ErrTransient)
	}
	overlay, err := factory.New(identityPath)
	if err != nil {
		return nil, fmt.Errorf("%s: open overlay context: %v: %w", dialOp, err, ErrTransient)
	}
	conn, err := overlay.DialContext(ctx, active.TunnelId)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s: dial tunnel '%s': %w", dialOp, active.TunnelId, err)
		}
		return nil, fmt.Errorf("%s: dial tunnel '%s': %v: %w", dialOp, active.TunnelId, err, ErrTransient)
	}
	return conn, nil
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

func getActiveDialerAttachment(ctx context.Context, controller primitiveController, environmentID, ref string) (*api.ActiveDialerAttachment, error) {
	res, err := controller.GetActiveDialerAttachment(ctx, api.GetActiveDialerAttachmentParams{
		EnvironmentId: environmentID,
		Tunnel:        ref,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: resolve dialer attachment '%s': %v: %w", dialOp, ref, err, ErrTransient)
	}
	switch typed := res.(type) {
	case *api.ActiveDialerAttachment:
		return typed, nil
	case *api.GetActiveDialerAttachmentNotFound:
		return nil, fmt.Errorf("%s: %s: %w", dialOp, typed.Message, ErrNotFound)
	case *api.GetActiveDialerAttachmentConflict:
		return nil, fmt.Errorf("%s: %s: %w", dialOp, typed.Message, ErrConflict)
	case *api.GetActiveDialerAttachmentForbidden:
		return nil, fmt.Errorf("%s: %s: %w", dialOp, typed.Message, ErrInvalidSpec)
	case *api.GetActiveDialerAttachmentUnauthorized:
		return nil, fmt.Errorf("%s: %s: %w", dialOp, typed.Message, ErrInvalidSpec)
	case *api.GetActiveDialerAttachmentInternalServerError:
		return nil, fmt.Errorf("%s: %s: %w", dialOp, typed.Message, ErrTransient)
	default:
		return nil, fmt.Errorf("%s: unexpected get active dialer response %T: %w", dialOp, res, ErrTransient)
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

func fromAPIAttachment(attachment *api.TunnelAttachment) *Attachment {
	if attachment == nil {
		return nil
	}
	return &Attachment{
		ID:            attachment.ID,
		TunnelID:      attachment.TunnelId,
		EnvironmentID: attachment.EnvironmentId,
		Kind:          AttachmentKind(attachment.Kind),
	}
}

func detachTargetRef(target any) (string, error) {
	switch typed := target.(type) {
	case string:
		ref := strings.TrimSpace(typed)
		if ref == "" {
			return "", invalidSpec("%s: tunnel name or id is required", detachOp)
		}
		return ref, nil
	case *Attachment:
		if typed == nil {
			return "", invalidSpec("%s: attachment is required", detachOp)
		}
		if strings.TrimSpace(typed.TunnelID) == "" {
			return "", invalidSpec("%s: attachment tunnel id is required", detachOp)
		}
		return strings.TrimSpace(typed.TunnelID), nil
	case Attachment:
		if strings.TrimSpace(typed.TunnelID) == "" {
			return "", invalidSpec("%s: attachment tunnel id is required", detachOp)
		}
		return strings.TrimSpace(typed.TunnelID), nil
	default:
		return "", invalidSpec("%s: tunnel name/id or attachment is required", detachOp)
	}
}

var _ primitiveController = (*api.Client)(nil)
var _ primitiveAgent = realPrimitiveAgent{}
