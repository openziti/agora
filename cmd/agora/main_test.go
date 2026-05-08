package main

import (
	"testing"

	"github.com/openziti/agora/environment"
)

func TestConfigureEnvironmentRootFromEnv(t *testing.T) {
	defer environment.SetRootDirName(".agora")

	rootPath := t.TempDir()
	t.Setenv("AGORA_ENV_ROOT", rootPath)

	configureEnvironmentRootFromEnv()

	root, err := environment.LoadRoot()
	if err != nil {
		t.Fatalf("load root: %v", err)
	}
	if got := root.Metadata().RootPath; got != rootPath {
		t.Fatalf("expected root path %q, got %q", rootPath, got)
	}
}
