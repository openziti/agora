package agent

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/openziti/agora/environment/env_core"
)

// EmbeddedOptions configures an Runtime constructed for in-process
// (library) use rather than as the standalone `agora network start`
// daemon. Embedded Agents never touch ~/.agora/network.json and never
// bind the daemon UDS. They are isolated from any daemon running on
// the same host; only the enrolled environment (account token and
// identity material) is shared.
type EmbeddedOptions struct {
	// Environment is the enrolled environment the runtime binds to.
	// Required.
	Environment *env_core.Environment

	// IdentityPath is the absolute filesystem path to the enrolled
	// OpenZiti identity that the runtime uses to dial the fabric.
	// Typically derived from the environment's own ~/.agora layout,
	// but supplied explicitly so embedded callers can point at any
	// identity they control. Required.
	IdentityPath string

	// Optional timing overrides. Zero values use package defaults.
	HeartbeatInterval        time.Duration
	HeartbeatTTL             time.Duration
	HeartbeatRequestTimeout  time.Duration
	ServeHeartbeatInterval   time.Duration
	ConnectHeartbeatInterval time.Duration
	RetryInitialDelay        time.Duration
	RetryMaxDelay            time.Duration
	RetryMultiplier          float64
	RetryJitter              float64
}

// NewEmbedded constructs an Runtime for in-process use. The returned
// Runtime has no environment root, no desired-state file, and no UDS
// listener. Call Start to begin heartbeat and reconciliation loops,
// and Shutdown to drain.
func NewEmbedded(opts EmbeddedOptions) (*Runtime, error) {
	if opts.Environment == nil {
		return nil, errors.New("agent.NewEmbedded: Environment is required")
	}
	if opts.IdentityPath == "" {
		return nil, errors.New("agent.NewEmbedded: IdentityPath is required")
	}

	agent := &Runtime{
		root:                     nil,
		env:                      cloneEnvironment(opts.Environment),
		network:                  &env_core.Network{},
		identityPath:             opts.IdentityPath,
		serves:                   map[string]*managedServe{},
		connects:                 map[string]*managedConnect{},
		socketPath:               "",
		startedAt:                time.Now().UTC(),
		pid:                      os.Getpid(),
		now:                      time.Now,
		heartbeatInterval:        orDefault(opts.HeartbeatInterval, defaultEnvironmentHeartbeatInterval),
		heartbeatTTL:             orDefault(opts.HeartbeatTTL, defaultEnvironmentHeartbeatTTL),
		heartbeatRequestTimeout:  orDefault(opts.HeartbeatRequestTimeout, defaultEnvironmentHeartbeatRequestTimeout),
		shutdownCh:               make(chan struct{}),
		controller:               apiTunnelController{},
		runtimeHost:              defaultRuntimeHost{},
		serveHeartbeatInterval:   orDefault(opts.ServeHeartbeatInterval, tunnelServeHeartbeatInterval),
		connectHeartbeatInterval: orDefault(opts.ConnectHeartbeatInterval, tunnelAttachmentHeartbeatInterval),
		retryInitialDelay:        orDefault(opts.RetryInitialDelay, defaultTunnelRetryInitialDelay),
		retryMaxDelay:            orDefault(opts.RetryMaxDelay, defaultTunnelRetryMaxDelay),
		retryMultiplier:          orFloat(opts.RetryMultiplier, defaultTunnelRetryMultiplier),
		retryJitter:              orFloat(opts.RetryJitter, defaultTunnelRetryJitter),
		retryRand:                rand.Float64,
	}
	agent.heartbeatSender = agent.sendEnvironmentHeartbeat
	return agent, nil
}

// Start begins the runtime's heartbeat and reconciliation loops.
// Non-blocking. Must be called before any EnsureServe/EnsureConnect
// call on an embedded Runtime.
func (a *Runtime) Start(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.env == nil {
		return fmt.Errorf("agent.Start: no environment configured")
	}
	a.replaceEnvironmentHeartbeatLoopLocked()
	a.startConfiguredActorsLocked()
	return nil
}

// Close stops the runtime cleanly: cancels in-flight serve/connect
// actors (deprovisioning controller-side serve and attachment
// records), stops the environment heartbeat loop, and returns when
// all runtime goroutines have exited. Intended for embedded callers
// that constructed the Runtime via NewEmbedded and drove it via Start.
//
// The standalone `agora network start` daemon tears down through its
// own Run-level cleanup path instead of calling Close.
func (a *Runtime) Close(_ context.Context) error {
	a.shutdownManagedActors()
	a.stopEnvironmentHeartbeatLoop()
	a.stop()
	return nil
}

func orDefault(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

func orFloat(v, def float64) float64 {
	if v != 0 {
		return v
	}
	return def
}
