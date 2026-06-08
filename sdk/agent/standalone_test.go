package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
)

func TestNewStandaloneConstructsUnstartedRuntime(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	prepareStandaloneRoot(t, rootPath)

	a, err := NewStandalone(StandaloneOptions{
		Name:        " llm-gateway ",
		EnvRoot:     rootPath,
		WithRuntime: true,
		Live:        true,
	})
	if err != nil {
		t.Fatalf("new standalone: %v", err)
	}
	if a.Name() != "llm-gateway" {
		t.Fatalf("unexpected name %q", a.Name())
	}
	if !a.Live() {
		t.Fatal("expected live flag to be set")
	}
	if a.Runtime() != nil {
		t.Fatal("runtime should not be visible before StartRuntime")
	}
	if a.runtime == nil {
		t.Fatal("expected embedded runtime to be constructed")
	}
	a.runtime.heartbeatSender = func(context.Context, *env_core.Environment) (bool, error) {
		return false, nil
	}

	if err := a.StartRuntime(context.Background()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	if a.Runtime() == nil {
		t.Fatal("expected runtime after StartRuntime")
	}
	if err := a.StartRuntime(context.Background()); err != nil {
		t.Fatalf("second start should be idempotent: %v", err)
	}
	if err := a.StopRuntime(context.Background()); err != nil {
		t.Fatalf("stop runtime: %v", err)
	}
	if a.Runtime() != nil {
		t.Fatal("runtime should not be visible after StopRuntime")
	}
	if err := a.StopRuntime(context.Background()); err != nil {
		t.Fatalf("second stop should be idempotent: %v", err)
	}
	if err := a.StartRuntime(context.Background()); err == nil || !strings.Contains(err.Error(), "runtime has been stopped") {
		t.Fatalf("expected stopped runtime error, got %v", err)
	}
}

func TestNewStandaloneCloseBeforeStartIsTerminal(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	prepareStandaloneRoot(t, rootPath)

	a, err := NewStandalone(StandaloneOptions{Name: "gateway", EnvRoot: rootPath, WithRuntime: true})
	if err != nil {
		t.Fatalf("new standalone: %v", err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("close before start: %v", err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("second close should be idempotent: %v", err)
	}
	if err := a.StartRuntime(context.Background()); err == nil {
		t.Fatal("expected StartRuntime after Close to fail")
	}
}

func TestNewStandaloneWithoutRuntimeLifecycleNoops(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	prepareStandaloneRoot(t, rootPath)

	a, err := NewStandalone(StandaloneOptions{Name: "catalog-only", EnvRoot: rootPath})
	if err != nil {
		t.Fatalf("new standalone: %v", err)
	}
	if a.Runtime() != nil {
		t.Fatal("expected no runtime")
	}
	if err := a.StartRuntime(context.Background()); err != nil {
		t.Fatalf("start without runtime should be no-op: %v", err)
	}
	if err := a.StopRuntime(context.Background()); err != nil {
		t.Fatalf("stop without runtime should be no-op: %v", err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("close without runtime should be no-op: %v", err)
	}
}

func TestNewStandaloneEnvRootFromEnvironment(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	prepareStandaloneRoot(t, rootPath)
	t.Setenv("AGORA_ENV_ROOT", rootPath)

	a, err := NewStandalone(StandaloneOptions{Name: "from-env"})
	if err != nil {
		t.Fatalf("new standalone: %v", err)
	}
	if got := a.EnvRoot().Metadata().RootPath; got != rootPath {
		t.Fatalf("expected env root %q, got %q", rootPath, got)
	}
}

func TestNewStandaloneCapturesStableIdentityPath(t *testing.T) {
	dir := t.TempDir()
	firstRoot := filepath.Join(dir, "first")
	secondRoot := filepath.Join(dir, "second")
	prepareStandaloneRoot(t, firstRoot)
	prepareStandaloneRoot(t, secondRoot)

	first, err := NewStandalone(StandaloneOptions{Name: "first", EnvRoot: firstRoot})
	if err != nil {
		t.Fatalf("new first standalone: %v", err)
	}
	second, err := NewStandalone(StandaloneOptions{Name: "second", EnvRoot: secondRoot})
	if err != nil {
		t.Fatalf("new second standalone: %v", err)
	}

	if got, want := first.EnvironmentIdentityPath(), filepath.Join(firstRoot, "identities", "environment.json"); got != want {
		t.Fatalf("expected first identity path %q, got %q", want, got)
	}
	if got, want := second.EnvironmentIdentityPath(), filepath.Join(secondRoot, "identities", "environment.json"); got != want {
		t.Fatalf("expected second identity path %q, got %q", want, got)
	}
}

func TestNewStandaloneValidation(t *testing.T) {
	if _, err := NewStandalone(StandaloneOptions{}); err == nil {
		t.Fatal("expected missing name error")
	}

	rootPath := filepath.Join(t.TempDir(), ".agora")
	environment.SetRootDirName(rootPath)
	if _, err := NewStandalone(StandaloneOptions{Name: "disabled", EnvRoot: rootPath}); err == nil || !strings.Contains(err.Error(), "environment is not enabled") {
		t.Fatalf("expected disabled environment error, got %v", err)
	}
}

func TestAgentLifecycleNilReceiverErrors(t *testing.T) {
	var a *Agent
	if err := a.StartRuntime(context.Background()); err == nil {
		t.Fatal("expected nil StartRuntime error")
	}
	if err := a.StopRuntime(context.Background()); err == nil {
		t.Fatal("expected nil StopRuntime error")
	}
	if err := a.Close(context.Background()); err == nil {
		t.Fatal("expected nil Close error")
	}
}

func TestNewStandaloneStartRuntimeFailureDoesNotStopLifecycle(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), ".agora")
	prepareStandaloneRoot(t, rootPath)

	a, err := NewStandalone(StandaloneOptions{Name: "gateway", EnvRoot: rootPath, WithRuntime: true})
	if err != nil {
		t.Fatalf("new standalone: %v", err)
	}
	a.runtime.env = nil
	if err := a.StartRuntime(context.Background()); err == nil {
		t.Fatal("expected start error")
	}
	a.runtime.env = &env_core.Environment{EnvironmentID: "ev_teststandaln", AccountToken: "token", APIEndpoint: "http://controller.example"}
	a.runtime.heartbeatSender = func(context.Context, *env_core.Environment) (bool, error) {
		return false, nil
	}
	if err := a.StartRuntime(context.Background()); err != nil {
		t.Fatalf("expected retryable start lifecycle, got %v", err)
	}
	if err := a.Close(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("close: %v", err)
	}
}

func prepareStandaloneRoot(t *testing.T, rootPath string) {
	t.Helper()
	environment.SetRootDirName(rootPath)
	root := loadTestRoot(t)
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: "ev_teststandaln",
		AccountToken:  "account-token",
		APIEndpoint:   "http://controller.example",
	}); err != nil {
		t.Fatalf("set environment: %v", err)
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, `{"ztAPI":"test"}`); err != nil {
		t.Fatalf("save identity: %v", err)
	}
}
