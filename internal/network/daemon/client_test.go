package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/sdk/agent/networkpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDaemonCallsUseLiteralSocketPath(t *testing.T) {
	t.Chdir(t.TempDir())
	paths := []string{"network.sock", "network % #.sock"}
	if runtime.GOOS != "windows" {
		// on Unix, Windows paths are valid literal filenames, so this regression can run there too.
		paths = append(paths, `C:\Users\mikeg\.agora\network.sock`)
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			socketPath := path
			if path != `C:\Users\mikeg\.agora\network.sock` {
				var err error
				socketPath, err = filepath.Abs(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			listener, err := net.Listen("unix", socketPath)
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			networkpb.RegisterNetworkServiceServer(server, &testNetworkServer{})
			done := make(chan error, 1)
			go func() { done <- server.Serve(listener) }()
			t.Cleanup(func() {
				server.Stop()
				_ = listener.Close()
				if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
					t.Errorf("serve: %v", err)
				}
			})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ping, err := Ping(ctx, socketPath)
			if err != nil {
				t.Fatalf("ping: %v", err)
			}
			if ping.Pid != 42 {
				t.Fatalf("ping pid: got %d, want 42", ping.Pid)
			}
			networkStatus, err := Status(ctx, socketPath)
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if networkStatus.Pid != 42 {
				t.Fatalf("status pid: got %d, want 42", networkStatus.Pid)
			}
			if err := ReloadEnvironmentIfRunning(testSocketRoot{socketPath: socketPath}); err != nil {
				t.Fatalf("reload environment: %v", err)
			}
			if err := Stop(ctx, socketPath); err != nil {
				t.Fatalf("stop: %v", err)
			}
		})
	}
}

type testNetworkServer struct {
	networkpb.UnimplementedNetworkServiceServer
}

type testSocketRoot struct {
	env_core.Root
	socketPath string
}

func (r testSocketRoot) NetworkSocketPath() (string, error) {
	return r.socketPath, nil
}

func (*testNetworkServer) Ping(context.Context, *networkpb.PingRequest) (*networkpb.PingResponse, error) {
	return &networkpb.PingResponse{Pid: 42}, nil
}

func (*testNetworkServer) GetNetworkStatus(context.Context, *networkpb.GetNetworkStatusRequest) (*networkpb.GetNetworkStatusResponse, error) {
	return &networkpb.GetNetworkStatusResponse{Status: &networkpb.NetworkStatus{Pid: 42}}, nil
}

func (*testNetworkServer) Shutdown(context.Context, *networkpb.ShutdownRequest) (*networkpb.ShutdownResponse, error) {
	return &networkpb.ShutdownResponse{}, nil
}

func (*testNetworkServer) ReloadEnvironment(context.Context, *networkpb.ReloadEnvironmentRequest) (*networkpb.ReloadEnvironmentResponse, error) {
	return &networkpb.ReloadEnvironmentResponse{}, nil
}

func TestNormalizeDialErrorPreservesCause(t *testing.T) {
	for _, cause := range []error{
		context.Canceled,
		context.DeadlineExceeded,
		status.Error(codes.Canceled, "request canceled"),
		status.Error(codes.DeadlineExceeded, "request timed out"),
		status.Error(codes.Unavailable, `dial unix C:\Users\mikeg\.agora\network.sock: access denied`),
		&os.PathError{Op: "dial", Path: "missing.sock", Err: os.ErrNotExist},
		fmt.Errorf("dial: %w", syscall.ECONNREFUSED),
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			err := normalizeDialError(cause)
			if !errors.Is(err, ErrNotRunning) {
				t.Fatalf("expected ErrNotRunning, got %v", err)
			}
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), cause.Error()) {
				t.Fatalf("lost underlying error %v: %v", cause, err)
			}
		})
	}
	for _, err := range []error{nil, errors.New("unexpected error"), status.Error(codes.PermissionDenied, "permission denied")} {
		if got := normalizeDialError(err); got != err {
			t.Errorf("expected unchanged error %v, got %v", err, got)
		}
	}
}

func TestDaemonCallsReturnErrNotRunningWhenSocketMissing(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "missing.sock")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := Ping(ctx, socketPath); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning from Ping, got %v", err)
	}
	if _, err := Status(ctx, socketPath); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning from Status, got %v", err)
	}
	if err := Stop(ctx, socketPath); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning from Stop, got %v", err)
	}
}
