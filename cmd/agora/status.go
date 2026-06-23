package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"github.com/spf13/cobra"
)

const (
	statusControllerRequestTimeout = 5 * time.Second
	statusEnvironmentLivenessTTL   = 45 * time.Second
)

const (
	controllerEnvironmentLivenessDisabled = "disabled"
	controllerEnvironmentLivenessOnline   = "online"
	controllerEnvironmentLivenessStale    = "stale"
	controllerEnvironmentLivenessUnknown  = "unknown"
)

type statusCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

type statusBuilder struct {
	now           func() time.Time
	networkStatus func(env_core.Root) (*networkpb.NetworkStatus, error)
	newClient     func(apiEndpoint, accountToken string) (*api.Client, error)
}

type compositeStatus struct {
	Local      localStatus      `json:"local"`
	Controller controllerStatus `json:"controller"`
}

type localStatus struct {
	APIEndpoint     string             `json:"api_endpoint,omitempty"`
	RootEnvironment *localEnvironment  `json:"root_environment,omitempty"`
	RootNetwork     *localDesiredState `json:"root_network,omitempty"`
	AgentRunning    bool               `json:"agent_running"`
	AgentError      string             `json:"agent_error,omitempty"`
	AgentStatus     *localAgentStatus  `json:"agent_status,omitempty"`
}

type localEnvironment struct {
	EnvironmentID string `json:"environment_id"`
	ZitiIdentity  string `json:"ziti_identity,omitempty"`
}

type localDesiredState struct {
	Serves   []desiredServe   `json:"serves,omitempty"`
	Connects []desiredConnect `json:"connects,omitempty"`
}

type localAgentStatus struct {
	SocketPath           string                     `json:"socket_path"`
	PID                  int32                      `json:"pid"`
	StartedAt            time.Time                  `json:"started_at"`
	EnvironmentEnabled   bool                       `json:"environment_enabled"`
	EnvironmentID        string                     `json:"environment_id,omitempty"`
	EnvironmentHeartbeat *environmentHeartbeatState `json:"environment_heartbeat,omitempty"`
	Serves               []managedServeStatus       `json:"serves,omitempty"`
	Connects             []managedConnectStatus     `json:"connects,omitempty"`
}

type environmentHeartbeatState struct {
	State         string     `json:"state"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type desiredServe struct {
	TunnelID      string   `json:"tunnel_id,omitempty"`
	Name          string   `json:"name"`
	Mode          string   `json:"mode"`
	BackendTarget string   `json:"backend_target"`
	GrantEmails   []string `json:"grant_emails,omitempty"`
}

type desiredConnect struct {
	TunnelID      string `json:"tunnel_id,omitempty"`
	Name          string `json:"name"`
	ListenAddress string `json:"listen_address"`
}

type managedServeStatus struct {
	Desired       desiredServe `json:"desired"`
	State         string       `json:"state"`
	TunnelID      string       `json:"tunnel_id,omitempty"`
	ServeID       string       `json:"serve_id,omitempty"`
	LastError     string       `json:"last_error,omitempty"`
	LastStartedAt *time.Time   `json:"last_started_at,omitempty"`
	NextRetryAt   *time.Time   `json:"next_retry_at,omitempty"`
	RetryAttempt  uint32       `json:"retry_attempt,omitempty"`
}

type managedConnectStatus struct {
	Desired       desiredConnect `json:"desired"`
	State         string         `json:"state"`
	TunnelID      string         `json:"tunnel_id,omitempty"`
	AttachmentID  string         `json:"attachment_id,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	LastStartedAt *time.Time     `json:"last_started_at,omitempty"`
	NextRetryAt   *time.Time     `json:"next_retry_at,omitempty"`
	RetryAttempt  uint32         `json:"retry_attempt,omitempty"`
}

type controllerStatus struct {
	Reachable           bool                    `json:"reachable"`
	Error               string                  `json:"error,omitempty"`
	Environment         *controllerEnvironment  `json:"environment,omitempty"`
	EnvironmentLiveness string                  `json:"environment_liveness,omitempty"`
	OwnedTunnels        []controllerOwnedTunnel `json:"owned_tunnels,omitempty"`
}

type controllerEnvironment struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	AccountID      string     `json:"account_id"`
	Description    *string    `json:"description,omitempty"`
	Host           *string    `json:"host,omitempty"`
	ZitiIdentityID string     `json:"ziti_identity_id"`
	State          string     `json:"state"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type controllerOwnedTunnel struct {
	Tunnel           controllerTunnel       `json:"tunnel"`
	ActiveServe      *controllerServe       `json:"active_serve,omitempty"`
	AttachmentCounts attachmentCounts       `json:"attachment_counts"`
	Attachments      []controllerAttachment `json:"attachments,omitempty"`
}

type controllerTunnel struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	AccountID      string    `json:"account_id"`
	EnvironmentID  string    `json:"environment_id"`
	Name           string    `json:"name"`
	Mode           string    `json:"mode"`
	Kind           string    `json:"kind"`
	BackendTarget  *string   `json:"backend_target,omitempty"`
	State          string    `json:"state"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type controllerServe struct {
	ID              string     `json:"id"`
	TunnelID        string     `json:"tunnel_id"`
	OrganizationID  string     `json:"organization_id"`
	AccountID       string     `json:"account_id"`
	EnvironmentID   string     `json:"environment_id"`
	EnvironmentHost *string    `json:"environment_host,omitempty"`
	TunnelName      *string    `json:"tunnel_name,omitempty"`
	TunnelMode      *string    `json:"tunnel_mode,omitempty"`
	State           string     `json:"state"`
	LastHeartbeatAt time.Time  `json:"last_heartbeat_at"`
	DisconnectedAt  *time.Time `json:"disconnected_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type controllerAttachment struct {
	ID              string     `json:"id"`
	TunnelID        string     `json:"tunnel_id"`
	OrganizationID  string     `json:"organization_id"`
	AccountID       string     `json:"account_id"`
	EnvironmentID   string     `json:"environment_id"`
	AccountEmail    *string    `json:"account_email,omitempty"`
	TunnelName      *string    `json:"tunnel_name,omitempty"`
	TunnelMode      *string    `json:"tunnel_mode,omitempty"`
	Kind            string     `json:"kind"`
	ListenAddress   string     `json:"listen_address"`
	State           string     `json:"state"`
	LastHeartbeatAt time.Time  `json:"last_heartbeat_at"`
	DisconnectedAt  *time.Time `json:"disconnected_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type attachmentCounts struct {
	Active       int `json:"active"`
	Stale        int `json:"stale"`
	Disconnected int `json:"disconnected"`
}

func init() {
	rootCmd.AddCommand(newStatusCommand().cmd)
}

func newStatusCommand() *statusCommand {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show composite Layer 1 network status",
		Args:  cobra.NoArgs,
	}
	command := &statusCommand{cmd: cmd}
	cmd.Flags().BoolVar(&command.jsonOutput, "json", false, "Output raw JSON instead of a human-readable summary")
	cmd.RunE = command.runE
	return command
}

func (cmd *statusCommand) runE(_ *cobra.Command, _ []string) error {
	root, err := environment.LoadRoot()
	if err != nil {
		return err
	}

	status := defaultStatusBuilder().build(root)
	if cmd.jsonOutput {
		return clioutput.RenderJSON(status)
	}

	renderCompositeStatus(status)
	return nil
}

func defaultStatusBuilder() statusBuilder {
	return statusBuilder{
		now:           func() time.Time { return time.Now().UTC() },
		networkStatus: networkStatusOrNil,
		newClient: func(apiEndpoint, accountToken string) (*api.Client, error) {
			return newAPIClient(apiEndpoint, staticSecuritySource{accountToken: accountToken})
		},
	}
}

func (b statusBuilder) build(root env_core.Root) *compositeStatus {
	status := &compositeStatus{}
	status.Local = b.buildLocalStatus(root)
	status.Controller = b.buildControllerStatus(root)
	return status
}

func (b statusBuilder) buildLocalStatus(root env_core.Root) localStatus {
	apiEndpoint, _ := root.APIEndpoint()
	local := localStatus{
		APIEndpoint: apiEndpoint,
	}

	if env := root.Environment(); env != nil {
		local.RootEnvironment = &localEnvironment{
			EnvironmentID: env.EnvironmentID,
			ZitiIdentity:  env.ZitiIdentity,
		}
	}
	if networkState := root.Network(); networkState != nil {
		local.RootNetwork = mapLocalDesiredState(networkState)
	}

	agentStatus, err := b.networkStatus(root)
	if err != nil {
		local.AgentError = err.Error()
		return local
	}
	if agentStatus != nil {
		local.AgentRunning = true
		local.AgentStatus = mapLocalAgentStatus(agentStatus)
	}

	return local
}

func (b statusBuilder) buildControllerStatus(root env_core.Root) controllerStatus {
	status := controllerStatus{
		Reachable:           false,
		EnvironmentLiveness: controllerEnvironmentLivenessDisabled,
	}

	if !root.IsEnabled() {
		return status
	}
	env := root.Environment()
	if env == nil {
		status.Error = "enabled environment is missing local state"
		status.EnvironmentLiveness = controllerEnvironmentLivenessUnknown
		return status
	}
	if env.EnvironmentID == "" {
		status.Error = "enabled environment is missing environment id"
		status.EnvironmentLiveness = controllerEnvironmentLivenessUnknown
		return status
	}

	client, err := b.newClient(env.APIEndpoint, env.AccountToken)
	if err != nil {
		status.Error = err.Error()
		status.EnvironmentLiveness = controllerEnvironmentLivenessUnknown
		return status
	}

	controllerEnv := b.fetchControllerEnvironment(client, env.EnvironmentID, &status)
	tunnels := b.fetchOwnedTunnels(client, &status)
	status.OwnedTunnels = tunnels
	if controllerEnv != nil {
		status.Environment = controllerEnv
		status.EnvironmentLiveness = deriveControllerEnvironmentLiveness(b.now(), controllerEnv)
	} else {
		status.EnvironmentLiveness = controllerEnvironmentLivenessUnknown
	}

	return status
}

func (b statusBuilder) fetchControllerEnvironment(client *api.Client, environmentID string, status *controllerStatus) *controllerEnvironment {
	ctx, cancel := context.WithTimeout(context.Background(), statusControllerRequestTimeout)
	defer cancel()

	res, err := client.GetEnvironment(ctx, api.GetEnvironmentParams{EnvironmentId: environmentID})
	if err != nil {
		status.Error = appendStatusError(status.Error, err.Error())
		return nil
	}

	switch typed := res.(type) {
	case *api.Environment:
		status.Reachable = true
		return mapControllerEnvironment(typed)
	case *api.GetEnvironmentUnauthorized:
		status.Reachable = true
		status.Error = appendStatusError(status.Error, typed.Message)
		return nil
	case *api.GetEnvironmentNotFound:
		status.Reachable = true
		status.Error = appendStatusError(status.Error, typed.Message)
		status.EnvironmentLiveness = controllerEnvironmentLivenessUnknown
		return nil
	case *api.GetEnvironmentInternalServerError:
		status.Reachable = true
		status.Error = appendStatusError(status.Error, typed.Message)
		status.EnvironmentLiveness = controllerEnvironmentLivenessUnknown
		return nil
	default:
		status.Error = appendStatusError(status.Error, fmt.Sprintf("unexpected get environment response: %T", res))
		return nil
	}
}

func (b statusBuilder) fetchOwnedTunnels(client *api.Client, status *controllerStatus) []controllerOwnedTunnel {
	ctx, cancel := context.WithTimeout(context.Background(), statusControllerRequestTimeout)
	defer cancel()

	params := api.ListTunnelsParams{}
	params.Scope.SetTo(api.ListTunnelsScopeOwned)
	res, err := client.ListTunnels(ctx, params)
	if err != nil {
		status.Error = appendStatusError(status.Error, err.Error())
		return nil
	}

	switch typed := res.(type) {
	case *api.ListTunnelsResponse:
		status.Reachable = true
		tunnels := make([]controllerOwnedTunnel, 0, len(*typed))
		for i := range *typed {
			tunnels = append(tunnels, b.buildOwnedTunnelStatus(client, &(*typed)[i], status))
		}
		return tunnels
	case *api.ListTunnelsUnauthorized:
		status.Reachable = true
		status.Error = appendStatusError(status.Error, typed.Message)
		return nil
	case *api.ListTunnelsInternalServerError:
		status.Reachable = true
		status.Error = appendStatusError(status.Error, typed.Message)
		return nil
	default:
		status.Error = appendStatusError(status.Error, fmt.Sprintf("unexpected list tunnels response: %T", res))
		return nil
	}
}

func (b statusBuilder) buildOwnedTunnelStatus(client *api.Client, tunnel *api.Tunnel, status *controllerStatus) controllerOwnedTunnel {
	result := controllerOwnedTunnel{
		Tunnel: mapControllerTunnel(tunnel),
	}

	if serve, err := b.fetchActiveServe(client, tunnel.ID); err != nil {
		status.Error = appendStatusError(status.Error, fmt.Sprintf("tunnel '%s' serve lookup failed: %v", tunnel.Name, err))
	} else {
		result.ActiveServe = serve
	}

	if attachments, err := b.fetchTunnelAttachments(client, tunnel.ID); err != nil {
		status.Error = appendStatusError(status.Error, fmt.Sprintf("tunnel '%s' attachment lookup failed: %v", tunnel.Name, err))
	} else {
		result.Attachments = attachments
		result.AttachmentCounts = countAttachments(attachments)
	}

	return result
}

func (b statusBuilder) fetchActiveServe(client *api.Client, tunnelID string) (*controllerServe, error) {
	ctx, cancel := context.WithTimeout(context.Background(), statusControllerRequestTimeout)
	defer cancel()

	res, err := client.GetTunnelServe(ctx, api.GetTunnelServeParams{TunnelId: tunnelID})
	if err != nil {
		return nil, err
	}

	switch typed := res.(type) {
	case *api.TunnelServe:
		mapped := mapControllerServe(typed)
		return &mapped, nil
	case *api.GetTunnelServeNotFound:
		return nil, nil
	case *api.GetTunnelServeUnauthorized:
		return nil, fmt.Errorf("%s", typed.Message)
	case *api.GetTunnelServeInternalServerError:
		return nil, fmt.Errorf("%s", typed.Message)
	default:
		return nil, fmt.Errorf("unexpected get tunnel serve response: %T", res)
	}
}

func (b statusBuilder) fetchTunnelAttachments(client *api.Client, tunnelID string) ([]controllerAttachment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), statusControllerRequestTimeout)
	defer cancel()

	res, err := client.ListTunnelAttachments(ctx, api.ListTunnelAttachmentsParams{TunnelId: tunnelID})
	if err != nil {
		return nil, err
	}

	switch typed := res.(type) {
	case *api.ListTunnelAttachmentsResponse:
		attachments := make([]controllerAttachment, 0, len(*typed))
		for i := range *typed {
			attachments = append(attachments, mapControllerAttachment(&(*typed)[i]))
		}
		return attachments, nil
	case *api.ListTunnelAttachmentsNotFound:
		return nil, fmt.Errorf("%s", typed.Message)
	case *api.ListTunnelAttachmentsUnauthorized:
		return nil, fmt.Errorf("%s", typed.Message)
	case *api.ListTunnelAttachmentsInternalServerError:
		return nil, fmt.Errorf("%s", typed.Message)
	default:
		return nil, fmt.Errorf("unexpected list tunnel attachments response: %T", res)
	}
}

func mapLocalDesiredState(networkState *env_core.Network) *localDesiredState {
	if networkState == nil {
		return nil
	}
	result := &localDesiredState{
		Serves:   make([]desiredServe, 0, len(networkState.Serves)),
		Connects: make([]desiredConnect, 0, len(networkState.Connects)),
	}
	for _, serve := range networkState.Serves {
		result.Serves = append(result.Serves, desiredServe{
			TunnelID:      serve.TunnelID,
			Name:          serve.Name,
			Mode:          string(serve.Mode),
			BackendTarget: serve.BackendTarget,
			GrantEmails:   append([]string(nil), serve.GrantEmails...),
		})
	}
	for _, connect := range networkState.Connects {
		result.Connects = append(result.Connects, desiredConnect{
			TunnelID:      connect.TunnelID,
			Name:          connect.Name,
			ListenAddress: connect.ListenAddress,
		})
	}
	return result
}

func mapLocalAgentStatus(status *networkpb.NetworkStatus) *localAgentStatus {
	result := &localAgentStatus{
		SocketPath:         status.SocketPath,
		PID:                status.Pid,
		StartedAt:          status.StartedAt.AsTime().UTC(),
		EnvironmentEnabled: status.EnvironmentEnabled,
		EnvironmentID:      status.EnvironmentId,
		Serves:             make([]managedServeStatus, 0, len(status.Serves)),
		Connects:           make([]managedConnectStatus, 0, len(status.Connects)),
	}
	if status.EnvironmentHeartbeat != nil {
		result.EnvironmentHeartbeat = &environmentHeartbeatState{
			State:         formatEnvironmentHeartbeatState(status.EnvironmentHeartbeat.State),
			LastAttemptAt: timestampPtr(status.EnvironmentHeartbeat.LastAttemptAt),
			LastSuccessAt: timestampPtr(status.EnvironmentHeartbeat.LastSuccessAt),
			LastError:     status.EnvironmentHeartbeat.LastError,
		}
	}
	for _, serve := range status.Serves {
		result.Serves = append(result.Serves, managedServeStatus{
			Desired: desiredServe{
				TunnelID:      serve.Desired.TunnelId,
				Name:          serve.Desired.Name,
				Mode:          serve.Desired.Mode,
				BackendTarget: serve.Desired.BackendTarget,
				GrantEmails:   append([]string(nil), serve.Desired.GrantEmails...),
			},
			State:         formatRuntimeState(serve.State),
			TunnelID:      serve.TunnelId,
			ServeID:       serve.ServeId,
			LastError:     serve.LastError,
			LastStartedAt: timestampPtr(serve.LastStartedAt),
			NextRetryAt:   timestampPtr(serve.NextRetryAt),
			RetryAttempt:  serve.RetryAttempt,
		})
	}
	for _, connect := range status.Connects {
		result.Connects = append(result.Connects, managedConnectStatus{
			Desired: desiredConnect{
				TunnelID:      connect.Desired.TunnelId,
				Name:          connect.Desired.Name,
				ListenAddress: connect.Desired.ListenAddress,
			},
			State:         formatRuntimeState(connect.State),
			TunnelID:      connect.TunnelId,
			AttachmentID:  connect.AttachmentId,
			LastError:     connect.LastError,
			LastStartedAt: timestampPtr(connect.LastStartedAt),
			NextRetryAt:   timestampPtr(connect.NextRetryAt),
			RetryAttempt:  connect.RetryAttempt,
		})
	}
	return result
}

func mapConfiguredServesFromDesired(desired *localDesiredState) []managedServeStatus {
	if desired == nil {
		return nil
	}
	result := make([]managedServeStatus, 0, len(desired.Serves))
	for _, serve := range desired.Serves {
		result = append(result, managedServeStatus{
			Desired:  serve,
			State:    "configured",
			TunnelID: serve.TunnelID,
		})
	}
	return result
}

func mapConfiguredConnectsFromDesired(desired *localDesiredState) []managedConnectStatus {
	if desired == nil {
		return nil
	}
	result := make([]managedConnectStatus, 0, len(desired.Connects))
	for _, connect := range desired.Connects {
		result = append(result, managedConnectStatus{
			Desired:  connect,
			State:    "configured",
			TunnelID: connect.TunnelID,
		})
	}
	return result
}

func mapControllerEnvironment(env *api.Environment) *controllerEnvironment {
	return &controllerEnvironment{
		ID:             env.ID,
		OrganizationID: env.OrganizationId,
		AccountID:      env.AccountId,
		Description:    optStringPtr(env.Description),
		Host:           optStringPtr(env.Host),
		ZitiIdentityID: env.ZitiIdentityId,
		State:          string(env.State),
		LastSeenAt:     optDateTimePtr(env.LastSeenAt),
		CreatedAt:      env.CreatedAt.UTC(),
		UpdatedAt:      env.UpdatedAt.UTC(),
	}
}

func mapControllerTunnel(tunnel *api.Tunnel) controllerTunnel {
	return controllerTunnel{
		ID:             tunnel.ID,
		OrganizationID: tunnel.OrganizationId,
		AccountID:      tunnel.AccountId,
		EnvironmentID:  tunnel.EnvironmentId.Or(""),
		Name:           tunnel.Name,
		Mode:           string(tunnel.Mode),
		Kind:           string(tunnel.Kind),
		BackendTarget:  optStringPtr(tunnel.BackendTarget),
		State:          string(tunnel.State),
		CreatedAt:      tunnel.CreatedAt.UTC(),
		UpdatedAt:      tunnel.UpdatedAt.UTC(),
	}
}

func mapControllerServe(serve *api.TunnelServe) controllerServe {
	return controllerServe{
		ID:              serve.ID,
		TunnelID:        serve.TunnelId,
		OrganizationID:  serve.OrganizationId,
		AccountID:       serve.AccountId,
		EnvironmentID:   serve.EnvironmentId,
		EnvironmentHost: optStringPtr(serve.EnvironmentHost),
		TunnelName:      optStringPtr(serve.TunnelName),
		TunnelMode:      optTunnelModePtr(serve.TunnelMode),
		State:           string(serve.State),
		LastHeartbeatAt: serve.LastHeartbeatAt.UTC(),
		DisconnectedAt:  optDateTimePtr(serve.DisconnectedAt),
		CreatedAt:       serve.CreatedAt.UTC(),
		UpdatedAt:       serve.UpdatedAt.UTC(),
	}
}

func mapControllerAttachment(attachment *api.TunnelAttachment) controllerAttachment {
	return controllerAttachment{
		ID:              attachment.ID,
		TunnelID:        attachment.TunnelId,
		OrganizationID:  attachment.OrganizationId,
		AccountID:       attachment.AccountId,
		EnvironmentID:   attachment.EnvironmentId,
		AccountEmail:    optStringPtr(attachment.AccountEmail),
		TunnelName:      optStringPtr(attachment.TunnelName),
		TunnelMode:      optTunnelModePtr(attachment.TunnelMode),
		Kind:            string(attachment.Kind),
		ListenAddress:   attachment.ListenAddress.Or(""),
		State:           string(attachment.State),
		LastHeartbeatAt: attachment.LastHeartbeatAt.UTC(),
		DisconnectedAt:  optDateTimePtr(attachment.DisconnectedAt),
		CreatedAt:       attachment.CreatedAt.UTC(),
		UpdatedAt:       attachment.UpdatedAt.UTC(),
	}
}

func deriveControllerEnvironmentLiveness(now time.Time, env *controllerEnvironment) string {
	if env == nil {
		return controllerEnvironmentLivenessUnknown
	}
	if env.State == string(api.EnvironmentStateDisabled) {
		return controllerEnvironmentLivenessDisabled
	}
	if env.LastSeenAt == nil {
		return controllerEnvironmentLivenessUnknown
	}
	if now.UTC().Sub(env.LastSeenAt.UTC()) <= statusEnvironmentLivenessTTL {
		return controllerEnvironmentLivenessOnline
	}
	return controllerEnvironmentLivenessStale
}

func countAttachments(attachments []controllerAttachment) attachmentCounts {
	var counts attachmentCounts
	for _, attachment := range attachments {
		switch attachment.State {
		case string(api.TunnelAttachmentStateActive):
			counts.Active++
		case string(api.TunnelAttachmentStateStale):
			counts.Stale++
		case string(api.TunnelAttachmentStateDisconnected):
			counts.Disconnected++
		}
	}
	return counts
}

func appendStatusError(existing, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	return existing + "; " + next
}

func optStringPtr(v api.OptString) *string {
	if !v.Set {
		return nil
	}
	value := v.Value
	return &value
}

func optDateTimePtr(v api.OptDateTime) *time.Time {
	if !v.Set {
		return nil
	}
	value := v.Value.UTC()
	return &value
}

func optTunnelModePtr(v api.OptTunnelMode) *string {
	if !v.Set {
		return nil
	}
	value := string(v.Value)
	return &value
}

func formatControllerTunnelBackend(tunnel controllerTunnel) string {
	if tunnel.Kind == string(api.TunnelKindDirect) {
		return "direct listener"
	}
	if tunnel.BackendTarget == nil || strings.TrimSpace(*tunnel.BackendTarget) == "" {
		return "-"
	}
	return *tunnel.BackendTarget
}

func timestampPtr(v interface{ AsTime() time.Time }) *time.Time {
	if v == nil {
		return nil
	}
	value := v.AsTime().UTC()
	return &value
}

func renderCompositeStatus(status *compositeStatus) {
	fmt.Println()
	fmt.Println("local:")
	if status.Local.RootEnvironment != nil {
		fmt.Printf("environment: enabled '%s'\n", status.Local.RootEnvironment.EnvironmentID)
	} else {
		fmt.Println("environment: disabled")
	}
	if status.Local.APIEndpoint != "" {
		fmt.Printf("api endpoint: %s\n", status.Local.APIEndpoint)
	}
	switch {
	case status.Local.AgentError != "":
		fmt.Printf("network: error '%s'\n", status.Local.AgentError)
	case status.Local.AgentRunning && status.Local.AgentStatus != nil:
		fmt.Println("network: running")
		fmt.Printf("socket: %s\n", status.Local.AgentStatus.SocketPath)
		fmt.Printf("pid: %d\n", status.Local.AgentStatus.PID)
		fmt.Printf("started: %s\n", clioutput.TimeUTC(status.Local.AgentStatus.StartedAt))
		if heartbeat := status.Local.AgentStatus.EnvironmentHeartbeat; heartbeat != nil {
			fmt.Printf("heartbeat: %s\n", heartbeat.State)
			if heartbeat.LastAttemptAt != nil {
				fmt.Printf("heartbeat last attempt: %s\n", clioutput.TimeUTC(heartbeat.LastAttemptAt.UTC()))
			}
			if heartbeat.LastSuccessAt != nil {
				fmt.Printf("heartbeat last success: %s\n", clioutput.TimeUTC(heartbeat.LastSuccessAt.UTC()))
			}
			if heartbeat.LastError != "" {
				fmt.Printf("heartbeat last error: %s\n", heartbeat.LastError)
			}
		}
	default:
		fmt.Println("network: not running")
	}

	serves := []managedServeStatus(nil)
	connects := []managedConnectStatus(nil)
	if status.Local.AgentStatus != nil {
		serves = status.Local.AgentStatus.Serves
		connects = status.Local.AgentStatus.Connects
	} else if status.Local.RootNetwork != nil {
		serves = mapConfiguredServesFromDesired(status.Local.RootNetwork)
		connects = mapConfiguredConnectsFromDesired(status.Local.RootNetwork)
	}
	renderManagedServeSummary(serves)
	renderManagedConnectSummary(connects)
	if len(serves) == 0 && len(connects) == 0 {
		fmt.Println()
		fmt.Println("no desired serves or connects configured")
	}

	fmt.Println()
	fmt.Println("controller:")
	switch {
	case status.Local.RootEnvironment == nil:
		fmt.Println("skipped (no environment enabled)")
	case !status.Controller.Reachable && status.Controller.Error != "":
		fmt.Printf("unreachable: %s\n", status.Controller.Error)
	default:
		fmt.Printf("reachable: %t\n", status.Controller.Reachable)
		if status.Controller.Environment != nil {
			fmt.Printf("environment: %s '%s'\n", status.Controller.Environment.State, status.Controller.Environment.ID)
			if status.Controller.Environment.Host != nil {
				fmt.Printf("host: %s\n", *status.Controller.Environment.Host)
			}
			if status.Controller.Environment.Description != nil {
				fmt.Printf("description: %s\n", *status.Controller.Environment.Description)
			}
			if status.Controller.Environment.LastSeenAt != nil {
				fmt.Printf("last seen: %s\n", clioutput.TimeUTC(status.Controller.Environment.LastSeenAt.UTC()))
			}
		}
		if status.Controller.EnvironmentLiveness != "" {
			fmt.Printf("liveness: %s\n", status.Controller.EnvironmentLiveness)
		}
		if status.Controller.Error != "" {
			fmt.Printf("error: %s\n", status.Controller.Error)
		}
	}

	if len(status.Controller.OwnedTunnels) > 0 {
		t := clioutput.NewTable()
		t.AppendHeader(table.Row{"Tunnel", "Mode", "Backend", "Active Serve", "Serve Host", "Attachments"})
		for _, tunnel := range status.Controller.OwnedTunnels {
			activeServe := "-"
			serveHost := "-"
			if tunnel.ActiveServe != nil {
				activeServe = clioutput.StringOrDash(tunnel.ActiveServe.ID)
				if tunnel.ActiveServe.EnvironmentHost != nil {
					serveHost = *tunnel.ActiveServe.EnvironmentHost
				}
			}
			t.AppendRow(table.Row{
				tunnel.Tunnel.Name,
				tunnel.Tunnel.Mode,
				formatControllerTunnelBackend(tunnel.Tunnel),
				activeServe,
				serveHost,
				formatAttachmentCounts(tunnel.AttachmentCounts),
			})
		}
		clioutput.PrintTable(t)
		clioutput.PrintTotal("owned tunnel(s)", len(status.Controller.OwnedTunnels))
	} else if status.Controller.Reachable && status.Local.RootEnvironment != nil {
		fmt.Println()
		fmt.Println("no owned tunnels")
		fmt.Println()
	}
}

func renderManagedServeSummary(serves []managedServeStatus) {
	if len(serves) == 0 {
		return
	}
	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Serve", "Mode", "Backend", "State", "Tunnel ID", "Serve ID", "Retry", "Next Retry", "Last Started", "Last Error"})
	for _, serve := range serves {
		lastStarted := "-"
		if serve.LastStartedAt != nil {
			lastStarted = clioutput.TimeUTC(serve.LastStartedAt.UTC())
		}
		nextRetry := "-"
		if serve.NextRetryAt != nil {
			nextRetry = clioutput.TimeUTC(serve.NextRetryAt.UTC())
		}
		t.AppendRow(table.Row{
			serve.Desired.Name,
			serve.Desired.Mode,
			serve.Desired.BackendTarget,
			serve.State,
			clioutput.StringOrDash(serve.TunnelID),
			clioutput.StringOrDash(serve.ServeID),
			serve.RetryAttempt,
			nextRetry,
			lastStarted,
			clioutput.StringOrDash(serve.LastError),
		})
	}
	clioutput.PrintTable(t)
}

func renderManagedConnectSummary(connects []managedConnectStatus) {
	if len(connects) == 0 {
		return
	}
	t := clioutput.NewTable()
	t.AppendHeader(table.Row{"Connect", "Listen", "State", "Tunnel ID", "Attachment ID", "Retry", "Next Retry", "Last Started", "Last Error"})
	for _, connect := range connects {
		lastStarted := "-"
		if connect.LastStartedAt != nil {
			lastStarted = clioutput.TimeUTC(connect.LastStartedAt.UTC())
		}
		nextRetry := "-"
		if connect.NextRetryAt != nil {
			nextRetry = clioutput.TimeUTC(connect.NextRetryAt.UTC())
		}
		t.AppendRow(table.Row{
			connect.Desired.Name,
			connect.Desired.ListenAddress,
			connect.State,
			clioutput.StringOrDash(connect.TunnelID),
			clioutput.StringOrDash(connect.AttachmentID),
			connect.RetryAttempt,
			nextRetry,
			lastStarted,
			clioutput.StringOrDash(connect.LastError),
		})
	}
	clioutput.PrintTable(t)
}

func formatAttachmentCounts(counts attachmentCounts) string {
	total := counts.Active + counts.Stale + counts.Disconnected
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%d active / %d stale / %d disconnected", counts.Active, counts.Stale, counts.Disconnected)
}
