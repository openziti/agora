package agent_test

import (
	"context"

	"github.com/openziti/agora/sdk/agent"
)

func ExampleNewStandalone() {
	ctx := context.Background()

	run := func() error {
		a, err := agent.NewStandalone(agent.StandaloneOptions{
			Name:        "llm-gateway",
			EnvRoot:     "/path/to/enrolled/.agora",
			WithRuntime: true,
		})
		if err != nil {
			return err
		}
		defer func() { _ = a.Close(ctx) }()

		return a.StartRuntime(ctx)
	}
	_ = run
}
