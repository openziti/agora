package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	testAccountID      = "ac_aaaaaaaaaaaa"
	testAttachmentID1  = "ta_aaaaaaaaaaab"
	testAttachmentID2  = "ta_aaaaaaaaaaac"
	testEnvironmentID  = "ev_aaaaaaaaaaaa"
	testOrganizationID = "org_aaaaaaaaaaaa"
	testServeID        = "ts_aaaaaaaaaaaa"
	testTunnelID       = "tt_aaaaaaaaaaaa"
)

func TestStatusBuilderWithoutEnabledEnvironmentSkipsController(t *testing.T) {
	t.Parallel()

	root := &statusTestRoot{}
	builder := statusBuilder{
		now: func() time.Time { return time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC) },
		networkStatus: func(env_core.Root) (*networkpb.NetworkStatus, error) {
			return nil, nil
		},
		newClient: func(string, string) (*api.Client, error) {
			t.Fatal("unexpected controller client construction")
			return nil, nil
		},
	}

	status := builder.build(root)
	if status.Local.RootEnvironment != nil {
		t.Fatalf("expected no local environment, got %#v", status.Local.RootEnvironment)
	}
	if status.Controller.Reachable {
		t.Fatalf("expected controller to be skipped")
	}
	if status.Controller.EnvironmentLiveness != controllerEnvironmentLivenessDisabled {
		t.Fatalf("expected disabled liveness, got %s", status.Controller.EnvironmentLiveness)
	}
	if status.Controller.Error != "" {
		t.Fatalf("expected no controller error, got %q", status.Controller.Error)
	}
}

func TestStatusBuilderBuildsCompositeStatus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-5 * time.Minute)
	updatedAt := now.Add(-2 * time.Minute)
	lastSeenAt := now.Add(-20 * time.Second)
	lastHeartbeatAt := now.Add(-10 * time.Second)

	root := &statusTestRoot{
		apiEndpoint: "https://status.test",
		environment: &env_core.Environment{
			EnvironmentID: testEnvironmentID,
			AccountToken:  "account-token",
			ZitiIdentity:  "env",
			APIEndpoint:   "https://status.test",
		},
		network: &env_core.Network{
			Serves: []env_core.ManagedServe{{
				TunnelID:      testTunnelID,
				Name:          "gateway",
				Mode:          api.TunnelModeHTTP,
				BackendTarget: "https://backend.example.com",
				GrantEmails:   []string{"alice@example.com"},
			}},
		},
	}

	builder := statusBuilder{
		now: func() time.Time { return now },
		networkStatus: func(env_core.Root) (*networkpb.NetworkStatus, error) {
			return &networkpb.NetworkStatus{
				Pid:                4242,
				StartedAt:          timestamppb.New(createdAt),
				SocketPath:         "/tmp/agora-network.sock",
				EnvironmentEnabled: true,
				EnvironmentId:      testEnvironmentID,
				EnvironmentHeartbeat: &networkpb.EnvironmentHeartbeatStatus{
					State:         networkpb.EnvironmentHeartbeatState_ENVIRONMENT_HEARTBEAT_STATE_ONLINE,
					LastAttemptAt: timestamppb.New(lastHeartbeatAt),
					LastSuccessAt: timestamppb.New(lastHeartbeatAt),
				},
				Serves: []*networkpb.ManagedServeStatus{{
					Desired: &networkpb.DesiredServe{
						TunnelId:      testTunnelID,
						Name:          "gateway",
						Mode:          "http",
						BackendTarget: "https://backend.example.com",
					},
					State:         networkpb.RuntimeState_RUNTIME_STATE_RUNNING,
					TunnelId:      testTunnelID,
					ServeId:       testServeID,
					LastStartedAt: timestamppb.New(createdAt),
				}},
			}, nil
		},
		newClient: func(apiEndpoint, accountToken string) (*api.Client, error) {
			return api.NewClient(apiEndpoint+"/v1", staticSecuritySource{accountToken: accountToken}, api.WithClient(&http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch {
					case req.Method == http.MethodGet && req.URL.Path == "/v1/environments/"+testEnvironmentID:
						return jsonResponse(t, http.StatusOK, map[string]any{
							"id":             testEnvironmentID,
							"organizationId": testOrganizationID,
							"accountId":      testAccountID,
							"description":    "local laptop",
							"host":           "laptop-01",
							"zitiIdentityId": "ziti-env-1",
							"state":          "enabled",
							"lastSeenAt":     lastSeenAt.Format(time.RFC3339),
							"createdAt":      createdAt.Format(time.RFC3339),
							"updatedAt":      updatedAt.Format(time.RFC3339),
						}), nil
					case req.Method == http.MethodGet && req.URL.Path == "/v1/tunnels":
						if got := req.URL.Query().Get("scope"); got != "owned" {
							t.Fatalf("expected owned tunnel scope, got %q", got)
						}
						return jsonResponse(t, http.StatusOK, []map[string]any{{
							"id":             testTunnelID,
							"organizationId": testOrganizationID,
							"accountId":      testAccountID,
							"environmentId":  testEnvironmentID,
							"name":           "gateway",
							"mode":           "http",
							"kind":           "proxy",
							"backendTarget":  "https://backend.example.com",
							"state":          "active",
							"createdAt":      createdAt.Format(time.RFC3339),
							"updatedAt":      updatedAt.Format(time.RFC3339),
						}}), nil
					case req.Method == http.MethodGet && req.URL.Path == "/v1/tunnels/"+testTunnelID+"/serve":
						return jsonResponse(t, http.StatusOK, map[string]any{
							"id":              testServeID,
							"tunnelId":        testTunnelID,
							"organizationId":  testOrganizationID,
							"accountId":       testAccountID,
							"environmentId":   testEnvironmentID,
							"environmentHost": "laptop-01",
							"tunnelName":      "gateway",
							"tunnelMode":      "http",
							"state":           "active",
							"lastHeartbeatAt": lastHeartbeatAt.Format(time.RFC3339),
							"createdAt":       createdAt.Format(time.RFC3339),
							"updatedAt":       updatedAt.Format(time.RFC3339),
						}), nil
					case req.Method == http.MethodGet && req.URL.Path == "/v1/tunnels/"+testTunnelID+"/attachments":
						return jsonResponse(t, http.StatusOK, []map[string]any{
							{
								"id":              testAttachmentID1,
								"tunnelId":        testTunnelID,
								"organizationId":  testOrganizationID,
								"accountId":       testAccountID,
								"environmentId":   testEnvironmentID,
								"accountEmail":    "alice@example.com",
								"tunnelName":      "gateway",
								"tunnelMode":      "http",
								"listenAddress":   "127.0.0.1:8080",
								"state":           "active",
								"lastHeartbeatAt": lastHeartbeatAt.Format(time.RFC3339),
								"createdAt":       createdAt.Format(time.RFC3339),
								"updatedAt":       updatedAt.Format(time.RFC3339),
							},
							{
								"id":              testAttachmentID2,
								"tunnelId":        testTunnelID,
								"organizationId":  testOrganizationID,
								"accountId":       testAccountID,
								"environmentId":   testEnvironmentID,
								"accountEmail":    "alice@example.com",
								"tunnelName":      "gateway",
								"tunnelMode":      "http",
								"listenAddress":   "127.0.0.1:8081",
								"state":           "stale",
								"lastHeartbeatAt": lastHeartbeatAt.Add(-time.Minute).Format(time.RFC3339),
								"createdAt":       createdAt.Format(time.RFC3339),
								"updatedAt":       updatedAt.Format(time.RFC3339),
							},
						}), nil
					default:
						return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
					}
				}),
			}))
		},
	}

	status := builder.build(root)
	if !status.Local.AgentRunning {
		t.Fatalf("expected local agent to be running")
	}
	if status.Local.AgentStatus == nil {
		t.Fatalf("expected local agent status")
	}
	if status.Local.RootNetwork == nil || len(status.Local.RootNetwork.Serves) != 1 {
		t.Fatalf("expected persisted desired serve state, got %#v", status.Local.RootNetwork)
	}
	if !status.Controller.Reachable {
		t.Fatalf("expected controller to be reachable")
	}
	if status.Controller.Environment == nil {
		t.Fatalf("expected controller environment")
	}
	if status.Controller.EnvironmentLiveness != controllerEnvironmentLivenessOnline {
		t.Fatalf("expected online liveness, got %s", status.Controller.EnvironmentLiveness)
	}
	if len(status.Controller.OwnedTunnels) != 1 {
		t.Fatalf("expected one owned tunnel, got %d", len(status.Controller.OwnedTunnels))
	}
	tunnel := status.Controller.OwnedTunnels[0]
	if tunnel.ActiveServe == nil || tunnel.ActiveServe.ID != testServeID {
		t.Fatalf("expected active serve %q, got %#v", testServeID, tunnel.ActiveServe)
	}
	if tunnel.AttachmentCounts.Active != 1 || tunnel.AttachmentCounts.Stale != 1 || tunnel.AttachmentCounts.Disconnected != 0 {
		t.Fatalf("unexpected attachment counts: %#v", tunnel.AttachmentCounts)
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal composite status: %v", err)
	}
	if !bytes.Contains(data, []byte(`"root_environment"`)) {
		t.Fatalf("expected root_environment in JSON output: %s", string(data))
	}
	if !bytes.Contains(data, []byte(`"owned_tunnels"`)) {
		t.Fatalf("expected owned_tunnels in JSON output: %s", string(data))
	}
}

func TestStatusBuilderControllerUnavailableStillReturnsLocalState(t *testing.T) {
	t.Parallel()

	root := &statusTestRoot{
		apiEndpoint: "https://status.test",
		environment: &env_core.Environment{
			EnvironmentID: testEnvironmentID,
			AccountToken:  "account-token",
			ZitiIdentity:  "env",
			APIEndpoint:   "https://status.test",
		},
		network: &env_core.Network{
			Connects: []env_core.ManagedConnect{{
				Name:          "gateway",
				ListenAddress: "127.0.0.1:9191",
			}},
		},
	}

	builder := statusBuilder{
		now: func() time.Time { return time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC) },
		networkStatus: func(env_core.Root) (*networkpb.NetworkStatus, error) {
			return nil, nil
		},
		newClient: func(apiEndpoint, accountToken string) (*api.Client, error) {
			return api.NewClient(apiEndpoint+"/v1", staticSecuritySource{accountToken: accountToken}, api.WithClient(&http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("dial tcp: connection refused")
				}),
			}))
		},
	}

	status := builder.build(root)
	if status.Local.RootEnvironment == nil {
		t.Fatalf("expected local environment to be present")
	}
	if status.Local.RootNetwork == nil || len(status.Local.RootNetwork.Connects) != 1 {
		t.Fatalf("expected persisted desired connect state, got %#v", status.Local.RootNetwork)
	}
	if status.Controller.Reachable {
		t.Fatalf("expected controller to be unreachable")
	}
	if status.Controller.Error == "" {
		t.Fatalf("expected controller error")
	}
	if status.Controller.EnvironmentLiveness != controllerEnvironmentLivenessUnknown {
		t.Fatalf("expected unknown controller liveness, got %s", status.Controller.EnvironmentLiveness)
	}
}

func TestDeriveControllerEnvironmentLiveness(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-20 * time.Second)
	stale := now.Add(-2 * time.Minute)

	if got := deriveControllerEnvironmentLiveness(now, &controllerEnvironment{State: string(api.EnvironmentStateEnabled), LastSeenAt: &fresh}); got != controllerEnvironmentLivenessOnline {
		t.Fatalf("expected online liveness, got %s", got)
	}
	if got := deriveControllerEnvironmentLiveness(now, &controllerEnvironment{State: string(api.EnvironmentStateEnabled), LastSeenAt: &stale}); got != controllerEnvironmentLivenessStale {
		t.Fatalf("expected stale liveness, got %s", got)
	}
	if got := deriveControllerEnvironmentLiveness(now, &controllerEnvironment{State: string(api.EnvironmentStateEnabled)}); got != controllerEnvironmentLivenessUnknown {
		t.Fatalf("expected unknown liveness, got %s", got)
	}
	if got := deriveControllerEnvironmentLiveness(now, &controllerEnvironment{State: string(api.EnvironmentStateDisabled)}); got != controllerEnvironmentLivenessDisabled {
		t.Fatalf("expected disabled liveness, got %s", got)
	}
}

type statusTestRoot struct {
	apiEndpoint string
	environment *env_core.Environment
	network     *env_core.Network
}

func (r *statusTestRoot) Metadata() *env_core.Metadata { return &env_core.Metadata{} }
func (r *statusTestRoot) Obliterate() error            { panic("unused") }
func (r *statusTestRoot) HasConfig() (bool, error)     { return false, nil }
func (r *statusTestRoot) Config() *env_core.Config {
	return &env_core.Config{APIEndpoint: r.apiEndpoint}
}
func (r *statusTestRoot) SetConfig(*env_core.Config) error           { panic("unused") }
func (r *statusTestRoot) Client() (*api.Client, error)               { panic("unused") }
func (r *statusTestRoot) APIEndpoint() (string, string)              { return r.apiEndpoint, "test" }
func (r *statusTestRoot) IsEnabled() bool                            { return r.environment != nil }
func (r *statusTestRoot) Environment() *env_core.Environment         { return r.environment }
func (r *statusTestRoot) SetEnvironment(*env_core.Environment) error { panic("unused") }
func (r *statusTestRoot) DeleteEnvironment() error                   { panic("unused") }
func (r *statusTestRoot) Network() *env_core.Network                 { return r.network }
func (r *statusTestRoot) SetNetwork(*env_core.Network) error         { panic("unused") }
func (r *statusTestRoot) DeleteNetwork() error                       { panic("unused") }
func (r *statusTestRoot) NetworkSocketPath() (string, error)         { return "/tmp/agora-network.sock", nil }
func (r *statusTestRoot) ZitiIdentityNamed(string) (string, error)   { panic("unused") }
func (r *statusTestRoot) SaveZitiIdentityNamed(string, string) error { panic("unused") }
func (r *statusTestRoot) DeleteZitiIdentityNamed(string) error       { panic("unused") }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(t *testing.T, statusCode int, v any) *http.Response {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal JSON response: %v", err)
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func TestFormatAttachmentCounts(t *testing.T) {
	t.Parallel()

	if got := formatAttachmentCounts(attachmentCounts{}); got != "-" {
		t.Fatalf("expected dash for empty counts, got %q", got)
	}
	if got := formatAttachmentCounts(attachmentCounts{Active: 1, Stale: 2, Disconnected: 3}); !strings.Contains(got, "1 active / 2 stale / 3 disconnected") {
		t.Fatalf("unexpected formatted attachment counts: %q", got)
	}
}

var _ env_core.Root = (*statusTestRoot)(nil)
