package environment

import (
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/environment/env_v0"
)

func SetRootDirName(name string) {
	env_v0.SetRootDirName(name)
}

func LoadRoot() (env_core.Root, error) {
	if assert, err := env_v0.Assert(); assert && err == nil {
		return env_v0.Load()
	}
	return env_v0.Default()
}

func IsLatest(r env_core.Root) bool {
	if r == nil || r.Metadata() == nil {
		return false
	}
	return r.Metadata().V == env_v0.V
}
