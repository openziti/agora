package agent

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
)

// App is the configuration + lifecycle entrypoint for an SDK agent.
// Construct via New; drive via (*App).Run.
type App struct {
	name        string
	description string
	userFlags   *flag.FlagSet
	envRoot     string
	wantRuntime bool
}

// Agent is the SDK handle passed to an agent's business-logic
// function. It exposes the enrolled environment, a controller API
// client, an optional embedded Runtime (when WithRuntime was set),
// and a pre-scoped df/dl log builder.
type Agent struct {
	appName      string
	root         env_core.Root
	env          *env_core.Environment
	accountID    string
	api          *api.Client
	runtime      *Runtime
	runtimeMu    sync.Mutex
	runtimeState agentRuntimeState
	live         bool
}

type agentRuntimeState uint8

const (
	agentRuntimeNotStarted agentRuntimeState = iota
	agentRuntimeStarted
	agentRuntimeStopped
)

// Option configures an App.
type Option func(*App)

// WithDescription sets the agent's human-readable description.
// Included in log output and, later, advertisement metadata.
func WithDescription(desc string) Option {
	return func(a *App) { a.description = desc }
}

// WithFlagSet registers a user-provided *flag.FlagSet. Its flags are
// merged with the SDK's built-in flags during Run.
func WithFlagSet(fs *flag.FlagSet) Option {
	return func(a *App) { a.userFlags = fs }
}

// WithEnvRoot overrides the default environment root path. The
// default is the value of AGORA_ENV_ROOT if set, else the standard
// ~/.agora location. The --env-root built-in flag also overrides.
func WithEnvRoot(path string) Option {
	return func(a *App) { a.envRoot = path }
}

// WithRuntime causes Run to construct and start an embedded Layer 1
// runtime for the agent. Required for agents that accept sessions
// (providers, analytics tools) or open sessions (consumers). Omit
// for pure controller-API consumers.
//
// The embedded runtime does not touch ~/.agora/network.json and does
// not bind ~/.agora/network.sock; it is isolated from any standalone
// `agora network start` daemon running on the same host.
func WithRuntime() Option {
	return func(a *App) { a.wantRuntime = true }
}

// New constructs an App with the given name and options.
func New(name string, opts ...Option) *App {
	a := &App{name: name}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Run parses flags, initializes the agent (including an embedded
// Runtime if WithRuntime was set), invokes fn with a cancellable
// context and a fully-initialized *Agent, and blocks until fn
// returns or SIGINT/SIGTERM fires. On signal, the context is
// cancelled; fn is expected to return promptly. Returns fn's error
// or a setup error.
func (app *App) Run(fn func(context.Context, *Agent) error) error {
	if fn == nil {
		return errors.New("agent.App.Run: business-logic function is required")
	}

	var (
		fEnvRoot  string
		fLogLevel string
		fVerbose  bool
		fLive     bool
	)
	baseFlags := flag.NewFlagSet(app.name, flag.ExitOnError)
	baseFlags.StringVar(&fEnvRoot, "env-root", os.Getenv("AGORA_ENV_ROOT"), "Override the Agora environment root (defaults to ~/.agora)")
	baseFlags.StringVar(&fLogLevel, "log-level", envOr("AGORA_LOG_LEVEL", "info"), "Log level: debug, info, warn, error")
	baseFlags.BoolVar(&fVerbose, "v", false, "Verbose: shorthand for --log-level=debug")
	baseFlags.BoolVar(&fLive, "live", false, "Use live external data sources instead of local snapshots (provider-specific)")
	if app.userFlags != nil {
		app.userFlags.VisitAll(func(f *flag.Flag) {
			if baseFlags.Lookup(f.Name) != nil {
				return
			}
			baseFlags.Var(f.Value, f.Name, f.Usage)
		})
	}
	if err := baseFlags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if fVerbose {
		fLogLevel = "debug"
	}

	level := parseLogLevel(fLogLevel)
	// Operational logs go to stderr by convention so an agent's stdout
	// can be used cleanly for the program's own output (e.g. the macro
	// pulse orchestrator's brief).
	dl.Init(dl.DefaultOptions().SetOutput(os.Stderr).SetLevel(level).SetTrimPrefix("github.com/openziti/"))

	effectiveEnvRoot := fEnvRoot
	if effectiveEnvRoot == "" {
		effectiveEnvRoot = app.envRoot
	}
	if effectiveEnvRoot != "" {
		environment.SetRootDirName(effectiveEnvRoot)
	}

	root, err := environment.LoadRoot()
	if err != nil {
		return fmt.Errorf("load environment root: %w", err)
	}
	if !root.IsEnabled() {
		return fmt.Errorf("environment is not enabled (run `agora enable <account-token>` or set --env-root)")
	}
	env := root.Environment()

	apiClient, err := root.Client()
	if err != nil {
		return fmt.Errorf("build controller api client: %w", err)
	}

	a := &Agent{
		appName:   app.name,
		root:      root,
		env:       env,
		accountID: env.EnvironmentID,
		api:       apiClient,
		live:      fLive,
	}

	if app.wantRuntime {
		identityPath, idErr := root.ZitiIdentityNamed(environmentIdentityName)
		if idErr != nil {
			return fmt.Errorf("locate environment identity: %w", idErr)
		}
		rt, rtErr := NewEmbedded(EmbeddedOptions{
			Environment:  env,
			IdentityPath: identityPath,
		})
		if rtErr != nil {
			return fmt.Errorf("build embedded runtime: %w", rtErr)
		}
		if startErr := rt.Start(context.Background()); startErr != nil {
			return fmt.Errorf("start embedded runtime: %w", startErr)
		}
		a.runtime = rt
		a.runtimeState = agentRuntimeStarted
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.Log().Infof("agent '%s' starting", app.name)
	if app.description != "" {
		a.Log().Debugf("description: %s", app.description)
	}

	runErr := fn(ctx, a)

	if app.wantRuntime {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := a.Close(shutdownCtx); err != nil {
			dl.Warnf("runtime close error: %v", err)
		}
		cancel()
	}

	a.Log().Infof("agent '%s' stopped", app.name)
	return runErr
}

// Name returns the agent name passed to New.
func (a *Agent) Name() string { return a.appName }

// EnvRoot returns the loaded local environment root.
func (a *Agent) EnvRoot() env_core.Root { return a.root }

// Environment returns the enrolled environment record.
func (a *Agent) Environment() *env_core.Environment { return a.env }

// AccountID returns the enrolled environment's ID, which uniquely
// identifies this agent instance within its organization.
func (a *Agent) AccountID() string { return a.accountID }

// Controller returns a controller API client authenticated against
// the enrolled account token.
func (a *Agent) Controller() *api.Client { return a.api }

// Runtime returns the started embedded Layer 1 runtime, or nil if the
// agent has no runtime or its runtime has not been started.
func (a *Agent) Runtime() *Runtime {
	if a == nil {
		return nil
	}
	a.runtimeMu.Lock()
	defer a.runtimeMu.Unlock()
	if a.runtimeState != agentRuntimeStarted {
		return nil
	}
	return a.runtime
}

// Log returns a df/dl structured-log builder with the agent name and
// environment ID pre-populated.
func (a *Agent) Log() *dl.Builder {
	return dl.Log().With("agent", a.appName).With("environment_id", a.accountID)
}

// Live reports whether the --live flag was passed.
func (a *Agent) Live() bool { return a.live }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
