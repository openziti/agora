package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/network/daemon"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	tunnelCmd.AddCommand(newTunnelDeleteCommand().cmd)
}

type tunnelDeleteCommand struct {
	cmd *cobra.Command
}

func newTunnelDeleteCommand() *tunnelDeleteCommand {
	cmd := &cobra.Command{
		Use:   "delete <name|tt_...|ts_...|ta_...>",
		Short: "Delete tunnel resources or managed runtime state",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelDeleteCommand{cmd: cmd}
	cmd.Run = command.run
	cmd.AddCommand(newTunnelDeleteServeCommand().cmd)
	cmd.AddCommand(newTunnelDeleteConnectCommand().cmd)
	return command
}

func (cmd *tunnelDeleteCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	token := args[0]

	switch {
	case isTunnelServeID(token):
		client, cleanup := requireRunningNetworkClient(root)
		defer cleanup()
		_, err := client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{ServeId: token})
		panicIfErr(err)
		fmt.Printf("deleted managed serve '%s'\n", token)
		return
	case isTunnelAttachmentID(token):
		client, cleanup := requireRunningNetworkClient(root)
		defer cleanup()
		_, err := client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{AttachmentId: token})
		panicIfErr(err)
		fmt.Printf("deleted managed connect '%s'\n", token)
		return
	}

	client := openEnvironmentAPIClient(root)
	tunnel := resolveManagedTunnelIdentifier(client, token)
	drainManagedTunnelState(root, tunnel.ID, tunnel.Name)

	res, err := client.DeleteTunnel(context.Background(), api.DeleteTunnelParams{TunnelId: tunnel.ID})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.DeleteTunnelNoContent:
		fmt.Printf("deleted tunnel '%s'\n", tunnel.Name)
	case *api.DeleteTunnelNotFound:
		panic(typed.Message)
	case *api.DeleteTunnelUnauthorized:
		panic(typed.Message)
	case *api.DeleteTunnelInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected delete tunnel response: %T", res))
	}
}

type tunnelDeleteServeCommand struct {
	cmd *cobra.Command
}

func newTunnelDeleteServeCommand() *tunnelDeleteServeCommand {
	cmd := &cobra.Command{
		Use:   "serve <name|tt_...>",
		Short: "Delete managed serve state",
		Args:  cobra.ExactArgs(1),
	}
	command := &tunnelDeleteServeCommand{cmd: cmd}
	cmd.Run = command.run
	return command
}

func (cmd *tunnelDeleteServeCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	token := args[0]
	tunnelID := ""
	name := token
	if isTunnelID(token) {
		tunnelID = token
		name = ""
	}

	if client, cleanup, err := openNetworkClient(root); err == nil {
		defer cleanup()
		_, err = client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{
			TunnelId: tunnelID,
			Name:     name,
		})
		panicIfErr(err)
		fmt.Printf("deleted managed serve '%s'\n", token)
		return
	} else if err != nil && !isErrNotRunning(err) {
		panic(err)
	}

	removed, err := removeLocalDesiredServe(root, tunnelID, name)
	panicIfErr(err)
	if !removed {
		panic("managed serve not found")
	}
	fmt.Printf("deleted managed serve '%s'\n", token)
}

type tunnelDeleteConnectCommand struct {
	listen string
	all    bool
	cmd    *cobra.Command
}

func newTunnelDeleteConnectCommand() *tunnelDeleteConnectCommand {
	cmd := &cobra.Command{
		Use:   "connect <name|tt_...>",
		Short: "Delete managed connect state",
		Long: "Delete managed connect state for one listen address (--listen) or for every matching listen address (--all).\n\n" +
			"With --all, selection uses only locally saved connects: a tunnel ID matches connects saved with that ID (or, when " +
			"no ID was ever resolved, saved under that literal ID as their name); a name matches saved names case-insensitively. " +
			"Matching connects are removed even when actively connected. The controller tunnel and local serves are not affected.",
		Args: cobra.ExactArgs(1),
	}
	command := &tunnelDeleteConnectCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.listen, "listen", "", "Local listen address for the managed connect")
	cmd.Flags().BoolVar(&command.all, "all", false, "Delete every locally saved connect matching the reference across all listen addresses, including active connections")
	cmd.RunE = command.runE
	return command
}

func (cmd *tunnelDeleteConnectCommand) runE(_ *cobra.Command, args []string) (err error) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		if panicInstead {
			panic(recovered)
		}
		switch typed := recovered.(type) {
		case error:
			err = typed
		default:
			err = fmt.Errorf("%v", typed)
		}
	}()

	bulk, err := cmd.bulkMode()
	if err != nil {
		return err
	}
	if bulk {
		return cmd.runBulk(args[0])
	}
	cmd.runSingle(args[0])
	return nil
}

// bulkMode validates the removal mode before any environment or daemon access. an explicitly supplied --listen
// counts as single-address mode even when empty, so it is rejected alongside --all and validated otherwise.
func (cmd *tunnelDeleteConnectCommand) bulkMode() (bool, error) {
	listenSet := cmd.cmd.Flags().Changed("listen")
	switch {
	case cmd.all && listenSet:
		return false, errors.New("--listen and --all cannot be used together")
	case !cmd.all && !listenSet:
		return false, errors.New("one of --listen or --all is required")
	case cmd.all:
		return true, nil
	}
	if err := validateListenAddress(cmd.listen); err != nil {
		return false, err
	}
	return false, nil
}

func (cmd *tunnelDeleteConnectCommand) runSingle(token string) {
	root := requireEnabledRoot()
	tunnelID := ""
	name := token
	if isTunnelID(token) {
		tunnelID = token
		name = ""
	}

	if client, cleanup, err := openNetworkClient(root); err == nil {
		defer cleanup()
		_, err = client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{
			TunnelId:      tunnelID,
			Name:          name,
			ListenAddress: cmd.listen,
		})
		panicIfErr(err)
		fmt.Printf("deleted managed connect '%s' listen='%s'\n", token, cmd.listen)
		return
	} else if err != nil && !isErrNotRunning(err) {
		panic(err)
	}

	removed, err := removeLocalDesiredConnect(root, tunnelID, name, cmd.listen)
	panicIfErr(err)
	if !removed {
		panic("managed connect not found")
	}
	fmt.Printf("deleted managed connect '%s' listen='%s'\n", token, cmd.listen)
}

func (cmd *tunnelDeleteConnectCommand) runBulk(reference string) error {
	root := requireEnabledRoot()

	var removed int
	if client, cleanup, err := openNetworkClient(root); err == nil {
		defer cleanup()
		removed, err = removeDaemonConnects(client, reference, tunnelConnectBulkRPCTimeout)
		if err != nil {
			return err
		}
	} else if !isErrNotRunning(err) {
		return err
	} else {
		// the daemon probe also reports timeouts and cancellation as not running, so confirm absence at the socket
		// before editing local state.
		if err := confirmNetworkAbsent(networkSocketPath(root)); err != nil {
			return err
		}
		removed, err = removeLocalDesiredConnects(root, reference)
		if err != nil {
			return err
		}
	}

	if removed == 0 {
		fmt.Printf("no managed connects matched '%s'\n", reference)
		return nil
	}
	fmt.Printf("deleted %d managed connect(s) for '%s'\n", removed, reference)
	return nil
}

const tunnelConnectBulkRPCTimeout = 10 * time.Second

// windowsConnRefused is WSAECONNREFUSED, which windows reports for a socket file with no listener.
const windowsConnRefused = syscall.Errno(10061)

// confirmNetworkAbsent returns nil only when socket-level evidence shows no daemon is listening at socketPath: the
// socket file is missing, or connecting to it is refused. a daemon that accepts but does not answer, or any other
// probe failure, may still be running and own the desired state, so local network state must not be edited.
func confirmNetworkAbsent(socketPath string) error {
	if _, err := os.Lstat(socketPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	conn, err := net.DialTimeout("unix", socketPath, networkDialTimeout)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("agora network accepted a connection at '%s' but did not answer; refusing to edit local network state while it may be running", socketPath)
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) || (runtime.GOOS == "windows" && errors.Is(err, windowsConnRefused)) {
		return nil
	}
	return fmt.Errorf("could not confirm agora network is stopped at '%s'; refusing to edit local network state: %w", socketPath, err)
}

// connectBulkClient is the subset of the daemon client used for bulk connect removal.
type connectBulkClient interface {
	ListConnects(ctx context.Context, in *networkpb.ListConnectsRequest, opts ...grpc.CallOption) (*networkpb.ListConnectsResponse, error)
	RemoveConnect(ctx context.Context, in *networkpb.RemoveConnectRequest, opts ...grpc.CallOption) (*networkpb.RemoveConnectResponse, error)
}

// connectMatchesReference reports whether a locally saved connect is selected by a bulk removal reference. a
// tunnel ID selects connects saved with that ID, and connects that never resolved an ID but were saved under the
// literal ID as their name. a name selects saved names case-insensitively; it is never resolved to an ID.
func connectMatchesReference(reference, tunnelID, name string) bool {
	if isTunnelID(reference) {
		if tunnelID != "" {
			return tunnelID == reference
		}
		return name == reference
	}
	return strings.EqualFold(name, reference)
}

// removeDaemonConnects removes every daemon-managed connect matching reference, one listen address at a time. it
// returns the number of confirmed removals; on failure the error carries that count, and the remaining matches
// are left for a later invocation.
func removeDaemonConnects(client connectBulkClient, reference string, timeout time.Duration) (int, error) {
	listCtx, cancel := context.WithTimeout(context.Background(), timeout)
	res, err := client.ListConnects(listCtx, &networkpb.ListConnectsRequest{})
	cancel()
	if err != nil {
		return 0, fmt.Errorf("failed to list managed connects for '%s': %w", reference, err)
	}

	removed := 0
	for _, connect := range res.GetConnects() {
		desired := connect.GetDesired()
		tunnelID := desired.GetTunnelId()
		if tunnelID == "" {
			tunnelID = connect.GetTunnelId()
		}
		if !connectMatchesReference(reference, tunnelID, desired.GetName()) {
			continue
		}

		// identify the row by exactly one saved field: the runtime treats tunnel ID and name as alternatives, so
		// sending both could select a different tunnel under a reused name. the attachment ID is left empty
		// because heartbeat recovery can replace it between listing and removal.
		req := &networkpb.RemoveConnectRequest{ListenAddress: desired.GetListenAddress()}
		if desired.GetTunnelId() != "" {
			req.TunnelId = desired.GetTunnelId()
		} else {
			req.Name = desired.GetName()
		}

		removeCtx, cancel := context.WithTimeout(context.Background(), timeout)
		_, err := client.RemoveConnect(removeCtx, req)
		cancel()
		if err != nil {
			return removed, bulkConnectRemoveError(reference, desired, removed, err)
		}
		removed++
	}
	return removed, nil
}

func bulkConnectRemoveError(reference string, desired *networkpb.DesiredConnect, removed int, err error) error {
	unconfirmed := ""
	if errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
		unconfirmed = "; completion of this removal is unconfirmed"
	}
	return fmt.Errorf(
		"failed to remove managed connect '%s' listen='%s' after deleting %d managed connect(s) for '%s'%s; rerun to remove any remaining connects: %w",
		desired.GetName(), desired.GetListenAddress(), removed, reference, unconfirmed, err,
	)
}

func drainManagedTunnelState(root env_core.Root, tunnelID, name string) {
	if client, cleanup, err := openNetworkClient(root); err == nil {
		defer cleanup()
		status, err := client.GetNetworkStatus(context.Background(), &networkpb.GetNetworkStatusRequest{})
		panicIfErr(err)
		for _, serve := range status.Status.Serves {
			if serve.TunnelId != tunnelID && !strings.EqualFold(serve.Desired.Name, name) {
				continue
			}
			_, err := client.RemoveServe(context.Background(), &networkpb.RemoveServeRequest{
				ServeId:  serve.ServeId,
				TunnelId: serve.TunnelId,
				Name:     serve.Desired.Name,
			})
			panicIfErr(err)
		}
		for _, connect := range status.Status.Connects {
			if connect.TunnelId != tunnelID && !strings.EqualFold(connect.Desired.Name, name) {
				continue
			}
			_, err := client.RemoveConnect(context.Background(), &networkpb.RemoveConnectRequest{
				AttachmentId:  connect.AttachmentId,
				TunnelId:      connect.TunnelId,
				Name:          connect.Desired.Name,
				ListenAddress: connect.Desired.ListenAddress,
			})
			panicIfErr(err)
		}
		return
	} else if err != nil && !isErrNotRunning(err) {
		panic(err)
	}

	panicIfErr(removeLocalDesiredTunnel(root, tunnelID, name))
}

func isErrNotRunning(err error) bool {
	return errors.Is(err, daemon.ErrNotRunning)
}
