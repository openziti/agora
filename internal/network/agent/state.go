package agent

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func loadRootState() (env_core.Root, *env_core.Environment, *env_core.Network, error) {
	root, err := environment.LoadRoot()
	if err != nil {
		return nil, nil, nil, err
	}
	return root, cloneEnvironment(root.Environment()), cloneNetwork(root.Network()), nil
}

func cloneEnvironment(env *env_core.Environment) *env_core.Environment {
	if env == nil {
		return nil
	}
	cloned := *env
	return &cloned
}

func cloneNetwork(networkState *env_core.Network) *env_core.Network {
	if networkState == nil {
		return &env_core.Network{}
	}
	cloned := &env_core.Network{
		Serves:   make([]env_core.ManagedServe, 0, len(networkState.Serves)),
		Connects: make([]env_core.ManagedConnect, 0, len(networkState.Connects)),
	}
	for _, serve := range networkState.Serves {
		cloned.Serves = append(cloned.Serves, env_core.ManagedServe{
			TunnelID:      serve.TunnelID,
			Name:          serve.Name,
			Mode:          serve.Mode,
			BackendTarget: serve.BackendTarget,
			GrantEmails:   append([]string(nil), serve.GrantEmails...),
		})
	}
	for _, connect := range networkState.Connects {
		cloned.Connects = append(cloned.Connects, env_core.ManagedConnect{
			TunnelID:      connect.TunnelID,
			Name:          connect.Name,
			ListenAddress: connect.ListenAddress,
		})
	}
	return cloned
}

func validateDesiredServe(req *networkpb.EnsureServeRequest) (env_core.ManagedServe, error) {
	mode := api.TunnelMode(strings.ToLower(strings.TrimSpace(req.Mode)))
	name := strings.TrimSpace(req.Name)
	backendTarget := strings.TrimSpace(req.BackendTarget)
	if name == "" {
		return env_core.ManagedServe{}, fmt.Errorf("serve name is required")
	}
	switch mode {
	case api.TunnelModeHTTP:
		u, err := url.Parse(backendTarget)
		if err != nil {
			return env_core.ManagedServe{}, err
		}
		if u.Scheme == "" || u.Host == "" {
			return env_core.ManagedServe{}, fmt.Errorf("http backend target must include scheme and host")
		}
	case api.TunnelModeTCP, api.TunnelModeUDP:
		if _, _, err := net.SplitHostPort(backendTarget); err != nil {
			return env_core.ManagedServe{}, err
		}
	default:
		return env_core.ManagedServe{}, fmt.Errorf("unsupported tunnel mode '%s'", req.Mode)
	}
	grantEmails := make([]string, 0, len(req.GrantEmails))
	for _, email := range req.GrantEmails {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		grantEmails = append(grantEmails, email)
	}
	return env_core.ManagedServe{
		TunnelID:      strings.TrimSpace(req.TunnelId),
		Name:          name,
		Mode:          mode,
		BackendTarget: backendTarget,
		GrantEmails:   grantEmails,
	}, nil
}

func validateDesiredConnect(req *networkpb.EnsureConnectRequest) (env_core.ManagedConnect, error) {
	name := strings.TrimSpace(req.Name)
	listenAddress := strings.TrimSpace(req.ListenAddress)
	if name == "" {
		return env_core.ManagedConnect{}, fmt.Errorf("connect name is required")
	}
	if _, _, err := net.SplitHostPort(listenAddress); err != nil {
		return env_core.ManagedConnect{}, err
	}
	return env_core.ManagedConnect{
		TunnelID:      strings.TrimSpace(req.TunnelId),
		Name:          name,
		ListenAddress: listenAddress,
	}, nil
}

func upsertServe(networkState *env_core.Network, desired env_core.ManagedServe) {
	replaced := false
	serves := make([]env_core.ManagedServe, 0, len(networkState.Serves))
	for _, existing := range networkState.Serves {
		if sameServe(existing, desired) {
			serves = append(serves, desired)
			replaced = true
			continue
		}
		serves = append(serves, existing)
	}
	if !replaced {
		serves = append(serves, desired)
	}
	networkState.Serves = serves
}

func removeServe(networkState *env_core.Network, tunnelID, name string) {
	filtered := make([]env_core.ManagedServe, 0, len(networkState.Serves))
	for _, existing := range networkState.Serves {
		if matchesServe(existing, tunnelID, name) {
			continue
		}
		filtered = append(filtered, existing)
	}
	networkState.Serves = filtered
}

func upsertConnect(networkState *env_core.Network, desired env_core.ManagedConnect) {
	replaced := false
	connects := make([]env_core.ManagedConnect, 0, len(networkState.Connects))
	for _, existing := range networkState.Connects {
		if sameConnect(existing, desired) {
			connects = append(connects, desired)
			replaced = true
			continue
		}
		connects = append(connects, existing)
	}
	if !replaced {
		connects = append(connects, desired)
	}
	networkState.Connects = connects
}

func removeConnect(networkState *env_core.Network, tunnelID, name, listenAddress string) {
	filtered := make([]env_core.ManagedConnect, 0, len(networkState.Connects))
	for _, existing := range networkState.Connects {
		if matchesConnect(existing, tunnelID, name, listenAddress) {
			continue
		}
		filtered = append(filtered, existing)
	}
	networkState.Connects = filtered
}

func sameServe(left, right env_core.ManagedServe) bool {
	if left.TunnelID != "" && right.TunnelID != "" {
		return left.TunnelID == right.TunnelID
	}
	return strings.EqualFold(left.Name, right.Name)
}

func matchesServe(existing env_core.ManagedServe, tunnelID, name string) bool {
	if tunnelID != "" && existing.TunnelID == tunnelID {
		return true
	}
	return name != "" && strings.EqualFold(existing.Name, name)
}

func sameConnect(left, right env_core.ManagedConnect) bool {
	if left.TunnelID != "" && right.TunnelID != "" {
		return left.TunnelID == right.TunnelID && left.ListenAddress == right.ListenAddress
	}
	return strings.EqualFold(left.Name, right.Name) && left.ListenAddress == right.ListenAddress
}

func matchesConnect(existing env_core.ManagedConnect, tunnelID, name, listenAddress string) bool {
	if tunnelID != "" && existing.TunnelID == tunnelID && existing.ListenAddress == listenAddress {
		return true
	}
	return name != "" && strings.EqualFold(existing.Name, name) && existing.ListenAddress == listenAddress
}

func networkStatusProto(pid int, startedAt time.Time, socketPath string, env *env_core.Environment, networkState *env_core.Network, heartbeat *networkpb.EnvironmentHeartbeatStatus) *networkpb.NetworkStatus {
	status := &networkpb.NetworkStatus{
		Pid:                  int32(pid),
		StartedAt:            timestamppb.New(startedAt.UTC()),
		SocketPath:           socketPath,
		Serves:               make([]*networkpb.ManagedServeStatus, 0, len(networkState.Serves)),
		Connects:             make([]*networkpb.ManagedConnectStatus, 0, len(networkState.Connects)),
		EnvironmentHeartbeat: heartbeat,
	}
	if env != nil {
		status.EnvironmentEnabled = true
		status.EnvironmentId = env.EnvironmentID
	}
	for _, serve := range networkState.Serves {
		status.Serves = append(status.Serves, serveStatusProto(serve))
	}
	for _, connect := range networkState.Connects {
		status.Connects = append(status.Connects, connectStatusProto(connect))
	}
	return status
}

func serveStatusProto(desired env_core.ManagedServe) *networkpb.ManagedServeStatus {
	return &networkpb.ManagedServeStatus{
		Desired: &networkpb.DesiredServe{
			TunnelId:      desired.TunnelID,
			Name:          desired.Name,
			Mode:          string(desired.Mode),
			BackendTarget: desired.BackendTarget,
			GrantEmails:   append([]string(nil), desired.GrantEmails...),
		},
		State:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
		TunnelId: desired.TunnelID,
	}
}

func connectStatusProto(desired env_core.ManagedConnect) *networkpb.ManagedConnectStatus {
	return &networkpb.ManagedConnectStatus{
		Desired: &networkpb.DesiredConnect{
			TunnelId:      desired.TunnelID,
			Name:          desired.Name,
			ListenAddress: desired.ListenAddress,
		},
		State:    networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
		TunnelId: desired.TunnelID,
	}
}

func persistNetwork(root env_core.Root, networkState *env_core.Network) error {
	if networkState == nil || (len(networkState.Serves) == 0 && len(networkState.Connects) == 0) {
		return root.DeleteNetwork()
	}
	return root.SetNetwork(cloneNetwork(networkState))
}

func socketExists(socketPath string) bool {
	info, err := os.Stat(socketPath)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}
