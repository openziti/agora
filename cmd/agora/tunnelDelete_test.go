package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	bulkTunnelA = "tt_bulkaaaaaaaa"
	bulkTunnelB = "tt_bulkbbbbbbbb"
	bulkTunnelC = "tt_bulkcccccccc"
)

func TestTunnelDeleteConnectBulkMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flags   []string
		bulk    bool
		wantErr string
	}{
		{name: "listen only", flags: []string{"--listen", "127.0.0.1:8080"}},
		{name: "explicit all false with listen", flags: []string{"--all=false", "--listen", "127.0.0.1:8080"}},
		{name: "all only", flags: []string{"--all"}, bulk: true},
		{name: "all true", flags: []string{"--all=true"}, bulk: true},
		{name: "both", flags: []string{"--all", "--listen", "127.0.0.1:8080"}, wantErr: "cannot be used together"},
		{name: "all with empty listen", flags: []string{"--all", "--listen="}, wantErr: "cannot be used together"},
		{name: "neither", flags: nil, wantErr: "one of --listen or --all is required"},
		{name: "explicit all false alone", flags: []string{"--all=false"}, wantErr: "one of --listen or --all is required"},
		{name: "invalid listen", flags: []string{"--listen", "8080"}, wantErr: "missing port"},
		{name: "empty listen", flags: []string{"--listen="}, wantErr: "missing port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := newTunnelDeleteConnectCommand()
			if err := command.cmd.ParseFlags(tt.flags); err != nil {
				t.Fatalf("parse flags: %v", err)
			}
			bulk, err := command.bulkMode()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bulk != tt.bulk {
				t.Fatalf("expected bulk=%v, got %v", tt.bulk, bulk)
			}
		})
	}
}

func TestTunnelDeleteConnectValidatesModeBeforeEnvironment(t *testing.T) {
	useTempEnvironmentRoot(t)

	tests := []struct {
		name    string
		flags   []string
		wantErr string
	}{
		{name: "both", flags: []string{"--all", "--listen", "127.0.0.1:8080"}, wantErr: "cannot be used together"},
		{name: "neither", flags: []string{"--all=false"}, wantErr: "one of --listen or --all is required"},
		{name: "invalid listen", flags: []string{"--listen", "8080"}, wantErr: "missing port"},
		// valid forms proceed to environment loading, which fails here because no environment is enabled.
		{name: "single address reaches environment", flags: []string{"--all=false", "--listen", "127.0.0.1:8080"}, wantErr: "no environment is enabled"},
		{name: "bulk reaches environment", flags: []string{"--all"}, wantErr: "no environment is enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := newTunnelDeleteConnectCommand()
			if err := command.cmd.ParseFlags(tt.flags); err != nil {
				t.Fatalf("parse flags: %v", err)
			}
			err := command.runE(command.cmd, []string{bulkTunnelA})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestConnectMatchesReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference string
		tunnelID  string
		saved     string
		want      bool
	}{
		{name: "id matches stored id", reference: bulkTunnelA, tunnelID: bulkTunnelA, saved: "session-ses_one", want: true},
		{name: "id matches stored id under id name", reference: bulkTunnelA, tunnelID: bulkTunnelA, saved: bulkTunnelA, want: true},
		{name: "id matches unresolved literal name", reference: bulkTunnelA, saved: bulkTunnelA, want: true},
		{name: "id literal name match is exact", reference: bulkTunnelA, saved: strings.ToUpper(bulkTunnelA), want: false},
		{name: "id ignores other stored id", reference: bulkTunnelA, tunnelID: bulkTunnelB, saved: "session-ses_one", want: false},
		{name: "id ignores literal name with other stored id", reference: bulkTunnelA, tunnelID: bulkTunnelB, saved: bulkTunnelA, want: false},
		{name: "name matches case-insensitively", reference: "SESSION-ses_one", tunnelID: bulkTunnelA, saved: "session-ses_one", want: true},
		{name: "name matches unresolved", reference: "gateway", saved: "Gateway", want: true},
		{name: "name does not match id alias", reference: "session-ses_one", tunnelID: bulkTunnelA, saved: bulkTunnelA, want: false},
		{name: "name is not a prefix match", reference: "session-ses", tunnelID: bulkTunnelA, saved: "session-ses_one", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connectMatchesReference(tt.reference, tt.tunnelID, tt.saved); got != tt.want {
				t.Fatalf("connectMatchesReference(%q, %q, %q) = %v, want %v", tt.reference, tt.tunnelID, tt.saved, got, tt.want)
			}
		})
	}
}

func TestRemoveDaemonConnectsByID(t *testing.T) {
	t.Parallel()

	client := &fakeConnectBulkClient{connects: bulkDaemonConnects()}
	removed, err := removeDaemonConnects(client, bulkTunnelA, time.Second)
	if err != nil {
		t.Fatalf("remove daemon connects: %v", err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 removals, got %d", removed)
	}
	assertRemoveRequests(t, client.removes, []*networkpb.RemoveConnectRequest{
		{TunnelId: bulkTunnelA, ListenAddress: "127.0.0.1:8401"},
		{TunnelId: bulkTunnelA, ListenAddress: "127.0.0.1:8402"},
		{Name: bulkTunnelA, ListenAddress: "127.0.0.1:8403"},
	})
}

func TestRemoveDaemonConnectsByName(t *testing.T) {
	t.Parallel()

	client := &fakeConnectBulkClient{connects: bulkDaemonConnects()}
	removed, err := removeDaemonConnects(client, "SESSION-ses_one", time.Second)
	if err != nil {
		t.Fatalf("remove daemon connects: %v", err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 removals, got %d", removed)
	}
	// each selected row is removed by its own saved identifier, never by the user's token, and the ID aliases of
	// the same tunnel are not expanded into the selection.
	assertRemoveRequests(t, client.removes, []*networkpb.RemoveConnectRequest{
		{TunnelId: bulkTunnelA, ListenAddress: "127.0.0.1:8401"},
		{TunnelId: bulkTunnelC, ListenAddress: "127.0.0.1:8404"},
		{Name: "session-SES_one", ListenAddress: "127.0.0.1:8406"},
	})
}

func TestRemoveDaemonConnectsNoMatches(t *testing.T) {
	t.Parallel()

	client := &fakeConnectBulkClient{connects: bulkDaemonConnects()}
	removed, err := removeDaemonConnects(client, "tt_bulkmissing00", time.Second)
	if err != nil {
		t.Fatalf("remove daemon connects: %v", err)
	}
	if removed != 0 || len(client.removes) != 0 {
		t.Fatalf("expected no removals, got count=%d requests=%v", removed, client.removes)
	}
}

func TestRemoveDaemonConnectsListFailurePerformsNoRemovals(t *testing.T) {
	t.Parallel()

	client := &fakeConnectBulkClient{listErr: status.Error(codes.Unavailable, "daemon went away")}
	removed, err := removeDaemonConnects(client, bulkTunnelA, time.Second)
	if err == nil || !strings.Contains(err.Error(), "failed to list managed connects") {
		t.Fatalf("expected list failure, got %v", err)
	}
	if removed != 0 || len(client.removes) != 0 {
		t.Fatalf("expected no removals, got count=%d requests=%v", removed, client.removes)
	}
}

func TestRemoveDaemonConnectsPartialFailure(t *testing.T) {
	t.Parallel()

	removeErr := status.Error(codes.Internal, "persist failed")
	client := &fakeConnectBulkClient{connects: bulkDaemonConnects(), failAt: 2, removeErr: removeErr}
	removed, err := removeDaemonConnects(client, bulkTunnelA, time.Second)
	if err == nil {
		t.Fatal("expected removal failure")
	}
	if removed != 1 {
		t.Fatalf("expected 1 confirmed removal, got %d", removed)
	}
	if len(client.removes) != 2 {
		t.Fatalf("expected removal to stop at the failed request, got %v", client.removes)
	}
	if !errors.Is(err, removeErr) {
		t.Fatalf("expected wrapped removal error, got %v", err)
	}
	for _, want := range []string{"after deleting 1 managed connect(s)", "listen='127.0.0.1:8402'", "rerun"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %q", want, err.Error())
		}
	}
	if strings.Contains(err.Error(), "unconfirmed") {
		t.Fatalf("expected a definite failure not to be reported as unconfirmed, got %q", err.Error())
	}
}

func TestRemoveDaemonConnectsTimeoutIsUnconfirmed(t *testing.T) {
	t.Parallel()

	client := &fakeConnectBulkClient{connects: bulkDaemonConnects(), failAt: 1, removeErr: status.Error(codes.DeadlineExceeded, "context deadline exceeded")}
	removed, err := removeDaemonConnects(client, bulkTunnelA, time.Second)
	if err == nil || !strings.Contains(err.Error(), "unconfirmed") {
		t.Fatalf("expected unconfirmed timeout error, got %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected no confirmed removals, got %d", removed)
	}
}

func TestRemoveLocalDesiredConnectsByID(t *testing.T) {
	root := useTempEnvironmentRoot(t)
	if err := root.SetNetwork(bulkLocalNetwork()); err != nil {
		t.Fatalf("set network: %v", err)
	}

	removed, err := removeLocalDesiredConnects(root, bulkTunnelA)
	if err != nil {
		t.Fatalf("remove local connects: %v", err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 removals, got %d", removed)
	}

	expected := bulkLocalNetwork()
	expected.Connects = []env_core.ManagedConnect{
		expected.Connects[3],
		expected.Connects[4],
		expected.Connects[5],
		expected.Connects[6],
	}
	assertPersistedNetwork(t, expected)

	// a repeated cleanup is a successful no-op that leaves the file untouched.
	before := readNetworkFile(t)
	root = reloadEnvironmentRoot(t)
	removed, err = removeLocalDesiredConnects(root, bulkTunnelA)
	if err != nil {
		t.Fatalf("repeat remove local connects: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected repeat cleanup to remove nothing, got %d", removed)
	}
	if after := readNetworkFile(t); after != before {
		t.Fatalf("expected repeat cleanup not to rewrite network state\nbefore: %s\nafter: %s", before, after)
	}
}

func TestRemoveLocalDesiredConnectsByName(t *testing.T) {
	root := useTempEnvironmentRoot(t)
	if err := root.SetNetwork(bulkLocalNetwork()); err != nil {
		t.Fatalf("set network: %v", err)
	}

	removed, err := removeLocalDesiredConnects(root, "SESSION-ses_one")
	if err != nil {
		t.Fatalf("remove local connects: %v", err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 removals, got %d", removed)
	}

	expected := bulkLocalNetwork()
	expected.Connects = []env_core.ManagedConnect{
		expected.Connects[1],
		expected.Connects[2],
		expected.Connects[3],
		expected.Connects[5],
	}
	assertPersistedNetwork(t, expected)
}

func TestRemoveLocalDesiredConnectsMissingNetworkFile(t *testing.T) {
	root := useTempEnvironmentRoot(t)

	removed, err := removeLocalDesiredConnects(root, bulkTunnelA)
	if err != nil {
		t.Fatalf("remove local connects: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected no removals, got %d", removed)
	}
	if _, err := os.Stat(networkFilePath(t)); !os.IsNotExist(err) {
		t.Fatalf("expected no network file to be created, got %v", err)
	}
}

func TestRemoveLocalDesiredConnectsPreservesServeOnlyState(t *testing.T) {
	root := useTempEnvironmentRoot(t)
	network := bulkLocalNetwork()
	network.Connects = network.Connects[:3]
	if err := root.SetNetwork(network); err != nil {
		t.Fatalf("set network: %v", err)
	}

	removed, err := removeLocalDesiredConnects(root, bulkTunnelA)
	if err != nil {
		t.Fatalf("remove local connects: %v", err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 removals, got %d", removed)
	}
	expected := bulkLocalNetwork()
	expected.Connects = nil
	assertPersistedNetwork(t, expected)
}

func TestRemoveLocalDesiredConnectsDeletesEmptyState(t *testing.T) {
	root := useTempEnvironmentRoot(t)
	network := bulkLocalNetwork()
	network.Serves = nil
	network.Connects = network.Connects[:3]
	if err := root.SetNetwork(network); err != nil {
		t.Fatalf("set network: %v", err)
	}

	removed, err := removeLocalDesiredConnects(root, bulkTunnelA)
	if err != nil {
		t.Fatalf("remove local connects: %v", err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 removals, got %d", removed)
	}
	if _, err := os.Stat(networkFilePath(t)); !os.IsNotExist(err) {
		t.Fatalf("expected network file to be deleted, got %v", err)
	}
	if reloaded := reloadEnvironmentRoot(t).Network(); reloaded != nil {
		t.Fatalf("expected no network state after reload, got %#v", reloaded)
	}
}

func TestRemoveLocalDesiredConnectsPersistenceFailure(t *testing.T) {
	persistErr := errors.New("disk full")

	t.Run("save", func(t *testing.T) {
		root := useTempEnvironmentRoot(t)
		if err := root.SetNetwork(bulkLocalNetwork()); err != nil {
			t.Fatalf("set network: %v", err)
		}
		loaded := root.Network()
		snapshot := append([]env_core.ManagedConnect(nil), loaded.Connects...)

		removed, err := removeLocalDesiredConnects(&failingNetworkRoot{Root: root, err: persistErr}, bulkTunnelA)
		if !errors.Is(err, persistErr) {
			t.Fatalf("expected persistence error, got %v", err)
		}
		if removed != 0 {
			t.Fatalf("expected no reported removals on failure, got %d", removed)
		}
		if !reflect.DeepEqual(loaded.Connects, snapshot) {
			t.Fatalf("expected loaded connects to be unmodified, got %#v", loaded.Connects)
		}
		assertPersistedNetwork(t, bulkLocalNetwork())
	})

	t.Run("delete", func(t *testing.T) {
		root := useTempEnvironmentRoot(t)
		network := bulkLocalNetwork()
		network.Serves = nil
		network.Connects = network.Connects[:3]
		if err := root.SetNetwork(network); err != nil {
			t.Fatalf("set network: %v", err)
		}

		removed, err := removeLocalDesiredConnects(&failingNetworkRoot{Root: root, err: persistErr}, bulkTunnelA)
		if !errors.Is(err, persistErr) {
			t.Fatalf("expected persistence error, got %v", err)
		}
		if removed != 0 {
			t.Fatalf("expected no reported removals on failure, got %d", removed)
		}
		assertPersistedNetwork(t, network)
	})
}

// bulkDaemonConnects is a daemon snapshot covering one tunnel saved under several names and ports, an unresolved
// ID-as-name row, a different tunnel on a shared port, a case-variant name on another tunnel, and an unrelated row.
func bulkDaemonConnects() []*networkpb.ManagedConnectStatus {
	return []*networkpb.ManagedConnectStatus{
		{
			Desired:      &networkpb.DesiredConnect{TunnelId: bulkTunnelA, Name: "session-ses_one", ListenAddress: "127.0.0.1:8401"},
			State:        networkpb.RuntimeState_RUNTIME_STATE_RUNNING,
			TunnelId:     bulkTunnelA,
			AttachmentId: "ta_bulkaaaaaaa1",
		},
		{
			Desired:      &networkpb.DesiredConnect{TunnelId: bulkTunnelA, Name: bulkTunnelA, ListenAddress: "127.0.0.1:8402"},
			State:        networkpb.RuntimeState_RUNTIME_STATE_ERROR,
			TunnelId:     bulkTunnelA,
			RetryAttempt: 4,
		},
		{
			Desired:      &networkpb.DesiredConnect{Name: bulkTunnelA, ListenAddress: "127.0.0.1:8403"},
			State:        networkpb.RuntimeState_RUNTIME_STATE_ERROR,
			RetryAttempt: 9,
		},
		{
			Desired:      &networkpb.DesiredConnect{TunnelId: bulkTunnelB, Name: "session-ses_two", ListenAddress: "127.0.0.1:8401"},
			State:        networkpb.RuntimeState_RUNTIME_STATE_RUNNING,
			TunnelId:     bulkTunnelB,
			AttachmentId: "ta_bulkbbbbbbb1",
		},
		{
			Desired:  &networkpb.DesiredConnect{TunnelId: bulkTunnelC, Name: "Session-SES_one", ListenAddress: "127.0.0.1:8404"},
			State:    networkpb.RuntimeState_RUNTIME_STATE_RUNNING,
			TunnelId: bulkTunnelC,
		},
		{
			Desired: &networkpb.DesiredConnect{Name: "other", ListenAddress: "127.0.0.1:8405"},
			State:   networkpb.RuntimeState_RUNTIME_STATE_CONFIGURED,
		},
		{
			Desired: &networkpb.DesiredConnect{Name: "session-SES_one", ListenAddress: "127.0.0.1:8406"},
			State:   networkpb.RuntimeState_RUNTIME_STATE_ERROR,
		},
	}
}

// bulkLocalNetwork mirrors bulkDaemonConnects as offline desired state, with serves and attachment recovery tokens.
func bulkLocalNetwork() *env_core.Network {
	return &env_core.Network{
		Serves: []env_core.ManagedServe{
			{TunnelID: bulkTunnelA, Name: "session-ses_one", Mode: api.TunnelModeHTTP, BackendTarget: "http://127.0.0.1:9000", GrantEmails: []string{"one@example.com"}},
			{Name: "gateway", Mode: api.TunnelModeTCP, BackendTarget: "127.0.0.1:9001"},
		},
		Connects: []env_core.ManagedConnect{
			{TunnelID: bulkTunnelA, Name: "session-ses_one", ListenAddress: "127.0.0.1:8401", AttachmentID: "ta_bulkaaaaaaa1"},
			{TunnelID: bulkTunnelA, Name: bulkTunnelA, ListenAddress: "127.0.0.1:8402", AttachmentID: "ta_bulkaaaaaaa2"},
			{Name: bulkTunnelA, ListenAddress: "127.0.0.1:8403"},
			{TunnelID: bulkTunnelB, Name: "session-ses_two", ListenAddress: "127.0.0.1:8401", AttachmentID: "ta_bulkbbbbbbb1"},
			{TunnelID: bulkTunnelC, Name: "Session-SES_one", ListenAddress: "127.0.0.1:8404", AttachmentID: "ta_bulkccccccc1"},
			{Name: "other", ListenAddress: "127.0.0.1:8405", AttachmentID: "ta_bulkddddddd1"},
			{Name: "session-SES_one", ListenAddress: "127.0.0.1:8406"},
		},
	}
}

type fakeConnectBulkClient struct {
	connects  []*networkpb.ManagedConnectStatus
	listErr   error
	failAt    int
	removeErr error
	removes   []*networkpb.RemoveConnectRequest
}

func (f *fakeConnectBulkClient) ListConnects(ctx context.Context, _ *networkpb.ListConnectsRequest, _ ...grpc.CallOption) (*networkpb.ListConnectsResponse, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("list connects called without a deadline")
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &networkpb.ListConnectsResponse{Connects: f.connects}, nil
}

func (f *fakeConnectBulkClient) RemoveConnect(ctx context.Context, req *networkpb.RemoveConnectRequest, _ ...grpc.CallOption) (*networkpb.RemoveConnectResponse, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("remove connect called without a deadline")
	}
	f.removes = append(f.removes, req)
	if f.failAt != 0 && len(f.removes) == f.failAt {
		return nil, f.removeErr
	}
	return &networkpb.RemoveConnectResponse{}, nil
}

type failingNetworkRoot struct {
	env_core.Root
	err error
}

func (r *failingNetworkRoot) SetNetwork(*env_core.Network) error { return r.err }
func (r *failingNetworkRoot) DeleteNetwork() error               { return r.err }

func assertRemoveRequests(t *testing.T, got, want []*networkpb.RemoveConnectRequest) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d remove requests, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i].AttachmentId != "" || got[i].TunnelId != want[i].TunnelId || got[i].Name != want[i].Name || got[i].ListenAddress != want[i].ListenAddress {
			t.Fatalf("remove request %d: expected %v, got %v", i, want[i], got[i])
		}
	}
}

// useTempEnvironmentRoot points the environment package's global root at a fresh temporary directory. tests that
// call it must not run in parallel.
func useTempEnvironmentRoot(t *testing.T) env_core.Root {
	t.Helper()
	environment.SetRootDirName(filepath.Join(t.TempDir(), ".agora"))
	t.Cleanup(func() { environment.SetRootDirName(".agora") })
	return reloadEnvironmentRoot(t)
}

func reloadEnvironmentRoot(t *testing.T) env_core.Root {
	t.Helper()
	root, err := environment.LoadRoot()
	if err != nil {
		t.Fatalf("load root: %v", err)
	}
	return root
}

func assertPersistedNetwork(t *testing.T, expected *env_core.Network) {
	t.Helper()
	got := reloadEnvironmentRoot(t).Network()
	if got == nil {
		t.Fatalf("expected persisted network state %#v, got none", expected)
	}
	// empty and nil slices are equivalent once persisted.
	if len(got.Serves) == 0 && len(expected.Serves) == 0 {
		got.Serves, expected.Serves = nil, nil
	}
	if len(got.Connects) == 0 && len(expected.Connects) == 0 {
		got.Connects, expected.Connects = nil, nil
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected persisted network state\nexpected: %#v\ngot:      %#v", expected, got)
	}
}

func networkFilePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(reloadEnvironmentRoot(t).Metadata().RootPath, "network.json")
}

func readNetworkFile(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(networkFilePath(t))
	if err != nil {
		t.Fatalf("read network file: %v", err)
	}
	return string(data)
}

func TestConfirmNetworkAbsent(t *testing.T) {
	t.Parallel()

	t.Run("missing socket", func(t *testing.T) {
		t.Parallel()
		if err := confirmNetworkAbsent(filepath.Join(t.TempDir(), "network.sock")); err != nil {
			t.Fatalf("expected a missing socket to confirm absence, got %v", err)
		}
	})
	t.Run("refused socket", func(t *testing.T) {
		t.Parallel()
		socketPath := filepath.Join(t.TempDir(), "network.sock")
		staleNetworkSocket(t, socketPath)
		if err := confirmNetworkAbsent(socketPath); err != nil {
			t.Fatalf("expected a refused socket to confirm absence, got %v", err)
		}
	})
	t.Run("accepting socket", func(t *testing.T) {
		t.Parallel()
		socketPath := filepath.Join(t.TempDir(), "network.sock")
		listenUnresponsiveNetwork(t, socketPath, false)
		err := confirmNetworkAbsent(socketPath)
		if err == nil || !strings.Contains(err.Error(), "may be running") {
			t.Fatalf("expected an accepting socket to leave absence unconfirmed, got %v", err)
		}
	})
}

// TestBulkConnectRouting drives the bulk command against each daemon socket condition. only a missing or refused
// socket may fall back to editing network.json; a daemon that accepts but does not answer leaves it untouched.
func TestBulkConnectRouting(t *testing.T) {
	ambiguous := []struct {
		name          string
		closeAccepted bool
	}{
		{name: "silent", closeAccepted: false},
		{name: "closing", closeAccepted: true},
	}
	for _, tc := range ambiguous {
		t.Run(tc.name, func(t *testing.T) {
			root := useEnabledBulkEnvironmentRoot(t)
			listenUnresponsiveNetwork(t, networkSocketPath(root), tc.closeAccepted)
			before := readNetworkFile(t)

			cmd := newTunnelDeleteConnectCommand()
			err := cmd.runBulk(bulkTunnelA)
			if err == nil || !strings.Contains(err.Error(), "may be running") {
				t.Fatalf("expected bulk removal to refuse an unconfirmed daemon, got %v", err)
			}
			if after := readNetworkFile(t); after != before {
				t.Fatalf("expected network state untouched\nbefore: %s\nafter: %s", before, after)
			}
		})
	}

	absent := []struct {
		name  string
		setup func(t *testing.T, socketPath string)
	}{
		{name: "missing", setup: func(*testing.T, string) {}},
		{name: "stale", setup: staleNetworkSocket},
	}
	for _, tc := range absent {
		t.Run(tc.name, func(t *testing.T) {
			root := useEnabledBulkEnvironmentRoot(t)
			tc.setup(t, networkSocketPath(root))

			cmd := newTunnelDeleteConnectCommand()
			if err := cmd.runBulk(bulkTunnelA); err != nil {
				t.Fatalf("expected offline bulk removal, got %v", err)
			}
			expected := bulkLocalNetwork()
			expected.Connects = expected.Connects[3:]
			assertPersistedNetwork(t, expected)
		})
	}
}

// useEnabledBulkEnvironmentRoot is useTempEnvironmentRoot with an enabled environment and the bulk fixture saved.
func useEnabledBulkEnvironmentRoot(t *testing.T) env_core.Root {
	t.Helper()
	root := useTempEnvironmentRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_bulk00000001",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SetNetwork(bulkLocalNetwork()); err != nil {
		t.Fatalf("set network: %v", err)
	}
	return reloadEnvironmentRoot(t)
}

// listenUnresponsiveNetwork serves socketPath with a listener that accepts connections but never answers; with
// closeAccepted it closes each connection immediately instead of holding it open.
func listenUnresponsiveNetwork(t *testing.T, socketPath string, closeAccepted bool) {
	t.Helper()
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen on '%s': %v", socketPath, err)
	}
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if closeAccepted {
				_ = conn.Close()
				continue
			}
			mu.Lock()
			held = append(held, conn)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range held {
			_ = conn.Close()
		}
	})
}

// staleNetworkSocket leaves a socket file at socketPath with nothing listening, as a crashed daemon would.
func staleNetworkSocket(t *testing.T, socketPath string) {
	t.Helper()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("listen on '%s': %v", socketPath, err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatalf("close stale listener: %v", err)
	}
	if _, err := os.Lstat(socketPath); err != nil {
		t.Fatalf("expected stale socket file to remain: %v", err)
	}
}
