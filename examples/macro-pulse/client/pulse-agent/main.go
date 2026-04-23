package main

import (
	"context"
	"flag"
	"os"

	"github.com/openziti/agora/sdk/agent"
)

func main() {
	fs := flag.NewFlagSet("pulse-agent", flag.ExitOnError)

	app := agent.New("pulse-agent",
		agent.WithDescription("Macro Pulse orchestrator: composes across providers and tools to produce the morning brief"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		a.Log().Info("alive")
		<-ctx.Done()
		return nil
	}); err != nil {
		os.Exit(1)
	}
}
