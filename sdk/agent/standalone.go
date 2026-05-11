package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/openziti/agora/environment"
)

// StandaloneOptions configures an Agent constructed for caller-driven
// lifecycle. The caller is responsible for starting the optional
// embedded runtime, handling signals, and calling Close when done.
type StandaloneOptions struct {
	// Name is the agent name. Required.
	Name string

	// Description is optional human-readable text.
	Description string

	// EnvRoot is the path to the enrolled environment root. Empty
	// falls back to AGORA_ENV_ROOT, then the default ~/.agora root.
	EnvRoot string

	// WithRuntime causes NewStandalone to construct, but not start, an
	// embedded Layer 1 runtime.
	WithRuntime bool

	// Live mirrors the --live flag's effect for SDK consumers that
	// honor it.
	Live bool
}

// NewStandalone constructs an Agent from explicit options, without
// signal handling, flag parsing, os.Args access, or global logger
// initialization. It is suitable for external Go modules with their
// own process lifecycle.
//
// When EnvRoot or AGORA_ENV_ROOT is set, NewStandalone applies it to
// the environment package's process-wide root before loading the
// enrolled environment. That root mutation is the only global state
// this constructor changes.
func NewStandalone(opts StandaloneOptions) (*Agent, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, errors.New("agent.NewStandalone: Name is required")
	}

	envRoot := opts.EnvRoot
	if envRoot == "" {
		envRoot = os.Getenv("AGORA_ENV_ROOT")
	}
	if envRoot != "" {
		environment.SetRootDirName(envRoot)
	}

	root, err := environment.LoadRoot()
	if err != nil {
		return nil, fmt.Errorf("load environment root: %w", err)
	}
	if !root.IsEnabled() {
		return nil, fmt.Errorf("environment is not enabled (run `agora enable <account-token>` or set --env-root)")
	}
	env := root.Environment()

	apiClient, err := root.Client()
	if err != nil {
		return nil, fmt.Errorf("build controller api client: %w", err)
	}

	a := &Agent{
		appName:   name,
		root:      root,
		env:       env,
		accountID: env.EnvironmentID,
		api:       apiClient,
		live:      opts.Live,
	}

	if opts.WithRuntime {
		identityPath, idErr := root.ZitiIdentityNamed(environmentIdentityName)
		if idErr != nil {
			return nil, fmt.Errorf("locate environment identity: %w", idErr)
		}
		rt, rtErr := NewEmbedded(EmbeddedOptions{
			Environment:  env,
			IdentityPath: identityPath,
		})
		if rtErr != nil {
			return nil, fmt.Errorf("build embedded runtime: %w", rtErr)
		}
		a.runtime = rt
	}

	return a, nil
}

// StartRuntime starts the embedded runtime. It is a no-op when the
// agent has no runtime and is idempotent while the runtime is already
// started. A stopped runtime cannot be restarted; construct a new
// Agent instead.
func (a *Agent) StartRuntime(ctx context.Context) error {
	if a == nil {
		return errors.New("agent.StartRuntime: agent is nil")
	}

	a.runtimeMu.Lock()
	defer a.runtimeMu.Unlock()

	if a.runtime == nil {
		return nil
	}
	switch a.runtimeState {
	case agentRuntimeStarted:
		return nil
	case agentRuntimeStopped:
		return errors.New("agent.StartRuntime: runtime has been stopped")
	}

	if err := a.runtime.Start(ctx); err != nil {
		return fmt.Errorf("start embedded runtime: %w", err)
	}
	a.runtimeState = agentRuntimeStarted
	return nil
}

// StopRuntime stops the embedded runtime. It is a no-op when the
// agent has no runtime, when the runtime was never started, or when it
// has already been stopped. Once a started runtime has stopped, the
// agent cannot be restarted.
func (a *Agent) StopRuntime(ctx context.Context) error {
	if a == nil {
		return errors.New("agent.StopRuntime: agent is nil")
	}

	a.runtimeMu.Lock()
	defer a.runtimeMu.Unlock()

	if a.runtime == nil || a.runtimeState == agentRuntimeNotStarted {
		return nil
	}
	if a.runtimeState == agentRuntimeStopped {
		return nil
	}
	if err := a.runtime.Close(ctx); err != nil {
		return fmt.Errorf("stop embedded runtime: %w", err)
	}
	a.runtimeState = agentRuntimeStopped
	return nil
}

// Close releases agent-held resources. It is idempotent and terminal:
// after Close, a constructed runtime cannot be started or restarted.
func (a *Agent) Close(ctx context.Context) error {
	if a == nil {
		return errors.New("agent.Close: agent is nil")
	}

	a.runtimeMu.Lock()
	defer a.runtimeMu.Unlock()

	if a.runtimeState == agentRuntimeStopped {
		return nil
	}
	if a.runtime == nil || a.runtimeState == agentRuntimeNotStarted {
		a.runtimeState = agentRuntimeStopped
		return nil
	}
	if err := a.runtime.Close(ctx); err != nil {
		return fmt.Errorf("close embedded runtime: %w", err)
	}
	a.runtimeState = agentRuntimeStopped
	return nil
}
