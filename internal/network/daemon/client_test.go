package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPingAndStatusReturnErrNotRunningWhenSocketMissing(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "missing.sock")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := Ping(ctx, socketPath); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning from Ping, got %v", err)
	}
	if _, err := Status(ctx, socketPath); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning from Status, got %v", err)
	}
}
